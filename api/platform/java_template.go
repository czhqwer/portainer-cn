package platform

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const Java8BuildTemplate = "java8-jar-v1"
const java8BaseImage = "eclipse-temurin:8-jre"

type JavaBuildOptions struct {
	JVMArgs []string
	AppArgs []string
	Port    int
}

type JavaImageBuildRequest struct {
	EndpointID   int
	Context      io.Reader
	CandidateRef string
	LogWriter    func(string)
}
type JavaImageBuildResult struct {
	CandidateRef string
	ImageID      string
}
type JavaImageBuilder interface {
	Build(context.Context, JavaImageBuildRequest) (JavaImageBuildResult, error)
	Cleanup(context.Context, int, string) error
}

var javaArgumentPattern = regexp.MustCompile(`^[-A-Za-z0-9._:=/@+]+$`)

// ValidateJavaBuildOptions 只允许不含空白、shell 元字符或路径遍历的单个 token；这些 token 会被 JSON 数组 ENTRYPOINT 使用，绝不拼接 shell 命令。
func ValidateJavaBuildOptions(options JavaBuildOptions) error {
	if options.Port < 1 || options.Port > 65535 || len(options.JVMArgs) > 16 || len(options.AppArgs) > 16 {
		return errors.New("java build options are invalid")
	}
	for _, value := range append(append([]string{}, options.JVMArgs...), options.AppArgs...) {
		if !javaArgumentPattern.MatchString(value) || strings.Contains(value, "..") {
			return errors.New("java build argument is invalid")
		}
	}
	return nil
}

// ValidateJavaJar 确认制品是真实 ZIP/JAR 且包含 manifest；不解压用户内容，避免把构建前检查变成压缩炸弹入口。
func ValidateJavaJar(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil || info.Size() <= 0 {
		return errors.New("java archive is invalid")
	}
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return errors.New("java archive is invalid")
	}
	defer reader.Close()
	for _, file := range reader.File {
		if strings.EqualFold(file.Name, "META-INF/MANIFEST.MF") && !file.FileInfo().IsDir() {
			return nil
		}
	}
	return errors.New("java manifest is missing")
}

// NewJava8BuildContext 由平台构造唯一 Dockerfile 和固定 app.jar 名称。用户 Jar 只作为 tar 普通文件写入，不能提供额外构建上下文。
func NewJava8BuildContext(jar io.Reader, options JavaBuildOptions) ([]byte, error) {
	if err := ValidateJavaBuildOptions(options); err != nil {
		return nil, err
	}
	entrypoint := append([]string{"java"}, options.JVMArgs...)
	entrypoint = append(entrypoint, "-jar", "/opt/app/app.jar")
	entrypoint = append(entrypoint, options.AppArgs...)
	encoded, err := json.Marshal(entrypoint)
	if err != nil {
		return nil, err
	}
	dockerfile := fmt.Sprintf("FROM %s\nWORKDIR /opt/app\nCOPY app.jar /opt/app/app.jar\nEXPOSE %d\nENTRYPOINT %s\n", java8BaseImage, options.Port, encoded)
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writeBuildContextFile(writer, "Dockerfile", []byte(dockerfile), 0o600); err != nil {
		return nil, err
	}
	jarData, err := io.ReadAll(io.LimitReader(jar, 1<<30+1))
	if err != nil || len(jarData) > 1<<30 {
		return nil, errors.New("java archive is too large")
	}
	if err := writeBuildContextFile(writer, "app.jar", jarData, 0o600); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func writeBuildContextFile(writer *tar.Writer, name string, data []byte, mode int64) error {
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}
