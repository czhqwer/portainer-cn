package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	platformservice "github.com/portainer/portainer/api/platform"
	"github.com/portainer/portainer/pkg/fips"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

func init() {
	fips.InitFIPS(false)
}

type failingRollbackReleaseExecutor struct{}

func (failingRollbackReleaseExecutor) Execute(ctx context.Context, request platformservice.ReleaseExecutionRequest) (platformservice.ReleaseExecutionResult, error) {
	if request.Release.TriggerType != portainer.PlatformReleaseTriggerRollback {
		return fakeReleaseExecutor{}.Execute(ctx, request)
	}

	release := request.Release
	release.Status = portainer.PlatformReleaseStatusFailed
	release.FailureReason = platformservice.ReleaseFailureReasonImagePullFailed
	release.FinishedAt = time.Now().Unix()
	return platformservice.ReleaseExecutionResult{Release: release}, nil
}

func TestPlatformReleaseRollbackCreatesNewReleaseAndRedactsDiff(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}

	source, current := createRollbackReleaseHistory(t, ctx, project, application, service, deployment)
	source.ConfigSnapshot.DesiredSpecSnapshot.Ports[0].HostPort = 18081
	source.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides = []portainer.PlatformEnvVar{
		{Name: "ROLLBACK_PLAIN_CONFIG", Value: "rollback-plain-sentinel", Source: portainer.PlatformEnvVarSourceLiteral, HasValue: true},
		{Name: "ROLLBACK_SECRET", IsSecret: true, Source: portainer.PlatformEnvVarSourceLiteral, HasValue: true, Hash: "rollback-secret-hash"},
	}
	source.ConfigSnapshot.EffectiveConfigSnapshot.Entries = []portainer.PlatformConfigEntrySnapshot{
		{Key: "ROLLBACK_PLAIN_CONFIG", ValueType: portainer.PlatformConfigValuePlain, Value: "rollback-plain-sentinel", Source: portainer.PlatformConfigEntrySourceServiceDeployment, HasValue: true},
		{Key: "ROLLBACK_SECRET", ValueType: portainer.PlatformConfigValuePlain, Sensitive: true, Source: portainer.PlatformConfigEntrySourceServiceDeployment, HasValue: true, Hash: "rollback-secret-hash"},
	}
	source.ConfigSnapshot.ConfigHash = "rollback-config-hash"
	cipher, err := platformservice.NewSecretCipher(ctx.handler.DataStore.Connection())
	require.NoError(t, err)
	cipherText, hash, err := cipher.Encrypt("rollback-secret-sentinel")
	require.NoError(t, err)
	source.ConfigSnapshot.SecretSnapshots = []portainer.PlatformSecretSnapshot{{
		Name:              "ROLLBACK_SECRET",
		CipherText:        cipherText,
		EncryptionVersion: platformservice.PlatformSecretEncryptionVersion,
		Hash:              hash,
		HasValue:          true,
	}}
	require.NoError(t, ctx.handler.DataStore.PlatformRelease().Update(source.ID, source))

	diffRecorder := doRawJSONWithHeaders(t, ctx, ctx.adminJWT, http.MethodGet, fmt.Sprintf("/platform/releases/%d/rollback-diff", source.ID), nil, nil, http.StatusOK)
	require.NotContains(t, diffRecorder.Body.String(), "rollback-plain-sentinel")
	require.NotContains(t, diffRecorder.Body.String(), cipherText)
	var diff rollbackDiffResponse
	require.NoError(t, json.Unmarshal(diffRecorder.Body.Bytes(), &diff))
	require.Equal(t, source.ID, diff.SourceReleaseID)
	require.Equal(t, current.ID, diff.CurrentReleaseID)
	require.True(t, diff.ImageChanged)
	require.True(t, diff.ConfigChanged)
	require.True(t, diff.PortsChanged)
	require.True(t, diff.EnvironmentChanged)
	require.Equal(t, []string{"ROLLBACK_PLAIN_CONFIG"}, diff.ChangedConfigKeys)
	require.Equal(t, []string{"ROLLBACK_PLAIN_CONFIG"}, diff.ChangedEnvironmentNames)
	require.Equal(t, []rollbackSensitiveVariable{{Name: "ROLLBACK_SECRET", SourceHasValue: true, CurrentHasValue: false, Changed: true}}, diff.SensitiveVariables)

	rollback := postRollbackExpectAccepted(t, ctx, source.ID, "rollback-success", rollbackReleasePayload{})
	idempotentRollback := postRollbackExpectAccepted(t, ctx, source.ID, "rollback-success", rollbackReleasePayload{})
	require.Equal(t, rollback.ReleaseID, idempotentRollback.ReleaseID)
	storedSource, err := ctx.handler.DataStore.PlatformRelease().Read(source.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, storedSource.Status)

	rolledBack, err := ctx.handler.DataStore.PlatformRelease().Read(rollback.ReleaseID)
	require.NoError(t, err)
	require.NotEqual(t, source.ID, rolledBack.ID)
	require.Equal(t, portainer.PlatformReleaseTriggerRollback, rolledBack.TriggerType)
	require.Equal(t, source.ID, rolledBack.RollbackSourceReleaseID)
	require.Equal(t, current.ID, rolledBack.PreviousReleaseID)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, rolledBack.Status)
	require.Equal(t, source.ConfigSnapshot, rolledBack.ConfigSnapshot)

	storedDeployment, err := ctx.handler.DataStore.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, rolledBack.ID, storedDeployment.CurrentServingReleaseID)
	require.Equal(t, 18081, storedDeployment.DesiredSpec.Ports[0].HostPort)

	assertReleaseAuditAction(t, ctx, rolledBack.ID, portainer.PlatformAuditActionRollbackCreated, portainer.PlatformAuditResultSuccess)
	assertReleaseAuditAction(t, ctx, rolledBack.ID, portainer.PlatformAuditActionRollbackSucceeded, portainer.PlatformAuditResultSuccess)
}

