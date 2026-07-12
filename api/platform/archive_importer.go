package platform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/api/logs"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// ArchiveImportRequest 只携带已通过校验的归档流和受控候选 tag；调用方不得把用户输入当作 Docker 参数。
type ArchiveImportRequest struct {
	EndpointID   int
	Archive      io.Reader
	SourceRef    string
	CandidateRef string
	Architecture string
}

type ArchiveImportResult struct {
	CandidateRef string
	ImageID      string
	Architecture string
	Created      string
}

// ArchiveImageImporter 使 archive 安全校验与 Docker I/O 解耦，handler 可在事务外执行导入并可靠清理候选镜像。
type ArchiveImageImporter interface {
	Import(context.Context, ArchiveImportRequest) (ArchiveImportResult, error)
	Cleanup(context.Context, int, string) error
}

type dockerArchiveClientFactory interface {
	CreateClient(endpoint *portainer.Endpoint, nodeName string, timeout *time.Duration) (*client.Client, error)
}

// DockerArchiveImporter 只接受已完成安全校验的流。导入后立即转成平台唯一候选 tag，
// 再删除 archive 原始 tag，避免未推送的用户命名镜像被误当作可部署制品。
type DockerArchiveImporter struct {
	dataStore dataservices.DataStore
	factory   dockerArchiveClientFactory
}

func NewDockerArchiveImporter(dataStore dataservices.DataStore, factory *dockerclient.ClientFactory) *DockerArchiveImporter {
	return &DockerArchiveImporter{dataStore: dataStore, factory: factory}
}

func (importer *DockerArchiveImporter) Import(ctx context.Context, request ArchiveImportRequest) (ArchiveImportResult, error) {
	if importer == nil || importer.dataStore == nil || importer.factory == nil || request.Archive == nil || request.CandidateRef == "" {
		return ArchiveImportResult{}, errors.New("docker archive importer is unavailable")
	}
	endpoint, err := importer.dataStore.Endpoint().Endpoint(portainer.EndpointID(request.EndpointID))
	if err != nil {
		return ArchiveImportResult{}, err
	}
	cli, err := importer.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return ArchiveImportResult{}, err
	}
	defer logs.CloseAndLogErr(cli)

	if request.SourceRef != "" {
		if _, err := cli.ImageInspect(ctx, request.SourceRef); err == nil {
			return ArchiveImportResult{}, errors.New("archive source tag already exists")
		} else if !errdefs.IsNotFound(err) {
			return ArchiveImportResult{}, err
		}
	}
	loaded, err := cli.ImageLoad(ctx, request.Archive, client.ImageLoadWithQuiet(true))
	if err != nil {
		return ArchiveImportResult{}, err
	}
	defer logs.CloseAndLogErr(loaded.Body)
	output, err := io.ReadAll(io.LimitReader(loaded.Body, 1<<20))
	if err != nil {
		return ArchiveImportResult{}, err
	}
	source := request.SourceRef
	if source == "" {
		source = loadedImageReference(string(output))
		if source == "" {
			return ArchiveImportResult{}, errors.New("docker load did not return an image reference")
		}
	}
	importSucceeded := false
	defer func() {
		if !importSucceeded && source != "" {
			// source 在导入前已确认不存在；失败时仅移除本次 Docker load 新增的临时引用。
			_, _ = cli.ImageRemove(context.Background(), source, image.RemoveOptions{Force: false, PruneChildren: false})
		}
	}()
	if err := cli.ImageTag(ctx, source, request.CandidateRef); err != nil {
		return ArchiveImportResult{}, err
	}
	inspect, _, err := cli.ImageInspectWithRaw(ctx, request.CandidateRef)
	if err != nil {
		_ = importer.Cleanup(context.Background(), request.EndpointID, request.CandidateRef)
		return ArchiveImportResult{}, err
	}
	if request.Architecture != "" && inspect.Architecture != request.Architecture {
		_ = importer.Cleanup(context.Background(), request.EndpointID, request.CandidateRef)
		return ArchiveImportResult{}, errors.New("loaded image architecture changed")
	}
	if source != request.CandidateRef {
		_, _ = cli.ImageRemove(ctx, source, image.RemoveOptions{Force: false, PruneChildren: false})
	}
	importSucceeded = true
	return ArchiveImportResult{CandidateRef: request.CandidateRef, ImageID: inspect.ID, Architecture: inspect.Architecture, Created: inspect.Created}, nil
}

func (importer *DockerArchiveImporter) Cleanup(ctx context.Context, endpointID int, candidateRef string) error {
	if importer == nil || importer.dataStore == nil || importer.factory == nil || candidateRef == "" {
		return nil
	}
	endpoint, err := importer.dataStore.Endpoint().Endpoint(portainer.EndpointID(endpointID))
	if err != nil {
		return err
	}
	cli, err := importer.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)
	_, err = cli.ImageRemove(ctx, candidateRef, image.RemoveOptions{Force: true, PruneChildren: false})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

var loadedImagePattern = regexp.MustCompile(`(?m)Loaded image(?: ID)?:\s*([^\s]+)`)

func loadedImageReference(output string) string {
	match := loadedImagePattern.FindStringSubmatch(strings.TrimSpace(output))
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(bytes.NewBufferString(match[1]).String())
}
