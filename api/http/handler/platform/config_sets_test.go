package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestPlatformConfigSetCRUDMergeSnapshotAndDrift(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)

	projectConfig := createConfigSet(t, ctx, createConfigSetPayload{
		ProjectID: project.ID,
		ScopeType: portainer.PlatformConfigScopeProject,
		ScopeID:   int(project.ID),
		Entries: []portainer.PlatformConfigEntry{
			{Key: "APP_ENV", Value: "project"},
			{Key: "PROJECT_ONLY", Value: "project-only"},
			{Key: "API_TOKEN", Sensitive: true, ValueType: portainer.PlatformConfigValueSecretRef, Value: "secret-reference"},
		},
	})
	require.Equal(t, portainer.PlatformConfigSetDefaultName, projectConfig.Name)
	require.Equal(t, 1, projectConfig.Revision)
	require.Equal(t, portainer.PlatformConfigEntrySourceProject, projectConfig.Entries[0].Source)
	require.Empty(t, projectConfig.Entries[2].Value)

	createConfigSet(t, ctx, createConfigSetPayload{
		ProjectID: project.ID,
		ScopeType: portainer.PlatformConfigScopeEnvironment,
		ScopeID:   int(deployment.EnvironmentID),
		Entries: []portainer.PlatformConfigEntry{
			{Key: "APP_ENV", Value: "environment"},
			{Key: "ENV_ONLY", Value: "environment-only"},
		},
	})
	createConfigSet(t, ctx, createConfigSetPayload{
		ProjectID: project.ID,
		ScopeType: portainer.PlatformConfigScopeServiceDeployment,
		ScopeID:   int(deployment.ID),
		Entries: []portainer.PlatformConfigEntry{
			{Key: "SERVICE_ONLY", Value: "service-only"},
		},
	})

	updatedSpec := deployment.DesiredSpec
	updatedSpec.EnvOverrides = []portainer.PlatformEnvVar{{Name: "APP_ENV", Value: "deployment"}}
	deployment = doJSON[portainer.PlatformServiceDeployment](t, ctx, http.MethodPut, fmt.Sprintf("/platform/service-deployments/%d", deployment.ID), updateServiceDeploymentPayload{
		ResourceVersion: deployment.ResourceVersion,
		DesiredSpec:     &updatedSpec,
	}, http.StatusOK)

	effective := doJSON[effectiveConfigResponse](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/effective-config", deployment.ID), nil, http.StatusOK)
	entries := configSnapshotEntriesByKey(effective.EffectiveConfig.Entries)
	require.Equal(t, "deployment", entries["APP_ENV"].Value)
	require.Equal(t, "project-only", entries["PROJECT_ONLY"].Value)
	require.Equal(t, "environment-only", entries["ENV_ONLY"].Value)
	require.Equal(t, "service-only", entries["SERVICE_ONLY"].Value)
	require.Empty(t, entries["API_TOKEN"].Value)
	require.True(t, entries["API_TOKEN"].Sensitive)
	require.NotEmpty(t, effective.EffectiveConfig.Hash)

	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})
	releaseResponse := postReleaseExpectAccepted(t, ctx, "config-snapshot", createReleasePayloadFor(project, application, service, deployment, artifact))
	release := doJSON[portainer.PlatformRelease](t, ctx, http.MethodGet, fmt.Sprintf("/platform/releases/%d", releaseResponse.ReleaseID), nil, http.StatusOK)
	require.Equal(t, effective.EffectiveConfig.Hash, release.ConfigSnapshot.ConfigHash)
	require.Equal(t, effective.EffectiveConfig, release.ConfigSnapshot.EffectiveConfigSnapshot)

	projectConfig.Entries[1].Value = "project-updated"
	updatedProjectConfig := doJSON[portainer.PlatformConfigSet](t, ctx, http.MethodPut, fmt.Sprintf("/platform/config-sets/%d", projectConfig.ID), updateConfigSetPayload{
		ResourceVersion: projectConfig.ResourceVersion,
		Entries:         &projectConfig.Entries,
	}, http.StatusOK)
	require.Equal(t, 2, updatedProjectConfig.Revision)

	drifted := doJSON[portainer.PlatformServiceDeployment](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d", deployment.ID), nil, http.StatusOK)
	require.Equal(t, portainer.PlatformDeploymentDriftConfigChanged, drifted.DriftStatus)
}

func TestPlatformConfigSetRejectsCrossProjectScopeAndArchives(t *testing.T) {
	ctx, project, _, _, deployment := createPlatformReleaseFixture(t)
	otherProject := createProject(t, ctx, createProjectPayload{Name: "Other", Slug: "other"})

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, "/platform/config-sets", createConfigSetPayload{
		ProjectID: otherProject.ID,
		ScopeType: portainer.PlatformConfigScopeServiceDeployment,
		ScopeID:   int(deployment.ID),
		Entries:   []portainer.PlatformConfigEntry{{Key: "APP_ENV", Value: "invalid"}},
	}, http.StatusBadRequest)

	configSet := createConfigSet(t, ctx, createConfigSetPayload{
		ProjectID: project.ID,
		ScopeType: portainer.PlatformConfigScopeProject,
		ScopeID:   int(project.ID),
		Entries:   []portainer.PlatformConfigEntry{{Key: "APP_ENV", Value: "project"}},
	})
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/config-sets/%d", configSet.ID), nil, http.StatusNoContent)

	visible := doJSON[[]portainer.PlatformConfigSet](t, ctx, http.MethodGet, fmt.Sprintf("/platform/config-sets?projectId=%d", project.ID), nil, http.StatusOK)
	require.Empty(t, visible)
	archived := doJSON[[]portainer.PlatformConfigSet](t, ctx, http.MethodGet, fmt.Sprintf("/platform/config-sets?projectId=%d&includeArchived=true", project.ID), nil, http.StatusOK)
	require.Len(t, archived, 1)
	require.Equal(t, portainer.PlatformLifecycleStatusArchived, archived[0].LifecycleStatus)
}

func createConfigSet(t *testing.T, ctx platformTestContext, payload createConfigSetPayload) portainer.PlatformConfigSet {
	t.Helper()
	return doJSON[portainer.PlatformConfigSet](t, ctx, http.MethodPost, "/platform/config-sets", payload, http.StatusCreated)
}

func configSnapshotEntriesByKey(entries []portainer.PlatformConfigEntrySnapshot) map[string]portainer.PlatformConfigEntrySnapshot {
	result := make(map[string]portainer.PlatformConfigEntrySnapshot, len(entries))
	for _, entry := range entries {
		result[entry.Key] = entry
	}
	return result
}
