package platform

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	StaticNginxBuildTemplate = "static-nginx-v1"
	staticNginxBaseImage     = "nginx:1.27-alpine"
	maxDistFiles             = 50_000
	maxDistExpandedSize      = 2 << 30
)

var (
	errDistArchiveInvalid = errors.New("dist archive is invalid")
	errDistArchiveUnsafe  = errors.New("dist archive is unsafe")
	errDistEntryMissing   = errors.New("dist entry is missing")
	errDistOptionsInvalid = errors.New("dist build options are invalid")
	distConfigPathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

type DistMode string

const (
	DistModeSPA DistMode = "spa"
	DistModeMPA DistMode = "mpa"
)

type DistCachePolicy string

const (
	DistCachePolicyDefault   DistCachePolicy = ""
	DistCachePolicyNoCache   DistCachePolicy = "no-cache"
	DistCachePolicyImmutable DistCachePolicy = "immutable"
)

// DistBuildOptions 只表达平台允许的静态站点行为；不允许把用户输入转换为 Nginx 配置或构建命令。
type DistBuildOptions struct {
	Mode         DistMode
	CachePolicy  DistCachePolicy
	NotFoundPage string
}

// StaticImageBuilder 与 Java 包装共用同一个受控 Docker build adapter。两类构建上下文都只能由平台生成，
// 因此复用候选镜像清理契约，而不是开放一个可接收用户 Dockerfile 的泛化接口。
type StaticImageBuilder = JavaImageBuilder
type StaticImageBuildRequest = JavaImageBuildRequest
type StaticImageBuildResult = JavaImageBuildResult

// ValidateDistBuildOptions 把静态站点的少量可选项限制为枚举和安全的容器内相对路径，避免 Nginx 指令注入。
func ValidateDistBuildOptions(options DistBuildOptions) error {
	if options.Mode != DistModeSPA && options.Mode != DistModeMPA {
		return errDistOptionsInvalid
	}
	if options.CachePolicy != DistCachePolicyDefault && options.CachePolicy != DistCachePolicyNoCache && options.CachePolicy != DistCachePolicyImmutable {
		return errDistOptionsInvalid
	}
	if options.NotFoundPage != "" && !isSafeDistConfigPath(options.NotFoundPage) {
		return errDistOptionsInvalid
	}
	return nil
}

// StaticBuildTemplateName 将受控行为编码为模板追溯信息。可选 404 页只记录是否启用，避免把用户文件路径扩散到审计和响应。
func StaticBuildTemplateName(options DistBuildOptions) string {
	cachePolicy := "default"
	if options.CachePolicy != DistCachePolicyDefault {
		cachePolicy = string(options.CachePolicy)
	}
	notFound := "no404"
	if options.NotFoundPage != "" {
		notFound = "custom404"
	}
	return fmt.Sprintf("%s:%s:%s:%s", StaticNginxBuildTemplate, options.Mode, cachePolicy, notFound)
}

// NewStaticDistBuildContext 保留内存版本供小型单元测试和兼容调用使用。生产 handler 使用文件版本，
// 防止合法但接近解压上限的 dist 包占满服务进程内存。
func NewStaticDistBuildContext(filePath string, options DistBuildOptions) ([]byte, error) {
	var buffer bytes.Buffer
	if err := WriteStaticDistBuildContext(filePath, options, &buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// NewStaticDistBuildContextFile 在受控临时目录生成 build context。调用方负责在 Docker build 完成后关闭并删除文件，
// 这样 ZIP 从不解压到站点目录，也不会在失败时留下可被下次任务误用的半成品。
func NewStaticDistBuildContextFile(filePath, temporaryDirectory string, options DistBuildOptions) (*os.File, error) {
	if err := os.MkdirAll(temporaryDirectory, 0o700); err != nil {
		return nil, err
	}
	contextFile, err := os.CreateTemp(temporaryDirectory, "static-context-*")
	if err != nil {
		return nil, err
	}
	removeContext := true
	defer func() {
		if removeContext {
			_ = contextFile.Close()
			_ = os.Remove(contextFile.Name())
		}
	}()
	if err := WriteStaticDistBuildContext(filePath, options, contextFile); err != nil {
		return nil, err
	}
	if _, err := contextFile.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	removeContext = false
	return contextFile, nil
}

// WriteStaticDistBuildContext 逐条校验并写入 Docker context。它不把 ZIP 解压到文件系统，
// 因此路径穿越、链接和异常条目不会成为宿主机文件；解压大小和文件数仍由服务端硬性限制。
func WriteStaticDistBuildContext(filePath string, options DistBuildOptions, destination io.Writer) error {
	if err := ValidateDistBuildOptions(options); err != nil {
		return err
	}
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return errDistArchiveInvalid
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > maxDistFiles {
		return errDistArchiveUnsafe
	}

	writer := tar.NewWriter(destination)
	defer writer.Close()

	var totalSize int64
	entries := make(map[string]struct{}, len(reader.File))
	files := make(map[string]struct{}, len(reader.File))
	for _, entry := range reader.File {
		name, isDirectory, ok := safeDistZipEntry(entry)
		if !ok {
			return errDistArchiveUnsafe
		}
		if _, exists := entries[name]; exists {
			return errDistArchiveUnsafe
		}
		entries[name] = struct{}{}

		if isDirectory {
			continue
		}
		if entry.UncompressedSize64 > math.MaxInt64 || entry.UncompressedSize64 > uint64(maxDistExpandedSize-totalSize) {
			return errDistArchiveUnsafe
		}
		size := int64(entry.UncompressedSize64)
		totalSize += size
		source, err := entry.Open()
		if err != nil {
			return errDistArchiveInvalid
		}
		err = writeDistBuildContextFile(writer, "site/"+name, source, size)
		closeErr := source.Close()
		if err != nil || closeErr != nil {
			return errDistArchiveInvalid
		}
		files[name] = struct{}{}
	}

	if options.Mode == DistModeSPA {
		if _, found := files["index.html"]; !found {
			return errDistEntryMissing
		}
	}
	if options.NotFoundPage != "" {
		if _, found := files[options.NotFoundPage]; !found {
			return errDistEntryMissing
		}
	}

	if err := writeBuildContextFile(writer, "Dockerfile", []byte(staticDockerfile()), 0o600); err != nil {
		return err
	}
	if err := writeBuildContextFile(writer, "default.conf", []byte(staticNginxConfig(options)), 0o600); err != nil {
		return err
	}
	return writer.Close()
}

func writeDistBuildContextFile(writer *tar.Writer, name string, source io.Reader, size int64) error {
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: size, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	written, err := io.Copy(writer, io.LimitReader(source, size+1))
	if err != nil || written != size {
		return errDistArchiveInvalid
	}
	return nil
}

func staticDockerfile() string {
	return "FROM " + staticNginxBaseImage + "\nCOPY site/ /usr/share/nginx/html/\nCOPY default.conf /etc/nginx/conf.d/default.conf\n"
}

func staticNginxConfig(options DistBuildOptions) string {
	route := "try_files $uri $uri/ =404;"
	if options.Mode == DistModeSPA {
		route = "try_files $uri $uri/ /index.html;"
	}
	cacheHeader := ""
	if options.CachePolicy == DistCachePolicyNoCache {
		cacheHeader = "    add_header Cache-Control \"no-store\" always;\n"
	}
	config := "server {\n  listen 80;\n  root /usr/share/nginx/html;\n\n  location / {\n" + cacheHeader + "    " + route + "\n  }\n"
	if options.CachePolicy == DistCachePolicyImmutable {
		config += "\n  location ~* \\.(?:js|css|png|jpg|jpeg|gif|svg|ico|woff2?)$ {\n    try_files $uri =404;\n    add_header Cache-Control \"public, max-age=31536000, immutable\" always;\n  }\n"
	}
	if options.NotFoundPage != "" {
		config += "\n  error_page 404 /" + options.NotFoundPage + ";\n"
	}
	return config + "}\n"
}

func safeDistZipEntry(entry *zip.File) (string, bool, bool) {
	name := entry.Name
	isDirectory := entry.FileInfo().IsDir() || strings.HasSuffix(name, "/")
	name = strings.TrimSuffix(name, "/")
	if name == "" || strings.Contains(entry.Name, "\\") || strings.HasPrefix(entry.Name, "/") || path.Clean(name) != name {
		return "", false, false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false, false
		}
	}
	mode := entry.Mode()
	if mode&os.ModeSymlink != 0 {
		return "", false, false
	}
	if isDirectory {
		if mode&os.ModeType != 0 && mode&os.ModeType != os.ModeDir {
			return "", false, false
		}
	} else if mode&os.ModeType != 0 {
		return "", false, false
	}
	return name, isDirectory, true
}

func isSafeDistConfigPath(value string) bool {
	return distConfigPathPattern.MatchString(value) && path.Clean(value) == value && !strings.Contains(value, "..")
}

// DistBuildFailureReason 只返回冻结的 reason code，handler 不会把 ZIP、临时目录或 Docker 错误细节反馈给 API、审计或 toast。
func DistBuildFailureReason(err error) string {
	switch {
	case errors.Is(err, errDistEntryMissing):
		return "DIST_ENTRY_MISSING"
	case errors.Is(err, errDistArchiveUnsafe):
		return "ARCHIVE_UNSAFE"
	case errors.Is(err, errDistArchiveInvalid):
		return "ARTIFACT_TYPE_INVALID"
	default:
		return "CONTROLLED_BUILD_FAILED"
	}
}
