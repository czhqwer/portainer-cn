package platform

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/api/docker/images"
	"github.com/portainer/portainer/api/logs"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

const (
	platformLabelPrefix = "io.portainer.platform"
)

type dockerClientFactory interface {
	CreateClient(endpoint *portainer.Endpoint, nodeName string, timeout *time.Duration) (*client.Client, error)
}

type DockerRuntimeDriver struct {
	dataStore     dataservices.DataStore
	clientFactory dockerClientFactory
	httpClient    *http.Client
}

func NewDockerRuntimeDriver(dataStore dataservices.DataStore, clientFactory *dockerclient.ClientFactory) *DockerRuntimeDriver {
	return &DockerRuntimeDriver{
		dataStore:     dataStore,
		clientFactory: clientFactory,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (driver *DockerRuntimeDriver) PullImage(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget) error {
	cli, err := driver.clientForTarget(target)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)

	imageRef := request.Artifact.ImageRef
	img, err := images.ParseImage(images.ParseImageOptions{Name: imageRef})
	if err != nil {
		return errors.Wrapf(err, "parse image %s", imageRef)
	}

	if request.Deployment.DesiredSpec.Image.PullPolicy != portainer.PlatformImagePullPolicyAlways {
		if _, err := cli.ImageInspect(ctx, img.FullName()); err == nil {
			return nil
		} else if !errdefs.IsNotFound(err) {
			return err
		}
	}

	puller := images.NewPuller(cli, images.NewRegistryClient(driver.dataStore), driver.dataStore)
	return puller.Pull(ctx, img)
}

func (driver *DockerRuntimeDriver) StartCandidate(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget) (portainer.RuntimeRef, []portainer.PlatformPublishedPort, error) {
	return driver.createAndStartContainer(ctx, request, target, true)
}

func (driver *DockerRuntimeDriver) ValidateRuntime(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, runtimeRef portainer.RuntimeRef, ports []portainer.PlatformPublishedPort) (result portainer.PlatformHealthCheckResult, err error) {
	now := time.Now().Unix()
	result = portainer.PlatformHealthCheckResult{
		Level:     request.Deployment.DesiredSpec.HealthCheck.VerificationLevel,
		Type:      request.Deployment.DesiredSpec.HealthCheck.Type,
		Status:    portainer.PlatformHealthCheckStatusSkipped,
		StartedAt: now,
	}
	defer func() {
		result.FinishedAt = time.Now().Unix()
	}()

	switch request.Deployment.DesiredSpec.HealthCheck.VerificationLevel {
	case portainer.PlatformHealthVerificationUnverified:
		result.Status = portainer.PlatformHealthCheckStatusSkipped
		result.LogSummary = "Health verification is unverified."
		return result, nil
	case portainer.PlatformHealthVerificationStartupOnly:
		result.Status = portainer.PlatformHealthCheckStatusPassed
		result.LogSummary = "Startup-only verification passed after container start."
		return result, nil
	}

	switch request.Deployment.DesiredSpec.HealthCheck.Type {
	case portainer.PlatformHealthCheckTypeNone:
		result.Status = portainer.PlatformHealthCheckStatusSkipped
		result.LogSummary = "Health check type is none."
		return result, nil
	case portainer.PlatformHealthCheckTypeStartup:
		return driver.validateContainerRunning(ctx, runtimeRef, result)
	case portainer.PlatformHealthCheckTypeTCP:
		return driver.validateTCP(ctx, request, target, ports, result)
	default:
		return driver.validateHTTP(ctx, request, target, ports, result)
	}
}

func (driver *DockerRuntimeDriver) DeleteRuntime(ctx context.Context, runtimeRef portainer.RuntimeRef) error {
	if runtimeRef.ResourceID == "" {
		return nil
	}

	cli, err := driver.clientForRuntime(runtimeRef)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)

	timeout := 5
	if err := cli.ContainerStop(ctx, runtimeRef.ResourceID, dockercontainer.StopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err := cli.ContainerRemove(ctx, runtimeRef.ResourceID, dockercontainer.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	return nil
}

func (driver *DockerRuntimeDriver) Switch(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, _ CandidateValidationSnapshot) (RuntimeSwitchResult, error) {
	previous := request.Deployment.CurrentRuntimeRef
	if previous.ResourceID != "" {
		if err := driver.stopRuntime(ctx, previous, request.Deployment.DesiredSpec.Runtime.StopTimeoutSeconds); err != nil {
			return RuntimeSwitchResult{}, errors.Wrap(err, "stop previous runtime")
		}
	}

	currentRef, publishedPorts, err := driver.createAndStartContainer(ctx, request, target, false)
	if err != nil {
		return RuntimeSwitchResult{}, err
	}

	result := RuntimeSwitchResult{
		CurrentRuntimeRef: currentRef,
		PublishedPorts:    publishedPorts,
	}
	if previous.ResourceID != "" {
		result.RetainedRuntimeRefs = []portainer.RuntimeRef{previous}
	}

	return result, nil
}

func (driver *DockerRuntimeDriver) Recover(ctx context.Context, request ReleaseExecutionRequest, _ portainer.PlatformDeploymentTarget) error {
	previous := request.Deployment.CurrentRuntimeRef
	if previous.ResourceID == "" {
		return nil
	}

	cli, err := driver.clientForRuntime(previous)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)

	return cli.ContainerStart(ctx, previous.ResourceID, dockercontainer.StartOptions{})
}

func (driver *DockerRuntimeDriver) createAndStartContainer(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, candidate bool) (portainer.RuntimeRef, []portainer.PlatformPublishedPort, error) {
	cli, err := driver.clientForTarget(target)
	if err != nil {
		return portainer.RuntimeRef{}, nil, err
	}
	defer logs.CloseAndLogErr(cli)

	name := dockerContainerName(request, candidate)
	config, hostConfig, networkingConfig, err := dockerContainerCreateOptions(request, target, candidate)
	if err != nil {
		return portainer.RuntimeRef{}, nil, err
	}

	createResponse, err := cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, name)
	if err != nil {
		return portainer.RuntimeRef{}, nil, err
	}
	if err := cli.ContainerStart(ctx, createResponse.ID, dockercontainer.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(ctx, createResponse.ID, dockercontainer.RemoveOptions{Force: true})
		return portainer.RuntimeRef{}, nil, err
	}

	inspect, err := inspectContainerWithPublishedPorts(ctx, cli, createResponse.ID, request.Deployment.DesiredSpec.Ports)
	if err != nil {
		return portainer.RuntimeRef{}, nil, err
	}

	ref := portainer.RuntimeRef{
		DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
		EndpointID:   target.EndpointID,
		ResourceType: portainer.PlatformRuntimeResourceContainer,
		ResourceID:   createResponse.ID,
		Name:         strings.TrimPrefix(inspect.Name, "/"),
		Labels:       config.Labels,
	}

	return ref, publishedPortsFromInspect(inspect.NetworkSettings.Ports, request.Deployment.DesiredSpec.Ports), nil
}

