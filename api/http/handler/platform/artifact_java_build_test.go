package platform

import (
	"archive/zip"
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

type fakeJavaBuilder struct {
	result  platformservice.JavaImageBuildResult
	err     error
	builds  int
	cleanup []string
}

func (f *fakeJavaBuilder) Build(_ context.Context, request platformservice.JavaImageBuildRequest) (platformservice.JavaImageBuildResult, error) {
	f.builds++
	_, _ = io.Copy(io.Discard, request.Context)
	if f.err != nil {
		return platformservice.JavaImageBuildResult{}, f.err
	}
	r := f.result
	if r.CandidateRef == "" {
		r.CandidateRef = request.CandidateRef
	}
	return r, nil
}
func (f *fakeJavaBuilder) Cleanup(_ context.Context, _ int, c string) error {
	f.cleanup = append(f.cleanup, c)
	return nil
}

func TestPlatformJavaBuildUsesControlledTemplateAndCleansFailure(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Java Project", Slug: "java-project"})
	app := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Java App", Slug: "java-app"})
	service := createServiceDefinition(t, ctx, app.ID, createServiceDefinitionPayload{Name: "Java Service", Slug: "java-service", Type: portainer.PlatformServiceTypeJavaService})
	endpoint := &portainer.Endpoint{ID: 88, Name: "java-endpoint", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	artifact := uploadJavaArtifact(t, ctx, project, app, service, "1.0.0")
	builder := &fakeJavaBuilder{result: platformservice.JavaImageBuildResult{ImageID: "sha256:java"}}
	ctx.handler.JavaImageBuilder = builder
	result := doJSON[portainer.PlatformArtifact](t, ctx, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/build-java", artifact.ID), buildJavaArtifactPayload{EndpointID: endpoint.ID, JVMArgs: []string{"-Xmx512m"}, AppArgs: []string{"--server.port=8080"}, Port: 8080}, http.StatusOK)
	require.Equal(t, portainer.PlatformArtifactStatusBuilt, result.Status)
	require.Equal(t, platformservice.Java8BuildTemplate, result.BuildTemplate)
	require.Equal(t, "sha256:java", result.CandidateImageID)
	failed := uploadJavaArtifact(t, ctx, project, app, service, "2.0.0")
	builder.err = errors.New("build failed")
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifacts/%d/build-java", failed.ID), buildJavaArtifactPayload{EndpointID: endpoint.ID, Port: 8080}, http.StatusBadRequest)
	persisted, err := ctx.handler.DataStore.PlatformArtifact().Read(failed.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformArtifactStatusFailed, persisted.Status)
	require.NotEmpty(t, builder.cleanup)
}
func uploadJavaArtifact(t *testing.T, ctx platformTestContext, p portainer.PlatformProject, a portainer.PlatformApplication, s portainer.PlatformServiceDefinition, version string) portainer.PlatformArtifact {
	t.Helper()
	r := doRawMultipart(t, ctx, ctx.adminJWT, map[string]string{"ProjectId": fmt.Sprint(p.ID), "ApplicationId": fmt.Sprint(a.ID), "ServiceDefinitionId": fmt.Sprint(s.ID), "Name": "java-service", "Version": version, "Type": "java-jar"}, "service.jar", javaJarFixture(t), http.StatusCreated)
	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &artifact))
	return artifact
}
func javaJarFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create("META-INF/MANIFEST.MF")
	require.NoError(t, err)
	_, err = f.Write([]byte("Manifest-Version: 1.0\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return b.Bytes()
}
