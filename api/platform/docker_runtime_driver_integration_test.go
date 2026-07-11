package platform

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/crypto"
	"github.com/portainer/portainer/api/datastore"
	dockerclient "github.com/portainer/portainer/api/docker/client"
	"github.com/portainer/portainer/pkg/fips"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	fips.InitFIPS(false)
	os.Exit(m.Run())
}

func TestDockerRuntimeDriverPublishesPublicImageIntegration(t *testing.T) {
	if os.Getenv("PORTAINER_PLATFORM_DOCKER_IT") != "1" {
		t.Skip("set PORTAINER_PLATFORM_DOCKER_IT=1 to run the real Docker runtime driver integration test")
	}

	image := envOrDefault("PORTAINER_PLATFORM_DOCKER_PUBLIC_IMAGE", "nginx:alpine")
	driver, request := newDockerRuntimeIntegration(t, image, nil)

	result := executeDockerRuntimeIntegration(t, driver, request)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, result.Release.Status, "failure=%s health=%+v steps=%+v", result.Release.FailureReason, result.Release.HealthCheckResult, result.Release.Steps)
	require.Equal(t, portainer.PlatformHealthCheckStatusPassed, result.Release.HealthCheckResult.Status)
	require.NotEmpty(t, result.Release.RuntimeSnapshot.CandidateRuntimeRef.ResourceID)
	require.NotEmpty(t, result.Release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID)
	require.NotZero(t, result.Release.RuntimeSnapshot.PublishedPorts[0].HostPort)
}

func TestDockerRuntimeDriverPublishesPrivateImageIntegration(t *testing.T) {
	if os.Getenv("PORTAINER_PLATFORM_DOCKER_IT") != "1" {
		t.Skip("set PORTAINER_PLATFORM_DOCKER_IT=1 to run the real Docker runtime driver integration test")
	}

	image := os.Getenv("PORTAINER_PLATFORM_DOCKER_PRIVATE_IMAGE")
	registryURL := os.Getenv("PORTAINER_PLATFORM_DOCKER_REGISTRY_URL")
	username := os.Getenv("PORTAINER_PLATFORM_DOCKER_REGISTRY_USERNAME")
	password := os.Getenv("PORTAINER_PLATFORM_DOCKER_REGISTRY_PASSWORD")
	if image == "" || registryURL == "" || username == "" || password == "" {
		t.Skip("set private image and registry credential env vars to run private registry integration test")
	}

	registry := &portainer.Registry{
		Type:           portainer.CustomRegistry,
		Name:           "platform-it-registry",
		URL:            registryURL,
		Authentication: true,
		Username:       username,
		Password:       password,
	}
	driver, request := newDockerRuntimeIntegration(t, image, registry)

	result := executeDockerRuntimeIntegration(t, driver, request)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, result.Release.Status, "failure=%s health=%+v steps=%+v", result.Release.FailureReason, result.Release.HealthCheckResult, result.Release.Steps)
	require.Equal(t, portainer.PlatformHealthCheckStatusPassed, result.Release.HealthCheckResult.Status)
	require.NotEmpty(t, result.Release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID)
}

func TestDockerRuntimeDriverHealthFailureIntegration(t *testing.T) {
	if os.Getenv("PORTAINER_PLATFORM_DOCKER_IT") != "1" {
		t.Skip("set PORTAINER_PLATFORM_DOCKER_IT=1 to run the real Docker runtime driver integration test")
	}

	image := envOrDefault("PORTAINER_PLATFORM_DOCKER_PUBLIC_IMAGE", "nginx:alpine")
	driver, request := newDockerRuntimeIntegration(t, image, nil)
	request.Deployment.DesiredSpec.HealthCheck.Path = "/definitely-not-found"
	request.Release.ConfigSnapshot.DesiredSpecSnapshot = request.Deployment.DesiredSpec

	result := executeDockerRuntimeIntegration(t, driver, request)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.Equal(t, ReleaseFailureReasonHealthcheckFailed, result.Release.FailureReason)
	require.NotEmpty(t, result.Release.RuntimeSnapshot.CandidateRuntimeRef.ResourceID)
	require.Empty(t, result.Release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID)
}

