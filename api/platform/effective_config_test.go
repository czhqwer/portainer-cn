package platform

import (
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestBuildEffectiveConfigSnapshotMergesScopesAndRedactsSensitiveEntries(t *testing.T) {
	deployment := portainer.PlatformServiceDeployment{
		ID:            30,
		ProjectID:     10,
		EnvironmentID: 20,
		SpecRevision:  4,
		DesiredSpec: portainer.PlatformDeploymentDesiredSpec{
			EnvOverrides: []portainer.PlatformEnvVar{
				{Name: "APP_ENV", Value: "deployment"},
			},
			ConfigRefs: []portainer.PlatformConfigRef{
				{ConfigSetID: 4, Key: "EXTRA", Required: true},
			},
		},
	}
	configSets := []portainer.PlatformConfigSet{
		configSet(1, 10, portainer.PlatformConfigScopeProject, 10, "default", 2,
			portainer.PlatformConfigEntry{Key: "APP_ENV", Value: "project"},
			portainer.PlatformConfigEntry{Key: "PROJECT_ONLY", Value: "project-only"},
		),
		configSet(2, 10, portainer.PlatformConfigScopeEnvironment, 20, "default", 3,
			portainer.PlatformConfigEntry{Key: "APP_ENV", Value: "environment"},
			portainer.PlatformConfigEntry{Key: "ENV_ONLY", Value: "environment-only"},
		),
		configSet(3, 10, portainer.PlatformConfigScopeServiceDeployment, 30, "default", 5,
			portainer.PlatformConfigEntry{Key: "SERVICE_ONLY", Value: "service-only"},
			portainer.PlatformConfigEntry{Key: "API_TOKEN", Sensitive: true, ValueType: portainer.PlatformConfigValueSecretRef, Value: "secret-reference"},
		),
		configSet(4, 10, portainer.PlatformConfigScopeEnvironment, 20, "extra", 7,
			portainer.PlatformConfigEntry{Key: "EXTRA", Value: "referenced"},
		),
	}

	snapshot, err := BuildEffectiveConfigSnapshot(deployment, configSets)

	require.NoError(t, err)
	require.Equal(t, 4, snapshot.SpecRevision)
	require.Equal(t, 4, len(snapshot.ConfigSetRevisions))
	require.NotEmpty(t, snapshot.Hash)
	entries := configEntriesByKey(snapshot.Entries)
	require.Equal(t, "deployment", entries["APP_ENV"].Value)
	require.Equal(t, "project-only", entries["PROJECT_ONLY"].Value)
	require.Equal(t, "environment-only", entries["ENV_ONLY"].Value)
	require.Equal(t, "service-only", entries["SERVICE_ONLY"].Value)
	require.Equal(t, "referenced", entries["EXTRA"].Value)
	require.True(t, entries["API_TOKEN"].Sensitive)
	require.True(t, entries["API_TOKEN"].HasValue)
	require.Empty(t, entries["API_TOKEN"].Value)
	require.NotEmpty(t, entries["API_TOKEN"].Hash)
}

func TestBuildEffectiveConfigSnapshotIsDeterministicAndChecksRequiredReferences(t *testing.T) {
	deployment := portainer.PlatformServiceDeployment{
		ID:            3,
		ProjectID:     1,
		EnvironmentID: 2,
		SpecRevision:  1,
	}
	project := configSet(1, 1, portainer.PlatformConfigScopeProject, 1, "default", 1, portainer.PlatformConfigEntry{Key: "A", Value: "1"})
	environment := configSet(2, 1, portainer.PlatformConfigScopeEnvironment, 2, "default", 1, portainer.PlatformConfigEntry{Key: "B", Value: "2"})

	first, err := BuildEffectiveConfigSnapshot(deployment, []portainer.PlatformConfigSet{project, environment})
	require.NoError(t, err)
	second, err := BuildEffectiveConfigSnapshot(deployment, []portainer.PlatformConfigSet{environment, project})
	require.NoError(t, err)
	require.Equal(t, first, second)

	deployment.DesiredSpec.ConfigRefs = []portainer.PlatformConfigRef{{ConfigSetID: 99, Key: "MISSING", Required: true}}
	_, err = BuildEffectiveConfigSnapshot(deployment, []portainer.PlatformConfigSet{project, environment})
	require.Error(t, err)
}

func TestDockerEnvUsesEffectivePlainConfigAndKeepsLegacyFallback(t *testing.T) {
	request := ReleaseExecutionRequest{
		Deployment: portainer.PlatformServiceDeployment{
			DesiredSpec: portainer.PlatformDeploymentDesiredSpec{
				EnvOverrides: []portainer.PlatformEnvVar{{Name: "LEGACY", Value: "legacy"}},
			},
		},
		Release: portainer.PlatformRelease{
			ConfigSnapshot: portainer.PlatformServiceConfigSnapshot{
				EffectiveConfigSnapshot: portainer.PlatformEffectiveConfigSnapshot{
					Hash: "effective-hash",
					Entries: []portainer.PlatformConfigEntrySnapshot{
						{Key: "APP_ENV", ValueType: portainer.PlatformConfigValuePlain, Value: "production"},
						{Key: "API_TOKEN", ValueType: portainer.PlatformConfigValueSecretRef, Sensitive: true, HasValue: true},
					},
				},
			},
		},
	}

	require.Equal(t, []string{"APP_ENV=production"}, dockerEnv(request))
	request.Release.ConfigSnapshot.EffectiveConfigSnapshot = portainer.PlatformEffectiveConfigSnapshot{}
	require.Equal(t, []string{"LEGACY=legacy"}, dockerEnv(request))
}

func configSet(
	id portainer.PlatformConfigSetID,
	projectID portainer.PlatformProjectID,
	scope portainer.PlatformConfigScopeType,
	scopeID int,
	name string,
	revision int,
	entries ...portainer.PlatformConfigEntry,
) portainer.PlatformConfigSet {
	configSet := portainer.NewPlatformConfigSet()
	configSet.ID = id
	configSet.ProjectID = projectID
	configSet.ScopeType = scope
	configSet.ScopeID = scopeID
	configSet.Name = name
	configSet.Revision = revision
	configSet.Entries = entries
	portainer.NormalizePlatformConfigSet(&configSet)
	return configSet
}

func configEntriesByKey(entries []portainer.PlatformConfigEntrySnapshot) map[string]portainer.PlatformConfigEntrySnapshot {
	result := make(map[string]portainer.PlatformConfigEntrySnapshot, len(entries))
	for _, entry := range entries {
		result[entry.Key] = entry
	}
	return result
}
