package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/api/logs"
)

type controlledBuildError struct {
	reason string
}

func (err controlledBuildError) Error() string {
	return err.reason
}

type dockerBuildStreamMessage struct {
	Stream   string `json:"stream"`
	Status   string `json:"status"`
	ID       string `json:"id"`
	Progress string `json:"progress"`
	Error    string `json:"error"`
}

// ControlledBuildFailureReason 仅将 Docker 构建输出归类为固定原因码。
// 原始输出可能包含镜像仓库地址、认证或环境细节，不能写入制品、审计、错误响应或前端日志。
func ControlledBuildFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "BUILD_TIMEOUT"
	}
	var buildErr controlledBuildError
	if errors.As(err, &buildErr) && buildErr.reason != "" {
		return buildErr.reason
	}
	return "CONTROLLED_BUILD_FAILED"
}

func consumeDockerBuildOutput(reader io.Reader, logWriter func(string)) error {
	decoder := json.NewDecoder(reader)
	for {
		var message dockerBuildStreamMessage
		if err := decoder.Decode(&message); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return controlledBuildError{reason: "CONTROLLED_BUILD_FAILED"}
		}
		if message.Stream != "" && logWriter != nil {
			logWriter(message.Stream)
		}
		if message.Status != "" && logWriter != nil {
			status := strings.TrimSpace(message.Status)
			if message.ID != "" {
				status += " " + message.ID
			}
			if message.Progress != "" {
				status += " " + strings.TrimSpace(message.Progress)
			}
			logWriter(status)
		}
		if message.Error != "" {
			if logWriter != nil {
				logWriter(message.Error)
			}
			return controlledBuildError{reason: classifyDockerBuildFailure(message.Error)}
		}
	}
}

func classifyDockerBuildFailure(message string) string {
	message = strings.ToLower(message)
	if strings.Contains(message, "pull") ||
		strings.Contains(message, "manifest unknown") ||
		strings.Contains(message, "not found") {
		return "BASE_IMAGE_UNAVAILABLE"
	}
	return "CONTROLLED_BUILD_FAILED"
}

type DockerJavaImageBuilder struct {
	dataStore dataservices.DataStore
	factory   dockerArchiveClientFactory
}

func NewDockerJavaImageBuilder(dataStore dataservices.DataStore, factory *dockerclient.ClientFactory) *DockerJavaImageBuilder {
	return &DockerJavaImageBuilder{dataStore: dataStore, factory: factory}
}
func (b *DockerJavaImageBuilder) Build(ctx context.Context, request JavaImageBuildRequest) (JavaImageBuildResult, error) {
	if b == nil || b.dataStore == nil || b.factory == nil || request.Context == nil {
		return JavaImageBuildResult{}, errors.New("java image builder unavailable")
	}
	endpoint, err := b.dataStore.Endpoint().Endpoint(portainer.EndpointID(request.EndpointID))
	if err != nil {
		return JavaImageBuildResult{}, err
	}
	cli, err := b.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return JavaImageBuildResult{}, err
	}
	defer logs.CloseAndLogErr(cli)
	response, err := cli.ImageBuild(ctx, request.Context, build.ImageBuildOptions{Dockerfile: "Dockerfile", Tags: []string{request.CandidateRef}, Remove: true, ForceRemove: true})
	if err != nil {
		return JavaImageBuildResult{}, err
	}
	defer logs.CloseAndLogErr(response.Body)
	if err := consumeDockerBuildOutput(response.Body, request.LogWriter); err != nil {
		return JavaImageBuildResult{}, err
	}
	inspect, _, err := cli.ImageInspectWithRaw(ctx, request.CandidateRef)
	if err != nil {
		_ = b.Cleanup(context.Background(), request.EndpointID, request.CandidateRef)
		return JavaImageBuildResult{}, err
	}
	return JavaImageBuildResult{CandidateRef: request.CandidateRef, ImageID: inspect.ID}, nil
}
func (b *DockerJavaImageBuilder) Cleanup(ctx context.Context, endpointID int, candidate string) error {
	if b == nil || b.dataStore == nil || b.factory == nil || candidate == "" {
		return nil
	}
	endpoint, err := b.dataStore.Endpoint().Endpoint(portainer.EndpointID(endpointID))
	if err != nil {
		return err
	}
	cli, err := b.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)
	_, err = cli.ImageRemove(ctx, candidate, image.RemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}
