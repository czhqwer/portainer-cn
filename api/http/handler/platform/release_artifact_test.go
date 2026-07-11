package platform

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	"github.com/portainer/portainer/api/internal/testhelpers"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

type releaseErrorResponse struct {
	Code    string                `json:"code"`
	Message string                `json:"message"`
	Details platformErrorDetails  `json:"details"`
	Data    releaseCreateResponse `json:"data"`
}

func TestPlatformArtifactImageReferenceLifecycle(t *testing.T) {
	ctx, project, application, service, _ := createPlatformReleaseFixture(t)

	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})
	require.Equal(t, portainer.PlatformArtifactTypeImage, artifact.Type)
	require.Equal(t, portainer.PlatformArtifactSourceImageReference, artifact.SourceType)
	require.Equal(t, portainer.PlatformTraceabilityWeak, artifact.Traceability)

	artifacts := doJSON[[]portainer.PlatformArtifact](t, ctx, http.MethodGet, fmt.Sprintf("/platform/artifacts?projectId=%d&serviceDefinitionId=%d", project.ID, service.ID), nil, http.StatusOK)
	require.Len(t, artifacts, 1)
	require.Equal(t, artifact.ID, artifacts[0].ID)

	validation := doJSON[artifactValidationResponse](t, ctx, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/validate", artifact.ID), nil, http.StatusOK)
	require.True(t, validation.Valid)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/artifacts/%d", artifact.ID), nil, http.StatusNoContent)

	artifacts = doJSON[[]portainer.PlatformArtifact](t, ctx, http.MethodGet, fmt.Sprintf("/platform/artifacts?projectId=%d", project.ID), nil, http.StatusOK)
	require.Empty(t, artifacts)

	archivedArtifacts := doJSON[[]portainer.PlatformArtifact](t, ctx, http.MethodGet, fmt.Sprintf("/platform/artifacts?projectId=%d&includeArchived=true", project.ID), nil, http.StatusOK)
	require.Len(t, archivedArtifacts, 1)
	require.Equal(t, portainer.PlatformLifecycleStatusArchived, archivedArtifacts[0].LifecycleStatus)
}

func TestPlatformReleaseCreateIsIdempotentAndGate0BBlocked(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})

	payload := createReleasePayloadFor(project, application, service, deployment, artifact)
	first := postReleaseExpectError(t, ctx, "same-key", payload, http.StatusBadRequest)
	require.Equal(t, errPlatformUnsupportedOperation, first.Code)
	require.Equal(t, releaseGate0BRequiredReason, first.Details.Reason)
	require.True(t, first.Data.Blocked)
	require.NotZero(t, first.Data.ReleaseID)

	second := postReleaseExpectError(t, ctx, "same-key", payload, http.StatusBadRequest)
	require.Equal(t, first.Data.ReleaseID, second.Data.ReleaseID)

	payload.Version = "1.0.1"
	mismatch := postReleaseExpectError(t, ctx, "same-key", payload, http.StatusConflict)
	require.Equal(t, errPlatformIdempotencyPayloadMismatch, mismatch.Code)
	require.Equal(t, releaseIdempotencyMismatchReason, mismatch.Details.Reason)

	release := doJSON[portainer.PlatformRelease](t, ctx, http.MethodGet, fmt.Sprintf("/platform/releases/%d", first.Data.ReleaseID), nil, http.StatusOK)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, release.Status)
	require.Equal(t, releaseGate0BRequiredReason, release.FailureReason)
	require.Len(t, release.Steps, 1)
	require.Equal(t, releaseGate0BRequiredReason, release.Steps[0].Reason)

	releases := doJSON[[]portainer.PlatformRelease](t, ctx, http.MethodGet, fmt.Sprintf("/platform/releases?serviceDeploymentId=%d", deployment.ID), nil, http.StatusOK)
	require.Len(t, releases, 1)
}

func TestPlatformReleaseConflictWithActiveLock(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})

	payload := createReleasePayloadFor(project, application, service, deployment, artifact)
	payloadHash, err := releasePayloadHash(payload)
	require.NoError(t, err)

	now := time.Now().Unix()
	lockedRelease := &portainer.PlatformRelease{
		ProjectID:            project.ID,
		EnvironmentID:        deployment.EnvironmentID,
		ApplicationID:        application.ID,
		ServiceDefinitionID:  service.ID,
		ServiceDeploymentID:  deployment.ID,
		ArtifactID:           artifact.ID,
		Version:              "locked",
		TriggerType:          portainer.PlatformReleaseTriggerDeploy,
		Strategy:             portainer.NewPlatformReleaseStrategy(),
		Status:               portainer.PlatformReleaseStatusQueued,
		OperatorUserID:       1,
		IdempotencyKeyHash:   releaseIdempotencyHash(1, deployment.ID, "active-key"),
		PayloadHash:          payloadHash,
		ExpectedSpecRevision: deployment.SpecRevision,
		CreatedAt:            now,
		QueueExpiresAt:       now + 600,
	}
	require.NoError(t, ctx.handler.DataStore.PlatformRelease().Create(lockedRelease))
	require.NoError(t, ctx.handler.DataStore.PlatformReleaseLock().Create(&portainer.PlatformReleaseLock{
		ServiceDeploymentID: deployment.ID,
		ReleaseID:           lockedRelease.ID,
		IdempotencyKeyHash:  lockedRelease.IdempotencyKeyHash,
		PayloadHash:         lockedRelease.PayloadHash,
		CreatedAt:           now,
		UpdatedAt:           now,
	}))

	conflict := postReleaseExpectError(t, ctx, "different-key", payload, http.StatusConflict)
	require.Equal(t, errPlatformReleaseConflict, conflict.Code)
	require.Equal(t, releaseLockedReason, conflict.Details.Reason)
	require.Equal(t, lockedRelease.ID, conflict.Data.ReleaseID)

	reused := postReleaseExpectAccepted(t, ctx, "active-key", payload)
	require.Equal(t, lockedRelease.ID, reused.ReleaseID)
	require.Equal(t, portainer.PlatformReleaseStatusQueued, reused.Status)

	lockID := platformreleaselock.LockIDForServiceDeployment(deployment.ID)
	lock, err := ctx.handler.DataStore.PlatformReleaseLock().Read(lockID)
	require.NoError(t, err)
	require.Equal(t, lockedRelease.ID, lock.ReleaseID)
}