func inspectContainerWithPublishedPorts(ctx context.Context, cli *client.Client, containerID string, specs []portainer.PlatformPortSpec) (dockercontainer.InspectResponse, error) {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return inspect, err
	}
	if len(specs) == 0 || len(publishedPortsFromInspect(inspect.NetworkSettings.Ports, specs)) > 0 {
		return inspect, nil
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return inspect, ctx.Err()
		case <-timeout.C:
			return inspect, nil
		case <-ticker.C:
			inspect, err = cli.ContainerInspect(ctx, containerID)
			if err != nil {
				return inspect, err
			}
			if len(publishedPortsFromInspect(inspect.NetworkSettings.Ports, specs)) > 0 {
				return inspect, nil
			}
		}
	}
}

func (driver *DockerRuntimeDriver) stopRuntime(ctx context.Context, runtimeRef portainer.RuntimeRef, timeoutSeconds int) error {
	if runtimeRef.ResourceID == "" {
		return nil
	}

	cli, err := driver.clientForRuntime(runtimeRef)
	if err != nil {
		return err
	}
	defer logs.CloseAndLogErr(cli)

	timeout := timeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}

	if err := cli.ContainerStop(ctx, runtimeRef.ResourceID, dockercontainer.StopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	return nil
}

func (driver *DockerRuntimeDriver) validateContainerRunning(ctx context.Context, runtimeRef portainer.RuntimeRef, result portainer.PlatformHealthCheckResult) (portainer.PlatformHealthCheckResult, error) {
	cli, err := driver.clientForRuntime(runtimeRef)
	if err != nil {
		result.Status = portainer.PlatformHealthCheckStatusFailed
		result.ErrorMessage = err.Error()
		return result, err
	}
	defer logs.CloseAndLogErr(cli)

	inspect, err := cli.ContainerInspect(ctx, runtimeRef.ResourceID)
	if err != nil {
		result.Status = portainer.PlatformHealthCheckStatusFailed
		result.ErrorMessage = err.Error()
		return result, err
	}
	if inspect.State != nil && inspect.State.Running {
		result.Status = portainer.PlatformHealthCheckStatusPassed
		result.LogSummary = "Container is running."
		return result, nil
	}

	result.Status = portainer.PlatformHealthCheckStatusFailed
	result.ErrorMessage = "container is not running"
	return result, codedRuntimeError{reason: ReleaseFailureReasonCandidateHealthFailed, message: result.ErrorMessage}
}

