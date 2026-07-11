package portainer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPlatformDeploymentDesiredSpecAppliesV01Defaults(t *testing.T) {
	spec := NewPlatformDeploymentDesiredSpec()

	require.Equal(t, PlatformRuntimeDriverDockerContainer, spec.Runtime.RuntimeDriver)
	require.Equal(t, 1, spec.Runtime.Replicas)
	require.Equal(t, PlatformReleaseStrategyReplace, spec.Strategy.Type)
	require.Equal(t, PlatformImagePullPolicyIfNotPresent, spec.Image.PullPolicy)
	require.Equal(t, PlatformTraceabilityWeak, spec.Image.Traceability)
	require.Equal(t, PlatformHealthVerificationVerified, spec.HealthCheck.VerificationLevel)
	require.Equal(t, PlatformHealthCheckTypeHTTP, spec.HealthCheck.Type)
	require.Equal(t, "/health", spec.HealthCheck.Path)
	require.Equal(t, PlatformRuntimeRestartPolicyUnlessStopped, spec.Runtime.RestartPolicy)
	require.Equal(t, 10, spec.Runtime.StopTimeoutSeconds)

	require.NoError(t, ValidatePlatformDeploymentDesiredSpecV01(spec))
}

func TestNormalizePlatformDeploymentDesiredSpecAppliesNestedDefaults(t *testing.T) {
	spec := PlatformDeploymentDesiredSpec{
		Ports: []PlatformPortSpec{
			{ContainerPort: 8080, HostPort: 18080},
		},
		EnvOverrides: []PlatformEnvVar{
			{Name: "APP_ENV", Value: "prod"},
		},
	}

	NormalizePlatformDeploymentDesiredSpec(&spec)

	require.Equal(t, PlatformPortProtocolTCP, spec.Ports[0].Protocol)
	require.Equal(t, PlatformPortExposeModePublished, spec.Ports[0].ExposeMode)
	require.Equal(t, PlatformEnvVarSourceLiteral, spec.EnvOverrides[0].Source)
	require.Equal(t, PlatformRuntimeDriverDockerContainer, spec.Runtime.RuntimeDriver)
	require.Equal(t, 1, spec.Runtime.Replicas)
	require.Equal(t, PlatformReleaseStrategyReplace, spec.Strategy.Type)
}

func TestValidatePlatformDeploymentDesiredSpecV01RejectsUnsupportedRuntime(t *testing.T) {
	spec := NewPlatformDeploymentDesiredSpec()
	spec.Runtime.RuntimeDriver = PlatformRuntimeDriverKubernetes

	require.Error(t, ValidatePlatformDeploymentDesiredSpecV01(spec))
}

func TestValidatePlatformDeploymentDesiredSpecV01RejectsUnsupportedReplicas(t *testing.T) {
	spec := NewPlatformDeploymentDesiredSpec()
	spec.Runtime.Replicas = 2

	require.Error(t, ValidatePlatformDeploymentDesiredSpecV01(spec))
}

func TestValidatePlatformDeploymentDesiredSpecV01RejectsUnsupportedStrategy(t *testing.T) {
	spec := NewPlatformDeploymentDesiredSpec()
	spec.Strategy.Type = "blue-green"

	require.Error(t, ValidatePlatformDeploymentDesiredSpecV01(spec))
}