func TestPlatformReleaseStateTransitions(t *testing.T) {
	require.NoError(t, validateReleaseTransition(portainer.PlatformReleaseStatusQueued, portainer.PlatformReleaseStatusValidating))
	require.NoError(t, validateReleaseTransition(portainer.PlatformReleaseStatusRecoveryFailed, portainer.PlatformReleaseStatusResolved))
	require.Error(t, validateReleaseTransition(portainer.PlatformReleaseStatusQueued, portainer.PlatformReleaseStatusSucceeded))

	require.True(t, releaseStatusReleasesLock(portainer.PlatformReleaseStatusSucceeded))
	require.True(t, releaseStatusReleasesLock(portainer.PlatformReleaseStatusFailed))
	require.False(t, releaseStatusReleasesLock(portainer.PlatformReleaseStatusRecoveryFailed))
	require.False(t, releaseStatusReleasesLock(portainer.PlatformReleaseStatusInterrupted))
}

func createPlatformReleaseFixture(t *testing.T) (platformTestContext, portainer.PlatformProject, portainer.PlatformApplication, portainer.PlatformServiceDefinition, portainer.PlatformServiceDeployment) {
	t.Helper()

	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Customer A", Slug: "customer-a"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Dev", Slug: "dev", Type: portainer.PlatformEnvironmentTypeDev})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Orders", Slug: "orders"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Orders API", Slug: "orders-api"})

	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/orders-api:1.0.0"
	spec.Ports = []portainer.PlatformPortSpec{{Name: "http", ContainerPort: 8080, HostPort: 18080}}
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{
		EnvironmentID: environment.ID,
		DesiredSpec:   &spec,
	})

	return ctx, project, application, service, deployment
}

func createImageReferenceArtifact(t *testing.T, ctx platformTestContext, payload createImageReferenceArtifactPayload) portainer.PlatformArtifact {
	t.Helper()

	return doJSON[portainer.PlatformArtifact](t, ctx, http.MethodPost, "/platform/artifacts/image-reference", payload, http.StatusCreated)
}

func createReleasePayloadFor(project portainer.PlatformProject, application portainer.PlatformApplication, service portainer.PlatformServiceDefinition, deployment portainer.PlatformServiceDeployment, artifact portainer.PlatformArtifact) createReleasePayload {
	return createReleasePayload{
		ProjectID:            project.ID,
		EnvironmentID:        deployment.EnvironmentID,
		ApplicationID:        application.ID,
		ServiceDefinitionID:  service.ID,
		ServiceDeploymentID:  deployment.ID,
		ArtifactID:           artifact.ID,
		Version:              artifact.Version,
		ExpectedSpecRevision: deployment.SpecRevision,
		Strategy:             portainer.NewPlatformReleaseStrategy(),
		TriggerType:          portainer.PlatformReleaseTriggerDeploy,
	}
}

func postReleaseExpectError(t *testing.T, ctx platformTestContext, idempotencyKey string, payload createReleasePayload, expectedStatus int) releaseErrorResponse {
	t.Helper()

	recorder := doRawJSONWithHeaders(t, ctx, ctx.adminJWT, http.MethodPost, "/platform/releases", payload, map[string]string{idempotencyKeyHeader: idempotencyKey}, expectedStatus)
	var result releaseErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))

	return result
}

func postReleaseExpectAccepted(t *testing.T, ctx platformTestContext, idempotencyKey string, payload createReleasePayload) releaseCreateResponse {
	t.Helper()

	recorder := doRawJSONWithHeaders(t, ctx, ctx.adminJWT, http.MethodPost, "/platform/releases", payload, map[string]string{idempotencyKeyHeader: idempotencyKey}, http.StatusAccepted)
	var result releaseCreateResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))

	return result
}

func doRawJSONWithHeaders(t *testing.T, ctx platformTestContext, token string, method string, target string, payload any, headers map[string]string, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()

	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	testhelpers.AddTestSecurityCookie(req, token)

	recorder := httptest.NewRecorder()
	ctx.handler.ServeHTTP(recorder, req)
	require.Equal(t, expectedStatus, recorder.Code, recorder.Body.String())

	return recorder
}