func TestPlatformReleaseRollbackFailureKeepsCurrentServingRelease(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	source, current := createRollbackReleaseHistory(t, ctx, project, application, service, deployment)
	ctx.handler.ReleaseExecutor = failingRollbackReleaseExecutor{}

	rollback := postRollbackExpectAccepted(t, ctx, source.ID, "rollback-failure", rollbackReleasePayload{})
	failed, err := ctx.handler.DataStore.PlatformRelease().Read(rollback.ReleaseID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, failed.Status)
	require.Equal(t, platformservice.ReleaseFailureReasonImagePullFailed, failed.FailureReason)
	require.Equal(t, source.ID, failed.RollbackSourceReleaseID)

	storedDeployment, err := ctx.handler.DataStore.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, current.ID, storedDeployment.CurrentServingReleaseID)
	require.Equal(t, current.Image, storedDeployment.CurrentImage)
	assertReleaseAuditAction(t, ctx, failed.ID, portainer.PlatformAuditActionRollbackFailed, portainer.PlatformAuditResultFailed)
}

func TestPlatformReleaseRollbackRejectsUnavailableSensitiveSnapshot(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	source, current := createRollbackReleaseHistory(t, ctx, project, application, service, deployment)
	source.ConfigSnapshot.SecretSnapshots = []portainer.PlatformSecretSnapshot{{
		Name:              "UNAVAILABLE_SECRET",
		EncryptionVersion: platformservice.PlatformSecretEncryptionVersion,
		Hash:              "hash",
		HasValue:          true,
	}}
	require.NoError(t, ctx.handler.DataStore.PlatformRelease().Update(source.ID, source))

	response := doRawJSONWithHeaders(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/releases/%d/rollback", source.ID), rollbackReleasePayload{}, map[string]string{idempotencyKeyHeader: "rollback-unavailable-secret"}, http.StatusBadRequest)
	require.Contains(t, response.Body.String(), "sensitive configuration snapshot is unavailable")

	releases, err := ctx.handler.DataStore.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		return release.ServiceDeploymentID == deployment.ID && release.TriggerType == portainer.PlatformReleaseTriggerRollback
	})
	require.NoError(t, err)
	require.Empty(t, releases)
	storedDeployment, err := ctx.handler.DataStore.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, current.ID, storedDeployment.CurrentServingReleaseID)
}

