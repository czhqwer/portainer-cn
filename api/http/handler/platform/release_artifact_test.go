package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	"github.com/portainer/portainer/api/internal/testhelpers"
	platformservice "github.com/portainer/portainer/api/platform"

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

type fakeReleaseExecutor struct{}

func (fakeReleaseExecutor) Execute(_ context.Context, request platformservice.ReleaseExecutionRequest) (platformservice.ReleaseExecutionResult, error) {
	now := time.Now().Unix()
	release := request.Release
	release.Status = portainer.PlatformReleaseStatusSucceeded
	release.StartedAt = now
	release.FinishedAt = now
	release.HealthCheckResult = portainer.PlatformHealthCheckResult{
		Level:      portainer.PlatformHealthVerificationVerified,
		Type:       portainer.PlatformHealthCheckTypeHTTP,
		Status:     portainer.PlatformHealthCheckStatusPassed,
		Target:     "http://127.0.0.1:18080/health",
		StartedAt:  now,
		FinishedAt: now,
	}
	release.RuntimeSnapshot.CurrentRuntimeRef = portainer.RuntimeRef{
		DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
		EndpointID:   1,
		ResourceType: portainer.PlatformRuntimeResourceContainer,
		ResourceID:   "container-1",
		Name:         "pcn-customer-a-dev-orders-api-r1",
	}
	release.RuntimeSnapshot.PublishedPorts = []portainer.PlatformPublishedPort{{Name: "http", ContainerPort: 8080, HostPort: 18080, Protocol: portainer.PlatformPortProtocolTCP}}
	release.Steps = []portainer.PlatformReleaseStep{{Name: "fake-execute", Status: portainer.PlatformReleaseStepStatusSucceeded, StartedAt: now, FinishedAt: now}}

	deployment := request.Deployment
	deployment.CurrentServingReleaseID = release.ID
	deployment.CurrentArtifactID = request.Artifact.ID
	deployment.CurrentRuntimeRef = release.RuntimeSnapshot.CurrentRuntimeRef
	deployment.CurrentImage = request.Artifact.ImageRef
	deployment.LastDeployedSpecRevision = deployment.SpecRevision
	deployment.LastDeployedAt = now
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone
	deployment.ResourceVersion++
	deployment.UpdatedAt = now

	return platformservice.ReleaseExecutionResult{Release: release, Deployment: &deployment}, nil
}

type fakeRuntimeInspector struct {
	inspection platformservice.RuntimeInspection
	logs       platformservice.RuntimeLogResult
}

func (inspector fakeRuntimeInspector) InspectRuntime(context.Context, portainer.RuntimeRef) (platformservice.RuntimeInspection, error) {
	return inspector.inspection, nil
}

func (inspector fakeRuntimeInspector) RuntimeLogs(_ context.Context, runtimeRef portainer.RuntimeRef, options platformservice.RuntimeLogOptions) (platformservice.RuntimeLogResult, error) {
	result := inspector.logs
	result.RuntimeRef = runtimeRef
	if result.Tail == 0 {
		result.Tail = options.Tail
	}
	return result, nil
}

type fakeReleaseRecoveryExecutor struct {
	retryRequest  platformservice.ReleaseExecutionRequest
	cleanupResult platformservice.ReleaseCleanupResult
}

func (executor *fakeReleaseRecoveryExecutor) RetryRecovery(_ context.Context, request platformservice.ReleaseExecutionRequest) (platformservice.ReleaseExecutionResult, error) {
	executor.retryRequest = request
	now := time.Now().Unix()
	release := request.Release
	release.Status = portainer.PlatformReleaseStatusFailed
	release.ManualActionRequired = false
	release.FinishedAt = now
	release.LeaseOwner = ""
	release.LeaseExpiresAt = 0
	release.Steps = append(release.Steps, portainer.PlatformReleaseStep{
		Name:       "retry-recovery",
		Status:     portainer.PlatformReleaseStepStatusSucceeded,
		RuntimeRef: request.Deployment.CurrentRuntimeRef,
		StartedAt:  now,
		FinishedAt: now,
	})

	deployment := request.Deployment
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone

	return platformservice.ReleaseExecutionResult{Release: release, Deployment: &deployment}, nil
}

func (executor *fakeReleaseRecoveryExecutor) CleanupRuntime(_ context.Context, request platformservice.ReleaseCleanupRequest) (platformservice.ReleaseCleanupResult, error) {
	now := time.Now().Unix()
	result := executor.cleanupResult
	result.Release = request.Release
	result.Release.Steps = append(result.Release.Steps, portainer.PlatformReleaseStep{
		Name:       "cleanup-runtime",
		Status:     portainer.PlatformReleaseStepStatusSucceeded,
		StartedAt:  now,
		FinishedAt: now,
	})

	return result, nil
}