func (driver *DockerRuntimeDriver) validateHTTP(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, ports []portainer.PlatformPublishedPort, result portainer.PlatformHealthCheckResult) (portainer.PlatformHealthCheckResult, error) {
	hostPort, err := healthHostPort(request.Deployment.DesiredSpec.HealthCheck.Port, ports)
	if err != nil {
		result.Status = portainer.PlatformHealthCheckStatusFailed
		result.ErrorMessage = err.Error()
		return result, err
	}

	healthURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(healthCheckHost(request.Environment, target), strconv.Itoa(hostPort.HostPort)),
		Path:   request.Deployment.DesiredSpec.HealthCheck.Path,
	}
	result.Target = healthURL.String()

	retries := request.Deployment.DesiredSpec.HealthCheck.Retries
	if retries <= 0 {
		retries = 1
	}

	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, result.Target, nil)
		if err != nil {
			result.Status = portainer.PlatformHealthCheckStatusFailed
			result.ErrorMessage = err.Error()
			return result, err
		}

		resp, err := driver.httpClient.Do(req)
		if err == nil {
			result.StatusCode = resp.StatusCode
			_ = resp.Body.Close()
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
				result.Status = portainer.PlatformHealthCheckStatusPassed
				result.LogSummary = "HTTP health check passed."
				return result, nil
			}
			lastErr = fmt.Errorf("HTTP health check returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		sleepBetweenHealthAttempts(ctx, request.Deployment.DesiredSpec.HealthCheck.IntervalSeconds)
	}

	result.Status = portainer.PlatformHealthCheckStatusFailed
	result.ErrorMessage = lastErr.Error()

	return result, codedRuntimeError{reason: ReleaseFailureReasonHealthcheckHostUnreachable, message: result.ErrorMessage}
}

func (driver *DockerRuntimeDriver) validateTCP(ctx context.Context, request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, ports []portainer.PlatformPublishedPort, result portainer.PlatformHealthCheckResult) (portainer.PlatformHealthCheckResult, error) {
	hostPort, err := healthHostPort(request.Deployment.DesiredSpec.HealthCheck.Port, ports)
	if err != nil {
		result.Status = portainer.PlatformHealthCheckStatusFailed
		result.ErrorMessage = err.Error()
		return result, err
	}

	result.Target = net.JoinHostPort(healthCheckHost(request.Environment, target), strconv.Itoa(hostPort.HostPort))
	retries := request.Deployment.DesiredSpec.HealthCheck.Retries
	if retries <= 0 {
		retries = 1
	}

	timeout := time.Duration(request.Deployment.DesiredSpec.HealthCheck.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		dialer := net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(ctx, "tcp", result.Target)
		if err == nil {
			_ = conn.Close()
			result.Status = portainer.PlatformHealthCheckStatusPassed
			result.LogSummary = "TCP health check passed."
			return result, nil
		}
		lastErr = err
		sleepBetweenHealthAttempts(ctx, request.Deployment.DesiredSpec.HealthCheck.IntervalSeconds)
	}

	result.Status = portainer.PlatformHealthCheckStatusFailed
	result.ErrorMessage = lastErr.Error()

	return result, codedRuntimeError{reason: ReleaseFailureReasonHealthcheckHostUnreachable, message: result.ErrorMessage}
}

func (driver *DockerRuntimeDriver) clientForTarget(target portainer.PlatformDeploymentTarget) (*client.Client, error) {
	if driver == nil || driver.clientFactory == nil || driver.dataStore == nil {
		return nil, codedRuntimeError{reason: ReleaseFailureReasonExecutorUnavailable, message: "Docker runtime driver is not configured."}
	}

	endpoint, err := driver.dataStore.Endpoint().Endpoint(target.EndpointID)
	if err != nil {
		return nil, err
	}

	return driver.clientFactory.CreateClient(endpoint, target.NodeName, nil)
}