func TestPlatformReleaseRollbackRequiresProductionConfirmation(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	environment, err := ctx.handler.DataStore.PlatformEnvironment().Read(deployment.EnvironmentID)
	require.NoError(t, err)
	environment.Type = portainer.PlatformEnvironmentTypeProd
	environment.IsProduction = true
	require.NoError(t, ctx.handler.DataStore.PlatformEnvironment().Update(environment.ID, environment))

	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	source, _ := createRollbackReleaseHistory(t, ctx, project, application, service, deployment)

	response := doRawJSONWithHeaders(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/releases/%d/rollback", source.ID), rollbackReleasePayload{}, map[string]string{idempotencyKeyHeader: "rollback-production"}, http.StatusBadRequest)
	require.Contains(t, response.Body.String(), "Production rollback requires explicit confirmation")

	confirmed := postRollbackExpectAccepted(t, ctx, source.ID, "rollback-production-confirmed", rollbackReleasePayload{ConfirmProduction: true})
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, confirmed.Status)
}

func createRollbackReleaseHistory(t *testing.T, ctx platformTestContext, project portainer.PlatformProject, application portainer.PlatformApplication, service portainer.PlatformServiceDefinition, deployment portainer.PlatformServiceDeployment) (*portainer.PlatformRelease, *portainer.PlatformRelease) {
	t.Helper()

	sourceArtifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID: project.ID, ApplicationID: application.ID, ServiceDefinitionID: service.ID,
		Name: "orders-api", Version: "1.0.0", ImageRef: "registry.example.com/orders-api:1.0.0",
	})
	sourceResponse := postReleaseExpectAccepted(t, ctx, "release-v1", createReleasePayloadFor(project, application, service, deployment, sourceArtifact))
	source, err := ctx.handler.DataStore.PlatformRelease().Read(sourceResponse.ReleaseID)
	require.NoError(t, err)

	currentArtifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID: project.ID, ApplicationID: application.ID, ServiceDefinitionID: service.ID,
		Name: "orders-api", Version: "2.0.0", ImageRef: "registry.example.com/orders-api:2.0.0",
	})
	currentResponse := postReleaseExpectAccepted(t, ctx, "release-v2", createReleasePayloadFor(project, application, service, deployment, currentArtifact))
	current, err := ctx.handler.DataStore.PlatformRelease().Read(currentResponse.ReleaseID)
	require.NoError(t, err)
	return source, current
}

func postRollbackExpectAccepted(t *testing.T, ctx platformTestContext, releaseID portainer.PlatformReleaseID, idempotencyKey string, payload rollbackReleasePayload) releaseCreateResponse {
	t.Helper()
	recorder := doRawJSONWithHeaders(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/releases/%d/rollback", releaseID), payload, map[string]string{idempotencyKeyHeader: idempotencyKey}, http.StatusAccepted)
	var result releaseCreateResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	return result
}

func assertReleaseAuditAction(t *testing.T, ctx platformTestContext, releaseID portainer.PlatformReleaseID, action portainer.PlatformAuditAction, result portainer.PlatformAuditResult) {
	t.Helper()
	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ReleaseID == releaseID && audit.Action == action
	})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, result, audits[0].Result)
	require.NotContains(t, strings.Join(audits[0].SensitiveFields, ","), "rollback-cipher-sentinel")
}
