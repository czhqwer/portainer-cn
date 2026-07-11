package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	platformservice "github.com/portainer/portainer/api/platform"
	"github.com/portainer/portainer/pkg/fips"
	"github.com/stretchr/testify/require"
)

func TestPlatformSensitiveConfigEncryptsRedactsAuditsAndInjectsReleaseSnapshot(t *testing.T) {
	fips.InitFIPS(false)
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	const sensitiveValue = "value-for-encryption-test"

	configSet := createConfigSet(t, ctx, createConfigSetPayload{
		ProjectID: project.ID,
		ScopeType: portainer.PlatformConfigScopeProject,
		ScopeID:   int(project.ID),
		Entries: []portainer.PlatformConfigEntry{
			{Key: "API_TOKEN", Sensitive: true, Value: sensitiveValue, Required: true},
		},
	})
	entry := configSet.Entries[0]
	require.Empty(t, entry.Value)
	require.Empty(t, entry.CipherText)
	require.True(t, entry.HasValue)
	require.NotEmpty(t, entry.Hash)
	require.Equal(t, platformservice.PlatformSecretEncryptionVersion, entry.EncryptionVersion)

	stored, err := ctx.handler.DataStore.PlatformConfigSet().Read(configSet.ID)
	require.NoError(t, err)
	require.Empty(t, stored.Entries[0].Value)
	require.NotEmpty(t, stored.Entries[0].CipherText)
	require.NotEqual(t, sensitiveValue, stored.Entries[0].CipherText)

	doRawJSON(t, ctx, ctx.standardJWT, http.MethodPost, fmt.Sprintf("/platform/config-sets/%d/entries/API_TOKEN/reveal", configSet.ID), nil, http.StatusForbidden)
	revealed := doJSON[secretRevealResponse](t, ctx, http.MethodPost, fmt.Sprintf("/platform/config-sets/%d/entries/API_TOKEN/reveal", configSet.ID), nil, http.StatusOK)
	require.Equal(t, sensitiveValue, revealed.Value)
	copied := doJSON[secretRevealResponse](t, ctx, http.MethodPost, fmt.Sprintf("/platform/config-sets/%d/entries/API_TOKEN/copy", configSet.ID), nil, http.StatusOK)
	require.Equal(t, sensitiveValue, copied.Value)

	audits := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?projectId=%d", project.ID), nil, http.StatusOK)
	require.Len(t, audits, 2)
	for _, audit := range audits {
		require.NotContains(t, fmt.Sprint(audit.AfterSummary), sensitiveValue)
		require.Equal(t, []string{"API_TOKEN"}, audit.SensitiveFields)
	}

	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})
	releaseResponse := postReleaseExpectAccepted(t, ctx, "secret-snapshot", createReleasePayloadFor(project, application, service, deployment, artifact))
	release := doJSON[portainer.PlatformRelease](t, ctx, http.MethodGet, fmt.Sprintf("/platform/releases/%d", releaseResponse.ReleaseID), nil, http.StatusOK)
	require.Len(t, release.ConfigSnapshot.SecretSnapshots, 1)
	secretSnapshot := release.ConfigSnapshot.SecretSnapshots[0]
	require.Equal(t, "API_TOKEN", secretSnapshot.Name)
	require.Empty(t, secretSnapshot.CipherText)
	storedRelease, err := ctx.handler.DataStore.PlatformRelease().Read(release.ID)
	require.NoError(t, err)
	storedSecretSnapshot := storedRelease.ConfigSnapshot.SecretSnapshots[0]
	require.NotEmpty(t, storedSecretSnapshot.CipherText)
	require.NotEqual(t, sensitiveValue, storedSecretSnapshot.CipherText)

}