func (driver *DockerRuntimeDriver) clientForRuntime(runtimeRef portainer.RuntimeRef) (*client.Client, error) {
	if driver == nil || driver.clientFactory == nil || driver.dataStore == nil {
		return nil, codedRuntimeError{reason: ReleaseFailureReasonExecutorUnavailable, message: "Docker runtime driver is not configured."}
	}

	endpoint, err := driver.dataStore.Endpoint().Endpoint(runtimeRef.EndpointID)
	if err != nil {
		return nil, err
	}

	nodeName := ""
	if runtimeRef.Labels != nil {
		nodeName = runtimeRef.Labels[platformLabelPrefix+".node-name"]
	}

	return driver.clientFactory.CreateClient(endpoint, nodeName, nil)
}

func dockerContainerCreateOptions(request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, candidate bool) (*dockercontainer.Config, *dockercontainer.HostConfig, *network.NetworkingConfig, error) {
	ports, err := dockerPortBindings(request.Deployment.DesiredSpec.Ports, candidate)
	if err != nil {
		return nil, nil, nil, err
	}

	stopTimeout := request.Deployment.DesiredSpec.Runtime.StopTimeoutSeconds
	if stopTimeout <= 0 {
		stopTimeout = 10
	}

	config := &dockercontainer.Config{
		Image:        request.Artifact.ImageRef,
		Env:          dockerEnv(request.Deployment.DesiredSpec.EnvOverrides),
		ExposedPorts: ports.exposed,
		Labels:       dockerLabels(request, target, candidate),
		StopTimeout:  &stopTimeout,
	}

	restartPolicy := dockercontainer.RestartPolicyDisabled
	if !candidate {
		restartPolicy = dockerRestartPolicy(request.Deployment.DesiredSpec.Runtime.RestartPolicy)
	}

	hostConfig := &dockercontainer.HostConfig{
		PortBindings:    ports.bindings,
		PublishAllPorts: ports.publishAll,
		RestartPolicy: dockercontainer.RestartPolicy{
			Name: restartPolicy,
		},
	}

	return config, hostConfig, &network.NetworkingConfig{}, nil
}

type dockerPorts struct {
	exposed    nat.PortSet
	bindings   nat.PortMap
	publishAll bool
}

func dockerPortBindings(specs []portainer.PlatformPortSpec, candidate bool) (dockerPorts, error) {
	result := dockerPorts{
		exposed:  nat.PortSet{},
		bindings: nat.PortMap{},
	}

	for _, spec := range specs {
		if spec.ContainerPort <= 0 {
			return dockerPorts{}, fmt.Errorf("container port is required")
		}
		if spec.ExposeMode != "" && spec.ExposeMode != portainer.PlatformPortExposeModePublished {
			continue
		}

		protocol := string(spec.Protocol)
		if protocol == "" {
			protocol = string(portainer.PlatformPortProtocolTCP)
		}
		port := nat.Port(fmt.Sprintf("%d/%s", spec.ContainerPort, protocol))
		result.exposed[port] = struct{}{}

		hostPort := ""
		if !candidate && spec.HostPort > 0 {
			hostPort = strconv.Itoa(spec.HostPort)
		}
		if hostPort == "" {
			result.publishAll = true
		}
		result.bindings[port] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: hostPort}}
	}

	return result, nil
}

func dockerEnv(vars []portainer.PlatformEnvVar) []string {
	env := make([]string, 0, len(vars))
	for _, variable := range vars {
		if variable.Source != "" && variable.Source != portainer.PlatformEnvVarSourceLiteral {
			continue
		}
		if variable.Name == "" {
			continue
		}
		// V0.1 只把字面量环境变量写入容器；配置中心和密钥引用仍保留在控制面快照，
		// 避免在执行器首版中误把未解析的敏感引用展开到 Docker 环境变量。
		env = append(env, variable.Name+"="+variable.Value)
	}

	return env
}