func TestPlatformReleaseCreateIsIdempotentAndExecutes(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})

	payload := createReleasePayloadFor(project, application, service, deployment, artifact)
	first := postReleaseExpectAccepted(t, ctx, "same-key", payload)
	require.NotZero(t, first.ReleaseID)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, first.Status)

	second := postReleaseExpectAccepted(t, ctx, "same-key", payload)
	require.Equal(t, first.ReleaseID, second.ReleaseID)

	payload.Version = "1.0.1"
	mismatch := postReleaseExpectError(t, ctx, "same-key", payload, http.StatusConflict)
	require.Equal(t, errPlatformIdempotencyPayloadMismatch, mismatch.Code)
	require.Equal(t, releaseIdempotencyMismatchReason, mismatch.Details.Reason)

	release := doJSON[portainer.PlatformRelease](t, ctx, http.MethodGet, fmt.Sprintf("/platform/releases/%d", first.ReleaseID), nil, http.StatusOK)
	require.Equal(t, portainer.PlatformReleaseStatusSucceeded, release.Status)
	require.Empty(t, release.FailureReason)
	require.Len(t, release.Steps, 1)
	require.Equal(t, portainer.PlatformHealthCheckStatusPassed, release.HealthCheckResult.Status)
	require.Equal(t, "container-1", release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID)

	updatedDeployment := doJSON[portainer.PlatformServiceDeployment](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d", deployment.ID), nil, http.StatusOK)
	require.Equal(t, release.ID, updatedDeployment.CurrentServingReleaseID)
	require.Equal(t, artifact.ID, updatedDeployment.CurrentArtifactID)
	require.Equal(t, artifact.ImageRef, updatedDeployment.CurrentImage)
	require.Equal(t, "container-1", updatedDeployment.CurrentRuntimeRef.ResourceID)

	lockID := platformreleaselock.LockIDForServiceDeployment(deployment.ID)
	_, err := ctx.handler.DataStore.PlatformReleaseLock().Read(lockID)
	require.Error(t, err)
	require.True(t, ctx.handler.DataStore.IsErrObjectNotFound(err))

	releases := doJSON[[]portainer.PlatformRelease](t, ctx, http.MethodGet, fmt.Sprintf("/platform/releases?serviceDeploymentId=%d", deployment.ID), nil, http.StatusOK)
	require.Len(t, releases, 1)

	audits := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?releaseId=%d", first.ReleaseID), nil, http.StatusOK)
	require.Len(t, audits, 2)
	require.Equal(t, portainer.PlatformAuditActionReleaseCreated, audits[0].Action)
	require.Equal(t, portainer.PlatformAuditActionReleaseSucceeded, audits[1].Action)
}

