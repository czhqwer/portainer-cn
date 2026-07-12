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

func TestNewPlatformConfigSetAppliesDefaults(t *testing.T) {
	configSet := NewPlatformConfigSet()

	require.Equal(t, PlatformConfigSetDefaultName, configSet.Name)
	require.Equal(t, 1, configSet.Revision)
	require.Equal(t, PlatformLifecycleStatusActive, configSet.LifecycleStatus)
	require.Equal(t, 1, configSet.ResourceVersion)
}

func TestNormalizePlatformConfigSetDerivesEntrySource(t *testing.T) {
	configSet := PlatformConfigSet{
		ProjectID: 1,
		ScopeType: PlatformConfigScopeEnvironment,
		ScopeID:   2,
		Entries: []PlatformConfigEntry{
			{Key: "APP_ENV"},
		},
	}

	NormalizePlatformConfigSet(&configSet)

	require.Equal(t, PlatformConfigSetDefaultName, configSet.Name)
	require.Equal(t, 1, configSet.Revision)
	require.Equal(t, PlatformConfigValuePlain, configSet.Entries[0].ValueType)
	require.Equal(t, PlatformConfigEntrySourceEnvironment, configSet.Entries[0].Source)
}

func TestValidatePlatformConfigSetRejectsUnsafeOrInvalidEntries(t *testing.T) {
	base := NewPlatformConfigSet()
	base.ProjectID = 1
	base.ScopeType = PlatformConfigScopeProject
	base.ScopeID = 1

	t.Run("invalid key", func(t *testing.T) {
		configSet := base
		configSet.Entries = []PlatformConfigEntry{{Key: "app-env"}}

		require.Error(t, ValidatePlatformConfigSet(configSet))
	})

	t.Run("duplicate key", func(t *testing.T) {
		configSet := base
		configSet.Entries = []PlatformConfigEntry{{Key: "APP_ENV"}, {Key: "APP_ENV"}}

		require.Error(t, ValidatePlatformConfigSet(configSet))
	})

	t.Run("sensitive plain value", func(t *testing.T) {
		configSet := base
		configSet.Entries = []PlatformConfigEntry{{
			Key:       "DATABASE_PASSWORD",
			Sensitive: true,
			Value:     "must-not-be-persisted",
		}}

		require.Error(t, ValidatePlatformConfigSet(configSet))
	})

	t.Run("secret reference metadata", func(t *testing.T) {
		configSet := base
		configSet.Entries = []PlatformConfigEntry{{
			Key:       "DATABASE_PASSWORD",
			Sensitive: true,
			ValueType: PlatformConfigValueSecretRef,
			Value:     "future-secret-reference",
		}}

		require.NoError(t, ValidatePlatformConfigSet(configSet))
	})
}

func TestPlatformArtifactStorageDefaultsAndValidation(t *testing.T) {
	storage := NewPlatformArtifactStorage()
	storage.Name = "delivery-minio"
	storage.Endpoint = "https://minio.example.com"
	storage.Bucket = "artifacts"
	storage.PathPrefix = "releases/v05"
	storage.AuthorizedProjectIDs = []PlatformProjectID{2, 1}
	storage.AccessKeyCipherText = "encrypted-access-key"
	storage.SecretKeyCipherText = "encrypted-secret-key"
	storage.CredentialEncryptionVersion = PlatformArtifactStorageCredentialEncryptionVersion
	storage.CredentialHash = "credential-hash"

	require.Equal(t, PlatformArtifactStorageProviderS3Compatible, storage.Provider)
	require.True(t, storage.UseTLS)
	// 归一化发生在 dataservice 持久化边界；校验函数接收值类型，不能隐式修改调用方的授权项目顺序。
	NormalizePlatformArtifactStorage(&storage)
	require.NoError(t, ValidatePlatformArtifactStorage(storage))
	require.Equal(t, []PlatformProjectID{1, 2}, storage.AuthorizedProjectIDs)

	storage.PathPrefix = "../escape"
	require.Error(t, ValidatePlatformArtifactStorage(storage))

	storage.PathPrefix = "releases"
	storage.SecretKeyCipherText = ""
	require.Error(t, ValidatePlatformArtifactStorage(storage))
}