func TestDockerRuntimeDriverPortConflictIntegration(t *testing.T) {
	if os.Getenv("PORTAINER_PLATFORM_DOCKER_IT") != "1" {
		t.Skip("set PORTAINER_PLATFORM_DOCKER_IT=1 to run the real Docker runtime driver integration test")
	}

	image := envOrDefault("PORTAINER_PLATFORM_DOCKER_PUBLIC_IMAGE", "nginx:alpine")
	driver, request := newDockerRuntimeIntegration(t, image, nil)
	hostPort := freeTCPPort(t)
	request.Deployment.DesiredSpec.Ports[0].HostPort = hostPort
	request.Release.ConfigSnapshot.DesiredSpecSnapshot = request.Deployment.DesiredSpec
	cleanup := startPortBlockerContainer(t, driver, request, hostPort)
	t.Cleanup(cleanup)

	result := executeDockerRuntimeIntegration(t, driver, request)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, result.Release.Status)
	require.Equal(t, ReleaseFailureReasonSwitchFailed, result.Release.FailureReason)
	require.NotEmpty(t, result.Release.RuntimeSnapshot.CandidateRuntimeRef.ResourceID)
	require.Empty(t, result.Release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID)
}

func newDockerRuntimeIntegration(t *testing.T, image string, registry *portainer.Registry) (*DockerRuntimeDriver, ReleaseExecutionRequest) {
	t.Helper()

	_, store := datastore.MustNewTestStore(t, true, false)
	endpointType := dockerIntegrationEndpointType()
	endpoint := &portainer.Endpoint{
		ID:        1,
		Name:      "platform-it-docker",
		Type:      endpointType,
		URL:       dockerIntegrationEndpointURL(endpointType),
		GroupID:   1,
		Status:    portainer.EndpointStatusUp,
		TLSConfig: dockerIntegrationEndpointTLSConfig(endpointType),
	}
	require.NoError(t, store.Endpoint().Create(endpoint))

	var registryID portainer.RegistryID
	if registry != nil {
		require.NoError(t, store.Registry().Create(registry))
		registryID = registry.ID
	}

	factory := dockerclient.NewClientFactory(dockerIntegrationSignatureService(t, endpointType), nil)
	driver := NewDockerRuntimeDriver(store, factory)
	request := sampleReleaseExecutionRequest()
	request.Project.Slug = "it-project"
	request.Environment.Slug = "it-env"
	request.Environment.HealthCheckHost = envOrDefault("PORTAINER_PLATFORM_DOCKER_HEALTH_HOST", "127.0.0.1")
	request.Environment.Targets = []portainer.PlatformDeploymentTarget{{
		EndpointID:  endpoint.ID,
		Role:        portainer.PlatformDeploymentTargetRoleWorkload,
		HostAddress: request.Environment.HealthCheckHost,
		Enabled:     true,
	}}
	request.ServiceDefinition.Slug = "nginx"
	request.Deployment.CurrentRuntimeRef = portainer.RuntimeRef{}
	request.Deployment.DesiredSpec.Image.Image = image
	request.Deployment.DesiredSpec.Image.RegistryID = registryID
	request.Deployment.DesiredSpec.Ports = []portainer.PlatformPortSpec{{
		Name:          "http",
		ContainerPort: 80,
		Protocol:      portainer.PlatformPortProtocolTCP,
		ExposeMode:    portainer.PlatformPortExposeModePublished,
	}}
	request.Deployment.DesiredSpec.HealthCheck.Path = "/"
	request.Deployment.DesiredSpec.HealthCheck.Port = 80
	request.Deployment.DesiredSpec.HealthCheck.Retries = 10
	request.Deployment.DesiredSpec.HealthCheck.IntervalSeconds = 1
	request.Artifact.ImageRef = image
	request.Artifact.RegistryID = registryID
	request.Release.ID = portainer.PlatformReleaseID(time.Now().UnixNano() % 1000000000)
	request.Release.Image = image
	request.Release.ArtifactSnapshot.ImageRef = image
	request.Release.ArtifactSnapshot.RegistryID = registryID
	applyDockerIntegrationSpecOverrides(&request)

	return driver, request
}

func executeDockerRuntimeIntegration(t *testing.T, driver *DockerRuntimeDriver, request ReleaseExecutionRequest) ReleaseExecutionResult {
	t.Helper()

	result, err := NewSingleTargetExecutor(driver).Execute(context.Background(), request)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = driver.DeleteRuntime(context.Background(), result.Release.RuntimeSnapshot.CandidateRuntimeRef)
		_ = driver.DeleteRuntime(context.Background(), result.Release.RuntimeSnapshot.CurrentRuntimeRef)
	})

	return result
}

