package platform

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	portainer "github.com/portainer/portainer/api"
	platformservice "github.com/portainer/portainer/api/platform"
	"github.com/stretchr/testify/require"
)

type fakeRegistryImagePusher struct {
	result   platformservice.RegistryImagePushResult
	err      error
	requests []platformservice.RegistryImagePushRequest
}

func (pusher *fakeRegistryImagePusher) Push(_ context.Context, request platformservice.RegistryImagePushRequest) (platformservice.RegistryImagePushResult, error) {
	pusher.requests = append(pusher.requests, request)
	if pusher.err != nil {
		return platformservice.RegistryImagePushResult{}, pusher.err
	}
	result := pusher.result
	if result.ImageRef == "" {
		result.ImageRef = request.TargetRef
	}
	if result.ImageTag == "" {
		result.ImageTag = request.TargetRef
	}
	if result.ImageDigest == "" {
		result.ImageDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}
	return result, nil
}

func TestPlatformArtifactRegistryPushMakesFileArtifactReleaseReadyAndCleanupSafe(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	registry := createPlatformTestRegistry(t, ctx, 91)
	pusher := &fakeRegistryImagePusher{}
	ctx.handler.RegistryImagePusher = pusher
	artifact := createBuiltJavaArtifact(t, ctx, project, application, service, "1.0.0", "sha256:candidate-one")

	pushed := doJSON[portainer.PlatformArtifact](t, ctx, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/push", artifact.ID), pushArtifactPayload{EndpointID: 88, RegistryID: registry.ID}, http.StatusOK)
	require.Equal(t, portainer.PlatformArtifactStatusReady, pushed.Status)
	require.Equal(t, fmt.Sprintf("registry.example.test:5000/customer-a/orders-api:1.0.0-%d", pushed.ID), pushed.ImageRef)
	require.Equal(t, pushed.ImageRef, pushed.ImageTag)
	require.Equal(t, registry.ID, pushed.RegistryID)
	require.Empty(t, pushed.CandidateImageRef)
	require.Empty(t, pushed.CandidateImageID)
	require.True(t, pushed.Cleanable)
	require.Len(t, pusher.requests, 1)
	require.Equal(t, "sha256:candidate-one", pusher.requests[0].CandidateRef)

	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	validation := doJSON[releaseValidateResponse](t, ctx, http.MethodPost, "/platform/releases/validate", createReleasePayloadFor(project, application, service, deployment, pushed), http.StatusOK)
	require.True(t, validation.Valid)

	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ArtifactID == pushed.ID && audit.Action == portainer.PlatformAuditActionArtifactPushed
	})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.NotContains(t, fmt.Sprint(audits[0].AfterSummary), "Password")

	persistedBeforeCleanup, err := ctx.handler.DataStore.PlatformArtifact().Read(pushed.ID)
	require.NoError(t, err)
	rawPath, err := ctx.handler.artifactLocalFilePath(*persistedBeforeCleanup)
	require.NoError(t, err)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/cleanup-original", pushed.ID), nil, http.StatusNoContent)
	cleaned, err := ctx.handler.DataStore.PlatformArtifact().Read(pushed.ID)
	require.NoError(t, err)
	require.False(t, cleaned.Retained)
	require.False(t, cleaned.Cleanable)
	require.Empty(t, cleaned.StoragePath)
	_, err = os.Stat(rawPath)
	require.True(t, os.IsNotExist(err))
}

func TestPlatformArtifactRegistryPushFailurePreservesCandidateAndCleanupBlocksReleaseReferences(t *testing.T) {
	ctx, project, application, service, deployment := createPlatformReleaseFixture(t)
	registry := createPlatformTestRegistry(t, ctx, 92)
	pusher := &fakeRegistryImagePusher{err: &platformservice.RegistryPushError{Reason: "REGISTRY_AUTH_FAILED"}}
	ctx.handler.RegistryImagePusher = pusher
	failed := createBuiltJavaArtifact(t, ctx, project, application, service, "2.0.0", "sha256:candidate-failed")

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/push", failed.ID), pushArtifactPayload{EndpointID: 88, RegistryID: registry.ID}, http.StatusBadRequest)
	persisted, err := ctx.handler.DataStore.PlatformArtifact().Read(failed.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformArtifactStatusFailed, persisted.Status)
	require.Equal(t, "REGISTRY_AUTH_FAILED", persisted.FailureReason)
	require.Equal(t, "sha256:candidate-failed", persisted.CandidateImageRef)
	require.Empty(t, persisted.ImageRef)

	pusher.err = nil
	ready := createBuiltJavaArtifact(t, ctx, project, application, service, "3.0.0", "sha256:candidate-ready")
	ready = doJSON[portainer.PlatformArtifact](t, ctx, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/push", ready.ID), pushArtifactPayload{EndpointID: 88, RegistryID: registry.ID}, http.StatusOK)
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	releaseResponse := postReleaseExpectAccepted(t, ctx, "release-uses-artifact", createReleasePayloadFor(project, application, service, deployment, ready))
	waitForReleaseExecution(t, ctx, releaseResponse.ReleaseID)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/cleanup-original", ready.ID), nil, http.StatusConflict)
	blockedAudits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ArtifactID == ready.ID && audit.Action == portainer.PlatformAuditActionArtifactCleanupBlocked
	})
	require.NoError(t, err)
	require.Len(t, blockedAudits, 1)
	require.Equal(t, "ARTIFACT_CLEANUP_BLOCKED", blockedAudits[0].FailureReason)
}

func createPlatformTestRegistry(t *testing.T, ctx platformTestContext, id portainer.RegistryID) *portainer.Registry {
	t.Helper()
	registry := &portainer.Registry{
		ID:             id,
		Name:           "platform-registry",
		URL:            "registry.example.test:5000",
		Authentication: false,
	}
	require.NoError(t, ctx.handler.DataStore.Registry().Create(registry))
	return registry
}

func createBuiltJavaArtifact(t *testing.T, ctx platformTestContext, project portainer.PlatformProject, application portainer.PlatformApplication, service portainer.PlatformServiceDefinition, version, candidate string) portainer.PlatformArtifact {
	t.Helper()
	artifact := uploadJavaArtifact(t, ctx, project, application, service, version)
	persisted, err := ctx.handler.DataStore.PlatformArtifact().Read(artifact.ID)
	require.NoError(t, err)
	persisted.Status = portainer.PlatformArtifactStatusBuilt
	persisted.CandidateImageRef = candidate
	persisted.CandidateImageID = "sha256:local-image"
	require.NoError(t, ctx.handler.DataStore.PlatformArtifact().Update(persisted.ID, persisted))
	return *persisted
}
