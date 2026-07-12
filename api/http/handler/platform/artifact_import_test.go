package platform

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	platformservice "github.com/portainer/portainer/api/platform"
	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

type fakeArchiveImporter struct {
	result  platformservice.ArchiveImportResult
	err     error
	imports int
	cleanup []string
}

func (f *fakeArchiveImporter) Import(_ context.Context, request platformservice.ArchiveImportRequest) (platformservice.ArchiveImportResult, error) {
	f.imports++
	_, _ = io.Copy(io.Discard, request.Archive)
	if f.err != nil {
		return platformservice.ArchiveImportResult{}, f.err
	}
	result := f.result
	if result.CandidateRef == "" {
		result.CandidateRef = request.CandidateRef
	}
	return result, nil
}
func (f *fakeArchiveImporter) Cleanup(_ context.Context, _ int, candidate string) error {
	f.cleanup = append(f.cleanup, candidate)
	return nil
}

func TestPlatformArtifactArchiveImportTracksCandidateAndCleansFailure(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Import Project", Slug: "import-project"})
	app := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Import App", Slug: "import-app"})
	service := createServiceDefinition(t, ctx, app.ID, createServiceDefinitionPayload{Name: "Import Service", Slug: "import-service"})
	endpoint := &portainer.Endpoint{ID: 77, Name: "import-endpoint", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	archive := dockerTarFixture(t)
	recorder := doRawMultipart(t, ctx, ctx.adminJWT, map[string]string{"ProjectId": fmt.Sprint(project.ID), "ApplicationId": fmt.Sprint(app.ID), "ServiceDefinitionId": fmt.Sprint(service.ID), "Name": "import-service", "Version": "1.0.0", "Type": string(portainer.PlatformArtifactTypeDockerTar)}, "orders.tar", archive, http.StatusCreated)
	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &artifact))
	fake := &fakeArchiveImporter{result: platformservice.ArchiveImportResult{ImageID: "sha256:candidate", Architecture: "amd64"}}
	ctx.handler.ArchiveImageImporter = fake
	updated := doJSON[portainer.PlatformArtifact](t, ctx, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/import", artifact.ID), importArtifactPayload{EndpointID: endpoint.ID}, http.StatusOK)
	require.Equal(t, portainer.PlatformArtifactStatusBuilt, updated.Status)
	require.Equal(t, "sha256:candidate", updated.CandidateImageID)
	require.NotEmpty(t, updated.CandidateImageRef)
	require.Equal(t, 1, fake.imports)

	failed := createUploadedDockerArtifact(t, ctx, project, app, service, archive)
	fake.err = errors.New("docker failed")
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/import", failed.ID), importArtifactPayload{EndpointID: endpoint.ID}, http.StatusBadRequest)
	persisted, err := ctx.handler.DataStore.PlatformArtifact().Read(failed.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformArtifactStatusFailed, persisted.Status)
	require.Equal(t, "IMAGE_IMPORT_FAILED", persisted.FailureReason)
	require.NotEmpty(t, fake.cleanup)
}

func createUploadedDockerArtifact(t *testing.T, ctx platformTestContext, project portainer.PlatformProject, app portainer.PlatformApplication, service portainer.PlatformServiceDefinition, archive []byte) portainer.PlatformArtifact {
	t.Helper()
	recorder := doRawMultipart(t, ctx, ctx.adminJWT, map[string]string{"ProjectId": fmt.Sprint(project.ID), "ApplicationId": fmt.Sprint(app.ID), "ServiceDefinitionId": fmt.Sprint(service.ID), "Name": "import-service", "Version": "2.0.0", "Type": string(portainer.PlatformArtifactTypeDockerTar)}, "orders.tar", archive, http.StatusCreated)
	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &artifact))
	return artifact
}
func dockerTarFixture(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	files := map[string][]byte{"manifest.json": []byte(`[{"Config":"config.json","RepoTags":["example/orders:1.0.0"],"Layers":["layer.tar"]}]`), "config.json": []byte(`{"os":"linux","architecture":"amd64"}`), "layer.tar": []byte("layer")}
	for name, value := range files {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Size: int64(len(value)), Typeflag: tar.TypeReg}))
		_, err := writer.Write(value)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}
