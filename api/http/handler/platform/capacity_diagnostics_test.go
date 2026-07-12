package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestPlatformCapacitySummaryAndFailureDiagnosticsStayMetadataOnly(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Capacity Project", Slug: "capacity-project"})
	first := &portainer.PlatformArtifact{ProjectID: project.ID, Name: "first", Version: "1", Type: portainer.PlatformArtifactTypeImage, SourceType: portainer.PlatformArtifactSourceImageReference, ImageRef: "registry.example.com/first:1", Traceability: portainer.PlatformTraceabilityWeak, Size: 100, Retained: true, PlatformLifecycle: portainer.NewPlatformLifecycle()}
	second := &portainer.PlatformArtifact{ProjectID: project.ID, Name: "second", Version: "1", Type: portainer.PlatformArtifactTypeImage, SourceType: portainer.PlatformArtifactSourceImageReference, ImageRef: "registry.example.com/second:1", Traceability: portainer.PlatformTraceabilityWeak, Size: 200, Retained: true, Cleanable: true, PlatformLifecycle: portainer.NewPlatformLifecycle()}
	require.NoError(t, ctx.handler.DataStore.PlatformArtifact().Create(first))
	require.NoError(t, ctx.handler.DataStore.PlatformArtifact().Create(second))
	require.NoError(t, ctx.handler.DataStore.PlatformAuditLog().Create(&portainer.PlatformAuditLog{Timestamp: 100, ProjectID: project.ID, Action: portainer.PlatformAuditActionReleaseFailed, Result: portainer.PlatformAuditResultFailed, FailureReason: "DATABASE_UNREACHABLE"}))

	summary := doJSON[platformCapacitySummary](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/capacity-summary?warningThresholdBytes=250", project.ID), nil, http.StatusOK)
	require.Equal(t, 2, summary.ArtifactCount)
	require.Equal(t, int64(300), summary.ArtifactBytes)
	require.Equal(t, int64(300), summary.RetainedArtifactBytes)
	require.Equal(t, int64(200), summary.CleanableArtifactBytes)
	require.Equal(t, "warning", summary.ThresholdStatus)
	require.Equal(t, "not-supported", summary.ObjectStorageCapacity)

	diagnostics := doJSON[platformFailureDiagnosticsResponse](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/failure-diagnostics?from=1&to=200", project.ID), nil, http.StatusOK)
	require.Equal(t, []platformFailureDiagnostic{{Source: "audit", Reason: "DATABASE_UNREACHABLE", Count: 1}}, diagnostics.Items)
}