func TestPlatformReleaseConflictWithActiveLock(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
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

func TestPlatformServiceDeploymentStatusMarksRuntimeMissing(t *testing.T) {
	ctx, _, _, _, deployment := createPlatformReleaseFixture(t)
	deployment.CurrentRuntimeRef = portainer.RuntimeRef{
		DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
		EndpointID:   1,
		ResourceType: portainer.PlatformRuntimeResourceContainer,
		ResourceID:   "missing-container",
		Name:         "missing",
	}
	require.NoError(t, ctx.handler.DataStore.PlatformServiceDeployment().Update(deployment.ID, &deployment))
	ctx.handler.RuntimeInspector = fakeRuntimeInspector{inspection: platformservice.RuntimeInspection{
		RuntimeRef: deployment.CurrentRuntimeRef,
		Found:      false,
		Message:    "No such container",
	}}

	status := doJSON[serviceDeploymentStatusResponse](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/status", deployment.ID), nil, http.StatusOK)
	require.False(t, status.RuntimeFound)
	require.Equal(t, runtimeReasonMissing, status.Reason)
	require.Equal(t, portainer.PlatformDeploymentDriftRuntimeMissing, status.DriftStatus)

	updated, err := ctx.handler.DataStore.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformDeploymentDriftRuntimeMissing, updated.DriftStatus)
}

func TestPlatformServiceDeploymentLogsReturnsTail(t *testing.T) {
	ctx, _, _, _, deployment := createPlatformReleaseFixture(t)
	deployment.CurrentRuntimeRef = portainer.RuntimeRef{
		DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
		EndpointID:   1,
		ResourceType: portainer.PlatformRuntimeResourceContainer,
		ResourceID:   "container-1",
		Name:         "orders-api",
	}
	require.NoError(t, ctx.handler.DataStore.PlatformServiceDeployment().Update(deployment.ID, &deployment))
	ctx.handler.RuntimeInspector = fakeRuntimeInspector{logs: platformservice.RuntimeLogResult{
		Available: true,
		Logs:      "line-1\nline-2\n",
	}}

	logs := doJSON[serviceDeploymentLogsResponse](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/logs?tail=2", deployment.ID), nil, http.StatusOK)
	require.True(t, logs.Available)
	require.Equal(t, 2, logs.Tail)
	require.Contains(t, logs.Logs, "line-2")
}

func TestPlatformReleaseResolveAcceptsCurrentAndAudits(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})
	release := &portainer.PlatformRelease{
		ProjectID:            project.ID,
		EnvironmentID:        deployment.EnvironmentID,
		ApplicationID:        application.ID,
		ServiceDefinitionID:  service.ID,
		ServiceDeploymentID:  deployment.ID,
		ArtifactID:           artifact.ID,
		Version:              "manual",
		TriggerType:          portainer.PlatformReleaseTriggerDeploy,
		Strategy:             portainer.NewPlatformReleaseStrategy(),
		Status:               portainer.PlatformReleaseStatusRecoveryFailed,
		ExpectedSpecRevision: deployment.SpecRevision,
		Image:                artifact.ImageRef,
		ConfigSnapshot:       portainer.PlatformServiceConfigSnapshot{SpecRevision: deployment.SpecRevision, DesiredSpecSnapshot: deployment.DesiredSpec},
		RuntimeSnapshot: portainer.PlatformRuntimeSnapshot{
			CurrentRuntimeRef: portainer.RuntimeRef{
				DriverID:     portainer.PlatformRuntimeDriverDockerContainer,
				EndpointID:   1,
				ResourceType: portainer.PlatformRuntimeResourceContainer,
				ResourceID:   "current-container",
				Name:         "orders-api-current",
			},
		},
		FailureReason:        platformservice.ReleaseFailureReasonRecoveryFailed,
		ManualActionRequired: true,
		LeaseOwner:           "executor",
		LeaseExpiresAt:       time.Now().Unix() + 600,
	}
	require.NoError(t, ctx.handler.DataStore.PlatformRelease().Create(release))
	require.NoError(t, ctx.handler.DataStore.PlatformReleaseLock().Create(&portainer.PlatformReleaseLock{
		ServiceDeploymentID: deployment.ID,
		ReleaseID:           release.ID,
		LeaseOwner:          "executor",
		LeaseExpiresAt:      release.LeaseExpiresAt,
	}))

	resolved := doJSON[portainer.PlatformRelease](t, ctx, http.MethodPost, fmt.Sprintf("/platform/releases/%d/resolve", release.ID), resolveReleasePayload{
		Action:  portainer.PlatformReleaseResolutionAcceptCurrent,
		Comment: "checked manually",
	}, http.StatusOK)
	require.Equal(t, portainer.PlatformReleaseStatusResolved, resolved.Status)
	require.Equal(t, portainer.PlatformReleaseResolutionAcceptCurrent, resolved.ResolutionAction)
	require.False(t, resolved.ManualActionRequired)

	updatedDeployment, err := ctx.handler.DataStore.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, release.ID, updatedDeployment.CurrentServingReleaseID)
	require.Equal(t, artifact.ID, updatedDeployment.CurrentArtifactID)
	require.Equal(t, "current-container", updatedDeployment.CurrentRuntimeRef.ResourceID)
	require.Equal(t, portainer.PlatformDeploymentDriftNone, updatedDeployment.DriftStatus)

	_, err = ctx.handler.DataStore.PlatformReleaseLock().Read(platformreleaselock.LockIDForServiceDeployment(deployment.ID))
	require.Error(t, err)
	require.True(t, ctx.handler.DataStore.IsErrObjectNotFound(err))

	audits := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?releaseId=%d&action=%s", release.ID, portainer.PlatformAuditActionReleaseResolved), nil, http.StatusOK)
	require.Len(t, audits, 1)
	require.Equal(t, portainer.PlatformAuditResultSuccess, audits[0].Result)
}

