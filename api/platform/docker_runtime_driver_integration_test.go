package platform

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/datastore"
	dockerclient "github.com/portainer/portainer/api/docker/client"

	"github.com/stretchr/testify/require"
)

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

func newDockerRuntimeIntegration(t *testing.T, image string, registry *portainer.Registry) (*DockerRuntimeDriver, ReleaseExecutionRequest) {
	t.Helper()

	_, store := datastore.MustNewTestStore(t, true, false)
	endpoint := &portainer.Endpoint{
		ID:      1,
		Name:    "platform-it-docker",
		Type:    portainer.DockerEnvironment,
		URL:     dockerIntegrationEndpointURL(),
		GroupID: 1,
		Status:  portainer.EndpointStatusUp,
	}
	require.NoError(t, store.Endpoint().Create(endpoint))

	var registryID portainer.RegistryID
	if registry != nil {
		require.NoError(t, store.Registry().Create(registry))
		registryID = registry.ID
	}

	factory := dockerclient.NewClientFactory(nil, nil)
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
	request.Release.ConfigSnapshot.DesiredSpecSnapshot = request.Deployment.DesiredSpec

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

func dockerIntegrationEndpointURL() string {
	if value := os.Getenv("PORTAINER_PLATFORM_DOCKER_ENDPOINT_URL"); value != "" {
		return value
	}
	if runtime.GOOS == "windows" {
		return "npipe:////./pipe/docker_engine"
	}

	return "unix:///var/run/docker.sock"
}

func envOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
