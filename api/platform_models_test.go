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