func TestPlatformArtifactNormalizationAndValidation(t *testing.T) {
	image := PlatformArtifact{
		ProjectID:  1,
		Name:       "orders",
		Version:    "1.0.0",
		Type:       PlatformArtifactTypeImage,
		SourceType: PlatformArtifactSourceImageReference,
		ImageRef:   "registry.example.com/orders:1.0.0",
	}

	NormalizePlatformArtifact(&image)
	require.Equal(t, PlatformArtifactStatusReady, image.Status)
	require.NoError(t, ValidatePlatformArtifact(image))

	archive := PlatformArtifact{
		ProjectID:   1,
		Name:        "orders",
		Version:     "1.0.1",
		Type:        PlatformArtifactTypeOCIArchive,
		SourceType:  PlatformArtifactSourceObjectStorage,
		StorageID:   1,
		StoragePath: "releases/orders.oci.tar",
		SourcePath:  "releases/orders.oci.tar",
		SHA256:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Retained:    true,
	}

	NormalizePlatformArtifact(&archive)
	require.Equal(t, PlatformArtifactStatusFetched, archive.Status)
	require.NoError(t, ValidatePlatformArtifact(archive))

	archive.StoragePath = "../../escape"
	require.Error(t, ValidatePlatformArtifact(archive))
}

func TestPlatformGatewayModelsNormalizeAndRejectUnsafeInputs(t *testing.T) {
	gateway := NewPlatformGateway()
	gateway.ProjectID = 1
	gateway.EnvironmentID = 2
	gateway.EndpointID = 3
	gateway.Name = "  production-gateway "
	require.NoError(t, ValidatePlatformGateway(gateway))
	NormalizePlatformGateway(&gateway)
	require.Equal(t, "production-gateway", gateway.Name)

	route := NewPlatformGatewayRoute()
	route.GatewayID = 1
	route.ProjectID = 1
	route.EnvironmentID = 2
	route.ServiceDeploymentID = 4
	route.Domain = "API.Example.COM."
	route.Path = "/api"
	route.TargetPort = 8080
	require.NoError(t, ValidatePlatformGatewayRoute(route))
	NormalizePlatformGatewayRoute(&route)
	require.Equal(t, "api.example.com", route.Domain)
	require.Equal(t, 60, route.ProxyTimeoutSeconds)

	route.Path = "/api\nproxy_pass http://untrusted"
	require.Error(t, ValidatePlatformGatewayRoute(route))

	route = NewPlatformGatewayRoute()
	route.GatewayID = 1
	route.ProjectID = 1
	route.EnvironmentID = 2
	route.ServiceDeploymentID = 4
	route.Domain = "api.example.com"
	route.TargetPort = 8080
	route.ForceHTTPS = true
	require.Error(t, ValidatePlatformGatewayRoute(route))
}

func TestPlatformGatewayCertificateAndConfigVersionValidation(t *testing.T) {
	certificate := NewPlatformGatewayCertificate()
	certificate.ProjectID = 1
	certificate.Name = "example.com"
	certificate.Domains = []string{"WWW.EXAMPLE.COM.", "example.com"}
	require.NoError(t, ValidatePlatformGatewayCertificate(certificate))
	NormalizePlatformGatewayCertificate(&certificate)
	require.Equal(t, []string{"example.com", "www.example.com"}, certificate.Domains)

	certificate.HasPrivateKey = true
	require.Error(t, ValidatePlatformGatewayCertificate(certificate))

	version := PlatformGatewayConfigVersion{
		GatewayID:  1,
		Revision:   1,
		Status:     PlatformGatewayConfigStatusCandidate,
		ConfigHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RouteIDs:   []PlatformGatewayRouteID{2, 1},
	}
	require.NoError(t, ValidatePlatformGatewayConfigVersion(version))
	NormalizePlatformGatewayConfigVersion(&version)
	require.Equal(t, []PlatformGatewayRouteID{1, 2}, version.RouteIDs)

	version.RouteIDs = []PlatformGatewayRouteID{1, 1}
	require.Error(t, ValidatePlatformGatewayConfigVersion(version))
}

func TestPlatformHostGroupAndBatchPolicyValidation(t *testing.T) {
	group := NewPlatformHostGroup()
	group.ProjectID = 1
	group.EnvironmentID = 2
	group.Name = "  production-a "
	group.Targets = []PlatformDeploymentTarget{
		{EndpointID: 2, NodeName: "node-b", HostAddress: "10.0.0.12", Role: PlatformDeploymentTargetRoleWorkload, Enabled: true},
		{EndpointID: 1, NodeName: "node-a", HostAddress: "10.0.0.11", Role: PlatformDeploymentTargetRoleWorkload, Enabled: true},
	}
	require.NoError(t, ValidatePlatformHostGroup(group))
	NormalizePlatformHostGroup(&group)
	require.Equal(t, "production-a", group.Name)
	require.Equal(t, EndpointID(1), group.Targets[0].EndpointID)

	group.Targets[1] = group.Targets[0]
	require.Error(t, ValidatePlatformHostGroup(group))

	policy := NewPlatformBatchPolicy()
	require.NoError(t, ValidatePlatformBatchPolicy(policy))
	policy.BatchSize = 0
	require.Error(t, ValidatePlatformBatchPolicy(policy))
}