func dockerLabels(request ReleaseExecutionRequest, target portainer.PlatformDeploymentTarget, candidate bool) map[string]string {
	return map[string]string{
		platformLabelPrefix + ".project-id":            strconv.Itoa(int(request.Project.ID)),
		platformLabelPrefix + ".environment-id":        strconv.Itoa(int(request.Environment.ID)),
		platformLabelPrefix + ".application-id":        strconv.Itoa(int(request.Application.ID)),
		platformLabelPrefix + ".service-definition-id": strconv.Itoa(int(request.ServiceDefinition.ID)),
		platformLabelPrefix + ".service-deployment-id": strconv.Itoa(int(request.Deployment.ID)),
		platformLabelPrefix + ".release-id":            strconv.Itoa(int(request.Release.ID)),
		platformLabelPrefix + ".artifact-id":           strconv.Itoa(int(request.Artifact.ID)),
		platformLabelPrefix + ".node-name":             target.NodeName,
		platformLabelPrefix + ".candidate":             strconv.FormatBool(candidate),
		platformLabelPrefix + ".managed":               "true",
	}
}

func dockerRestartPolicy(policy portainer.PlatformRuntimeRestartPolicy) dockercontainer.RestartPolicyMode {
	switch policy {
	case portainer.PlatformRuntimeRestartPolicyAlways:
		return dockercontainer.RestartPolicyAlways
	case portainer.PlatformRuntimeRestartPolicyOnFailure:
		return dockercontainer.RestartPolicyOnFailure
	case portainer.PlatformRuntimeRestartPolicyNo:
		return dockercontainer.RestartPolicyDisabled
	default:
		return dockercontainer.RestartPolicyUnlessStopped
	}
}

func dockerContainerName(request ReleaseExecutionRequest, candidate bool) string {
	name := fmt.Sprintf(
		"pcn-%s-%s-%s-r%d",
		normalizeDockerNamePart(request.Project.Slug),
		normalizeDockerNamePart(request.Environment.Slug),
		normalizeDockerNamePart(request.ServiceDefinition.Slug),
		request.Release.ID,
	)
	if candidate {
		name += "-candidate"
	}

	if len(name) > 128 {
		suffix := fmt.Sprintf("-r%d", request.Release.ID)
		if candidate {
			suffix += "-candidate"
		}
		name = name[:128-len(suffix)] + suffix
	}

	return name
}

func normalizeDockerNamePart(value string) string {
	value = strings.Trim(strings.ToLower(value), "-")
	if value == "" {
		return "unnamed"
	}

	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('-')
	}

	return strings.Trim(builder.String(), "-")
}

func publishedPortsFromInspect(portMap nat.PortMap, specs []portainer.PlatformPortSpec) []portainer.PlatformPublishedPort {
	result := make([]portainer.PlatformPublishedPort, 0)
	namesByPort := map[int]string{}
	for _, spec := range specs {
		namesByPort[spec.ContainerPort] = spec.Name
	}

	for port, bindings := range portMap {
		containerPort, _ := strconv.Atoi(port.Port())
		for _, binding := range bindings {
			hostPort, _ := strconv.Atoi(binding.HostPort)
			result = append(result, portainer.PlatformPublishedPort{
				Name:          namesByPort[containerPort],
				ContainerPort: containerPort,
				HostPort:      hostPort,
				Protocol:      portainer.PlatformPortProtocol(port.Proto()),
				HostIP:        binding.HostIP,
			})
		}
	}

	return result
}

func healthHostPort(healthPort int, ports []portainer.PlatformPublishedPort) (portainer.PlatformPublishedPort, error) {
	if len(ports) == 0 {
		return portainer.PlatformPublishedPort{}, fmt.Errorf("no published ports available for health check")
	}
	if healthPort <= 0 {
		return ports[0], nil
	}

	for _, port := range ports {
		if port.ContainerPort == healthPort && port.HostPort > 0 {
			return port, nil
		}
	}

	return portainer.PlatformPublishedPort{}, fmt.Errorf("health check port %d is not published", healthPort)
}

func healthCheckHost(environment portainer.PlatformEnvironment, target portainer.PlatformDeploymentTarget) string {
	host := strings.TrimSpace(environment.HealthCheckHost)
	if host == "" {
		host = strings.TrimSpace(target.HostAddress)
	}
	if host == "" {
		host = "127.0.0.1"
	}

	if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}

	return host
}

func sleepBetweenHealthAttempts(ctx context.Context, seconds int) {
	if seconds <= 0 {
		seconds = 1
	}

	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
