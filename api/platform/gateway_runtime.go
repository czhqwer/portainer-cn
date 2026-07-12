package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/api/logs"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	platformGatewayLabel = platformLabelPrefix + ".gateway-id"
	platformGatewayImage = "nginx:1.27-alpine"
)

// GatewayRuntime 只暴露固定的 Nginx 校验和 reload 动作，避免 handler 把用户字符串变成容器 exec 命令。
type GatewayRuntime interface {
	Ensure(ctx context.Context, gateway portainer.PlatformGateway, datastorePath string) (string, error)
	Test(ctx context.Context, gateway portainer.PlatformGateway, candidateHash string) error
	Reload(ctx context.Context, gateway portainer.PlatformGateway) error
}

type DockerGatewayRuntime struct {
	dataStore     dataservices.DataStore
	clientFactory dockerClientFactory
}

func NewDockerGatewayRuntime(dataStore dataservices.DataStore, factory *dockerclient.ClientFactory) *DockerGatewayRuntime {
	return &DockerGatewayRuntime{dataStore: dataStore, clientFactory: factory}
}

// Ensure 只会创建带平台标签的固定 Nginx 镜像，并将平台受控目录以只读方式挂载。
// 它不接收容器 ID、镜像、命令或宿主机路径等用户输入，避免网关管理接口变成通用 Docker 执行入口。
func (runtime *DockerGatewayRuntime) Ensure(ctx context.Context, gateway portainer.PlatformGateway, datastorePath string) (string, error) {
	if runtime == nil || runtime.dataStore == nil || runtime.clientFactory == nil || gateway.ID <= 0 || gateway.EndpointID <= 0 || datastorePath == "" {
		return "", errors.New("gateway runtime is not configured")
	}
	endpoint, err := runtime.dataStore.Endpoint().Endpoint(gateway.EndpointID)
	if err != nil {
		return "", err
	}
	cli, err := runtime.clientFactory.CreateClient(endpoint, gateway.NodeName, nil)
	if err != nil {
		return "", err
	}
	defer logs.CloseAndLogErr(cli)
	if gateway.ManagedContainerID != "" {
		inspect, err := cli.ContainerInspect(ctx, gateway.ManagedContainerID)
		if err == nil && isManagedGatewayContainer(inspect.Config.Labels, gateway.ID) {
			return gateway.ManagedContainerID, nil
		}
		if err == nil {
			return "", errors.New("gateway container is not platform managed")
		}
	}

	root := filepath.Join(datastorePath, "platform-gateways")
	activeConfig := filepath.Join(root, fmt.Sprintf("%d", gateway.ID), gatewayConfigFileName)
	config := &dockercontainer.Config{
		Image: platformGatewayImage,
		Labels: map[string]string{
			platformLabelPrefix + ".managed": "true",
			platformGatewayLabel:             strconv.Itoa(int(gateway.ID)),
		},
		ExposedPorts: nat.PortSet{nat.Port("80/tcp"): struct{}{}, nat.Port("443/tcp"): struct{}{}},
	}
	hostConfig := &dockercontainer.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port("80/tcp"):  []nat.PortBinding{{HostPort: "80"}},
			nat.Port("443/tcp"): []nat.PortBinding{{HostPort: "443"}},
		},
		RestartPolicy: dockercontainer.RestartPolicy{Name: dockercontainer.RestartPolicyUnlessStopped},
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: root, Target: "/etc/nginx/portainer", ReadOnly: true},
			{Type: mount.TypeBind, Source: activeConfig, Target: "/etc/nginx/nginx.conf", ReadOnly: true},
		},
	}
	name := fmt.Sprintf("portainer-platform-gateway-%d", gateway.ID)
	created, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		return "", err
	}
	if err := cli.ContainerStart(ctx, created.ID, dockercontainer.StartOptions{}); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (runtime *DockerGatewayRuntime) Test(ctx context.Context, gateway portainer.PlatformGateway, candidateHash string) error {
	if !isGatewayConfigHash(candidateHash) || gateway.ID <= 0 {
		return errors.New("gateway candidate config is invalid")
	}
	candidatePath := fmt.Sprintf("/etc/nginx/portainer/%d/versions/%s.conf", gateway.ID, candidateHash)
	return runtime.exec(ctx, gateway, []string{"nginx", "-t", "-c", candidatePath})
}

func (runtime *DockerGatewayRuntime) Reload(ctx context.Context, gateway portainer.PlatformGateway) error {
	return runtime.exec(ctx, gateway, []string{"nginx", "-s", "reload"})
}

// exec 在执行前确认容器带有网关 ID 和 platform managed 标签，
// 防止受控网关 API 被用于向任意容器执行 nginx 命令。
func (runtime *DockerGatewayRuntime) exec(ctx context.Context, gateway portainer.PlatformGateway, command []string) error {
	if runtime == nil || runtime.dataStore == nil || runtime.clientFactory == nil || gateway.EndpointID <= 0 || gateway.ManagedContainerID == "" {
		return errors.New("gateway runtime is not configured")
	}
	endpoint, err := runtime.dataStore.Endpoint().Endpoint(gateway.EndpointID)
	if err != nil {
		return err
	}
	cli, err := runtime.clientFactory.CreateClient(endpoint, gateway.NodeName, nil)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)
	inspect, err := cli.ContainerInspect(ctx, gateway.ManagedContainerID)
	if err != nil {
		return err
	}
	if !isManagedGatewayContainer(inspect.Config.Labels, gateway.ID) {
		return errors.New("gateway container is not platform managed")
	}
	return runGatewayExec(ctx, cli, gateway.ManagedContainerID, command)
}

func isManagedGatewayContainer(labels map[string]string, gatewayID portainer.PlatformGatewayID) bool {
	return labels != nil && labels[platformLabelPrefix+".managed"] == "true" && labels[platformGatewayLabel] == strconv.Itoa(int(gatewayID))
}

func runGatewayExec(ctx context.Context, cli *client.Client, containerID string, command []string) error {
	exec, err := cli.ContainerExecCreate(ctx, containerID, dockercontainer.ExecOptions{AttachStdout: true, AttachStderr: true, Cmd: command})
	if err != nil {
		return err
	}
	attach, err := cli.ContainerExecAttach(ctx, exec.ID, dockercontainer.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attach.Close()
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	result, err := cli.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("gateway nginx command failed: %d", result.ExitCode)
	}
	return nil
}
