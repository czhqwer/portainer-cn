package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/api/docker/images"
	"github.com/portainer/portainer/api/logs"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"
)

const maxRegistryPushOutput = 1 << 20

var registryDigestPattern = regexp.MustCompile(`sha256:[a-f0-9]{64}`)

type RegistryImagePushRequest struct {
	EndpointID   int
	RegistryID   portainer.RegistryID
	CandidateRef string
	TargetRef    string
}

type RegistryImagePushResult struct {
	ImageRef    string
	ImageTag    string
	ImageDigest string
}

// RegistryImagePusher 将 Docker 与 registry I/O 隔离在 handler 事务外，便于覆盖认证、tag 冲突和 digest 缺失的失败边界。
type RegistryImagePusher interface {
	Push(context.Context, RegistryImagePushRequest) (RegistryImagePushResult, error)
}

// RegistryPushError 只携带冻结的 reason code，禁止把 Docker 或 registry 返回的敏感内容写入 API、审计或 toast。
type RegistryPushError struct {
	Reason string
}

func (err *RegistryPushError) Error() string {
	return err.Reason
}

func RegistryPushFailureReason(err error) string {
	var pushErr *RegistryPushError
	if errors.As(err, &pushErr) && pushErr.Reason != "" {
		return pushErr.Reason
	}
	return "REGISTRY_PUSH_FAILED"
}

type DockerRegistryImagePusher struct {
	dataStore dataservices.DataStore
	factory   dockerArchiveClientFactory
}

func NewDockerRegistryImagePusher(dataStore dataservices.DataStore, factory *dockerclient.ClientFactory) *DockerRegistryImagePusher {
	return &DockerRegistryImagePusher{dataStore: dataStore, factory: factory}
}

// Push 使用 Portainer 已保存的 registry 凭据重新标记候选镜像。失败时仅移除本次新增的本地 target tag，
// 保留候选 tag 以便在修复认证或网络问题后重试，绝不修改 Release。
func (pusher *DockerRegistryImagePusher) Push(ctx context.Context, request RegistryImagePushRequest) (RegistryImagePushResult, error) {
	if pusher == nil || pusher.dataStore == nil || pusher.factory == nil || request.CandidateRef == "" || request.TargetRef == "" || request.RegistryID <= 0 {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	registry, err := pusher.dataStore.Registry().Read(request.RegistryID)
	if err != nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_AUTH_FAILED"}
	}
	auth, err := images.NewRegistryClient(pusher.dataStore).EncodedCertainRegistryAuth(registry)
	if err != nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_AUTH_FAILED"}
	}
	endpoint, err := pusher.dataStore.Endpoint().Endpoint(portainer.EndpointID(request.EndpointID))
	if err != nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	cli, err := pusher.factory.CreateClient(endpoint, "", nil)
	if err != nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	defer logs.CloseAndLogErr(cli)

	if _, _, err := cli.ImageInspectWithRaw(ctx, request.TargetRef); err == nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_TAG_CONFLICT"}
	} else if !errdefs.IsNotFound(err) {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	if _, _, err := cli.ImageInspectWithRaw(ctx, request.CandidateRef); err != nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	if err := cli.ImageTag(ctx, request.CandidateRef, request.TargetRef); err != nil {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	targetTagged := true
	defer func() {
		if targetTagged {
			_, _ = cli.ImageRemove(context.Background(), request.TargetRef, image.RemoveOptions{Force: false, PruneChildren: false})
		}
	}()

	stream, err := cli.ImagePush(ctx, request.TargetRef, image.PushOptions{RegistryAuth: auth})
	if err != nil {
		return RegistryImagePushResult{}, classifyRegistryPushError(err)
	}
	defer logs.CloseAndLogErr(stream)
	output, err := io.ReadAll(io.LimitReader(stream, maxRegistryPushOutput+1))
	if err != nil || len(output) > maxRegistryPushOutput {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
	if hasRegistryPushError(output) {
		return RegistryImagePushResult{}, classifyRegistryPushOutput(output)
	}
	digest := registryDigestPattern.FindString(strings.ToLower(string(output)))
	if digest == "" {
		return RegistryImagePushResult{}, &RegistryPushError{Reason: "REGISTRY_DIGEST_UNAVAILABLE"}
	}

	// 成功后删除候选 tag，最终 tag 仍指向同一 image；Docker daemon 仍可保留缓存层，但不会保留可误用的候选引用。
	_, _ = cli.ImageRemove(ctx, request.CandidateRef, image.RemoveOptions{Force: false, PruneChildren: false})
	targetTagged = false
	return RegistryImagePushResult{ImageRef: request.TargetRef, ImageTag: request.TargetRef, ImageDigest: digest}, nil
}

func hasRegistryPushError(output []byte) bool {
	for _, line := range strings.Split(string(output), "\n") {
		var event struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Error != "" {
			return true
		}
	}
	return false
}

func classifyRegistryPushOutput(output []byte) error {
	return classifyRegistryPushError(errors.New(strings.ToLower(string(output))))
}

func classifyRegistryPushError(err error) error {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unauthorized"), strings.Contains(message, "authentication"), strings.Contains(message, "denied"):
		return &RegistryPushError{Reason: "REGISTRY_AUTH_FAILED"}
	case strings.Contains(message, "x509"), strings.Contains(message, "tls"):
		return &RegistryPushError{Reason: "REGISTRY_TLS_FAILED"}
	default:
		return &RegistryPushError{Reason: "REGISTRY_PUSH_FAILED"}
	}
}