func TestPlatformReleaseRetryRecoveryReleasesLockAndAudits(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	executor := &fakeReleaseRecoveryExecutor{}
	ctx.handler.ReleaseRecoveryExecutor = executor
	deployment.CurrentRuntimeRef = portainer.RuntimeRef{ResourceID: "previous-container", Name: "orders-api-previous"}
	require.NoError(t, ctx.handler.DataStore.PlatformServiceDeployment().Update(deployment.ID, &deployment))

	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})
	release := createRecoveryFailedRelease(t, ctx, project, application, service, deployment, artifact)

	retried := doJSON[portainer.PlatformRelease](t, ctx, http.MethodPost, fmt.Sprintf("/platform/releases/%d/retry-recovery", release.ID), nil, http.StatusOK)
	require.Equal(t, portainer.PlatformReleaseStatusFailed, retried.Status)
	require.False(t, retried.ManualActionRequired)
	require.Equal(t, release.ID, executor.retryRequest.Release.ID)
	require.Equal(t, "previous-container", executor.retryRequest.Deployment.CurrentRuntimeRef.ResourceID)

	_, err := ctx.handler.DataStore.PlatformReleaseLock().Read(platformreleaselock.LockIDForServiceDeployment(deployment.ID))
	require.Error(t, err)
	require.True(t, ctx.handler.DataStore.IsErrObjectNotFound(err))

	audits := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?releaseId=%d&action=%s", release.ID, portainer.PlatformAuditActionReleaseRetryRecovery), nil, http.StatusOK)
	require.Len(t, audits, 1)
	require.Equal(t, portainer.PlatformAuditResultSuccess, audits[0].Result)
}

func TestPlatformReleaseCleanupRuntimeKeepsRecoveryLockAndAudits(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	executor := &fakeReleaseRecoveryExecutor{
		cleanupResult: platformservice.ReleaseCleanupResult{
			DeletedRuntimeRefs: []portainer.RuntimeRef{{ResourceID: "candidate-container"}, {ResourceID: "failed-current"}},
		},
	}
	ctx.handler.ReleaseRecoveryExecutor = executor

	artifact := createImageReferenceArtifact(t, ctx, createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "orders-api",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/orders-api:1.0.0",
	})
	release := createRecoveryFailedRelease(t, ctx, project, application, service, deployment, artifact)

	result := doJSON[platformservice.ReleaseCleanupResult](t, ctx, http.MethodPost, fmt.Sprintf("/platform/releases/%d/cleanup-runtime", release.ID), nil, http.StatusOK)
	require.Equal(t, portainer.PlatformReleaseStatusRecoveryFailed, result.Release.Status)
	require.Len(t, result.DeletedRuntimeRefs, 2)
	require.Equal(t, "cleanup-runtime", result.Release.Steps[len(result.Release.Steps)-1].Name)

	lock, err := ctx.handler.DataStore.PlatformReleaseLock().Read(platformreleaselock.LockIDForServiceDeployment(deployment.ID))
	require.NoError(t, err)
	require.Equal(t, release.ID, lock.ReleaseID)

	audits := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?releaseId=%d&action=%s", release.ID, portainer.PlatformAuditActionReleaseCleanupRuntime), nil, http.StatusOK)
	require.Len(t, audits, 1)
	require.Equal(t, portainer.PlatformAuditResultSuccess, audits[0].Result)
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

func createRecoveryFailedRelease(t *testing.T, ctx platformTestContext, project portainer.PlatformProject, application portainer.PlatformApplication, service portainer.PlatformServiceDefinition, deployment portainer.PlatformServiceDeployment, artifact portainer.PlatformArtifact) *portainer.PlatformRelease {
	t.Helper()

	release := &portainer.PlatformRelease{
		ProjectID:            project.ID,
		EnvironmentID:        deployment.EnvironmentID,
		ApplicationID:        application.ID,
		ServiceDefinitionID:  service.ID,
		ServiceDeploymentID:  deployment.ID,
		ArtifactID:           artifact.ID,
		Version:              "manual",
		TriggerType:          portainer.PlatformReleaseTriggerDeploy,
		Strategy:             portainer.NewPlatformReleaseStrategy(),
		Status:               portainer.PlatformReleaseStatusRecoveryFailed,
		ExpectedSpecRevision: deployment.SpecRevision,
		Image:                artifact.ImageRef,
		ConfigSnapshot:       portainer.PlatformServiceConfigSnapshot{SpecRevision: deployment.SpecRevision, DesiredSpecSnapshot: deployment.DesiredSpec},
		RuntimeSnapshot: portainer.PlatformRuntimeSnapshot{
			CandidateRuntimeRef: portainer.RuntimeRef{ResourceID: "candidate-container"},
			CurrentRuntimeRef:   portainer.RuntimeRef{ResourceID: "failed-current"},
		},
		FailureReason:        platformservice.ReleaseFailureReasonRecoveryFailed,
		ManualActionRequired: true,
		LeaseOwner:           "executor",
		LeaseExpiresAt:       time.Now().Unix() + 600,
	}
	require.NoError(t, ctx.handler.DataStore.PlatformRelease().Create(release))
	require.NoError(t, ctx.handler.DataStore.PlatformReleaseLock().Create(&portainer.PlatformReleaseLock{
		ServiceDeploymentID: deployment.ID,
		ReleaseID:           release.ID,
		LeaseOwner:          "executor",
		LeaseExpiresAt:      release.LeaseExpiresAt,
	}))

	return release
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