func dockerIntegrationEndpointType() portainer.EndpointType {
	switch strings.ToLower(os.Getenv("PORTAINER_PLATFORM_DOCKER_ENDPOINT_TYPE")) {
	case "agent", "agent-docker", "agent_on_docker":
		return portainer.AgentOnDockerEnvironment
	default:
		return portainer.DockerEnvironment
	}
}

func dockerIntegrationEndpointURL(endpointType portainer.EndpointType) string {
	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_ENDPOINT_URL"); value != "" {
		return value
	}
	if endpointType == portainer.AgentOnDockerEnvironment {
		return "127.0.0.1:9001"
	}
	if runtime.GOOS == "windows" {
		return "npipe:////./pipe/docker_engine"
	}

	return "unix:///var/run/docker.sock"
}

func dockerIntegrationEndpointTLSConfig(endpointType portainer.EndpointType) portainer.TLSConfiguration {
	tlsConfig := portainer.TLSConfiguration{}
	if endpointType == portainer.AgentOnDockerEnvironment {
		tlsConfig.TLS = true
		tlsConfig.TLSSkipVerify = true
	}
	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_ENDPOINT_TLS"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err == nil {
			tlsConfig.TLS = enabled
		}
	}
	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_ENDPOINT_TLS_SKIP_VERIFY"); value != "" {
		skip, err := strconv.ParseBool(value)
		if err == nil {
			tlsConfig.TLSSkipVerify = skip
		}
	}

	return tlsConfig
}

func dockerIntegrationSignatureService(t *testing.T, endpointType portainer.EndpointType) portainer.DigitalSignatureService {
	t.Helper()

	if endpointType != portainer.AgentOnDockerEnvironment {
		return nil
	}

	secret := os.Getenv("PORTAINER_PLATFORM_DOCKER_AGENT_SECRET")
	if secret == "" && os.Getenv("PORTAINER_PLATFORM_DOCKER_AGENT_ALLOW_EPHEMERAL_KEY") != "1" {
		t.Skip("set PORTAINER_PLATFORM_DOCKER_AGENT_SECRET, or PORTAINER_PLATFORM_DOCKER_AGENT_ALLOW_EPHEMERAL_KEY=1 for a disposable Agent pairing test")
	}

	signatureService := crypto.NewECDSAService(secret)
	_, _, err := signatureService.GenerateKeyPair()
	require.NoError(t, err)

	return signatureService
}

func envOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func applyDockerIntegrationSpecOverrides(request *ReleaseExecutionRequest) {
	if request == nil {
		return
	}

	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_HEALTH_VERIFICATION"); value != "" {
		request.Deployment.DesiredSpec.HealthCheck.VerificationLevel = portainer.PlatformHealthVerification(value)
	}
	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_HEALTH_TYPE"); value != "" {
		request.Deployment.DesiredSpec.HealthCheck.Type = portainer.PlatformHealthCheckType(value)
	}
	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_HOST_PORT"); value != "" && len(request.Deployment.DesiredSpec.Ports) > 0 {
		hostPort, err := strconv.Atoi(value)
		if err == nil {
			request.Deployment.DesiredSpec.Ports[0].HostPort = hostPort
		}
	}

	request.Release.ConfigSnapshot.DesiredSpecSnapshot = request.Deployment.DesiredSpec
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func startPortBlockerContainer(t *testing.T, driver *DockerRuntimeDriver, request ReleaseExecutionRequest, hostPort int) func() {
	t.Helper()

	ctx := context.Background()
	cli, err := driver.clientForTarget(request.Environment.Targets[0])
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })

	port := nat.Port("80/tcp")
	name := fmt.Sprintf("pcn-it-port-blocker-%d", hostPort)
	_ = cli.ContainerRemove(ctx, name, dockercontainer.RemoveOptions{Force: true})
	created, err := cli.ContainerCreate(ctx, &dockercontainer.Config{
		Image:        request.Artifact.ImageRef,
		ExposedPorts: nat.PortSet{port: struct{}{}},
		Labels:       map[string]string{platformLabelPrefix + ".managed": "true"},
	}, &dockercontainer.HostConfig{
		PortBindings: nat.PortMap{port: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(hostPort)}}},
	}, &network.NetworkingConfig{}, nil, name)
	require.NoError(t, err)
	require.NoError(t, cli.ContainerStart(ctx, created.ID, dockercontainer.StartOptions{}))

	return func() {
		_ = cli.ContainerRemove(ctx, created.ID, dockercontainer.RemoveOptions{Force: true})
	}
}
