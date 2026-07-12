package platform

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	platformservice "github.com/portainer/portainer/api/platform"
	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

func TestPlatformStaticBuildUsesControlledTemplateAndCleansFailures(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Static Project", Slug: "static-project"})
	app := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Static App", Slug: "static-app"})
	service := createServiceDefinition(t, ctx, app.ID, createServiceDefinitionPayload{Name: "Static Site", Slug: "static-site", Type: portainer.PlatformServiceTypeStaticSite})
	endpoint := &portainer.Endpoint{ID: 89, Name: "static-endpoint", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	builder := &fakeJavaBuilder{result: platformservice.JavaImageBuildResult{ImageID: "sha256:static"}}
	ctx.handler.StaticImageBuilder = builder

	artifact := uploadDistArtifact(t, ctx, project, app, service, "1.0.0", distZipFixture(t, map[string]string{"index.html": "ok"}))
	result := doJSON[portainer.PlatformArtifact](t, ctx, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/build-static", artifact.ID), buildStaticArtifactPayload{EndpointID: endpoint.ID, Mode: platformservice.DistModeSPA, CachePolicy: platformservice.DistCachePolicyImmutable}, http.StatusOK)
	require.Equal(t, portainer.PlatformArtifactStatusBuilt, result.Status)
	require.Equal(t, "static-nginx-v1:spa:immutable:no404", result.BuildTemplate)
	require.Equal(t, "sha256:static", result.CandidateImageID)

	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ArtifactID == artifact.ID
	})
	require.NoError(t, err)
	require.Len(t, audits, 2)
	require.Equal(t, portainer.PlatformAuditActionArtifactStaticBuilt, audits[1].Action)
	require.NotContains(t, fmt.Sprint(audits[1].AfterSummary), ctx.fileService.GetDatastorePath())

	failedBuild := uploadDistArtifact(t, ctx, project, app, service, "2.0.0", distZipFixture(t, map[string]string{"index.html": "ok"}))
	builder.err = errors.New("docker failed")
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/build-static", failedBuild.ID), buildStaticArtifactPayload{EndpointID: endpoint.ID, Mode: platformservice.DistModeSPA}, http.StatusBadRequest)
	persisted, err := ctx.handler.DataStore.PlatformArtifact().Read(failedBuild.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformArtifactStatusFailed, persisted.Status)
	require.Equal(t, "CONTROLLED_BUILD_FAILED", persisted.FailureReason)
	require.NotEmpty(t, builder.cleanup)

	missingEntry := uploadDistArtifact(t, ctx, project, app, service, "3.0.0", distZipFixture(t, map[string]string{"about.html": "about"}))
	builds := builder.builds
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/build-static", missingEntry.ID), buildStaticArtifactPayload{EndpointID: endpoint.ID, Mode: platformservice.DistModeSPA}, http.StatusBadRequest)
	require.Equal(t, builds, builder.builds)
	persisted, err = ctx.handler.DataStore.PlatformArtifact().Read(missingEntry.ID)
	require.NoError(t, err)
	require.Equal(t, "DIST_ENTRY_MISSING", persisted.FailureReason)
}

func uploadDistArtifact(t *testing.T, ctx platformTestContext, project portainer.PlatformProject, app portainer.PlatformApplication, service portainer.PlatformServiceDefinition, version string, archive []byte) portainer.PlatformArtifact {
	t.Helper()
	response := doRawMultipart(t, ctx, ctx.adminJWT, map[string]string{
		"ProjectId":           fmt.Sprint(project.ID),
		"ApplicationId":       fmt.Sprint(app.ID),
		"ServiceDefinitionId": fmt.Sprint(service.ID),
		"Name":                "static-site",
		"Version":             version,
		"Type":                "frontend-dist",
	}, "dist.zip", archive, http.StatusCreated)
	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &artifact))
	return artifact
}

func distZipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return archive.Bytes()
}
