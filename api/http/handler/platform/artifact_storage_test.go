package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	portainer "github.com/portainer/portainer/api"
	platformservice "github.com/portainer/portainer/api/platform"
	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

type fakeArtifactStorageAdapter struct {
	objects        map[string][]byte
	testErr        error
	headErr        error
	downloadErr    error
	listErr        error
	testCalls      int
	headCalls      int
	lastConnection platformservice.ArtifactStorageConnection
}

func (adapter *fakeArtifactStorageAdapter) Test(_ context.Context, connection platformservice.ArtifactStorageConnection) error {
	adapter.testCalls++
	adapter.lastConnection = connection
	return adapter.testErr
}

func (adapter *fakeArtifactStorageAdapter) ListObjects(_ context.Context, connection platformservice.ArtifactStorageConnection, prefix string) ([]platformservice.ArtifactStorageObject, error) {
	adapter.lastConnection = connection
	if adapter.listErr != nil {
		return nil, adapter.listErr
	}
	keys := make([]string, 0, len(adapter.objects))
	for key := range adapter.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	objects := make([]platformservice.ArtifactStorageObject, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, platformservice.ArtifactStorageObject{Key: key, Size: int64(len(adapter.objects[key]))})
	}
	return objects, nil
}

func (adapter *fakeArtifactStorageAdapter) HeadObject(_ context.Context, connection platformservice.ArtifactStorageConnection, key string) (platformservice.ArtifactStorageObject, error) {
	adapter.headCalls++
	adapter.lastConnection = connection
	if adapter.headErr != nil {
		return platformservice.ArtifactStorageObject{}, adapter.headErr
	}
	value, found := adapter.objects[key]
	if !found {
		return platformservice.ArtifactStorageObject{}, errors.New("object not found")
	}
	return platformservice.ArtifactStorageObject{Key: key, Size: int64(len(value))}, nil
}

func (adapter *fakeArtifactStorageAdapter) DownloadObject(_ context.Context, connection platformservice.ArtifactStorageConnection, key string) (io.ReadCloser, error) {
	adapter.lastConnection = connection
	if adapter.downloadErr != nil {
		return nil, adapter.downloadErr
	}
	value, found := adapter.objects[key]
	if !found {
		return nil, errors.New("object not found")
	}
	return io.NopCloser(bytes.NewReader(value)), nil
}

func TestPlatformArtifactStorageConfigurationEncryptsCredentialsAndScopesProjects(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Storage Project", Slug: "storage-project"})
	adapter := &fakeArtifactStorageAdapter{}
	ctx.handler.ArtifactStorageAdapter = adapter

	const accessKey = "test-access-key"
	const secretKey = "test-secret-key"
	recorder := doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, "/platform/artifact-storages", createArtifactStoragePayload{
		Name:                 "delivery-minio",
		Endpoint:             "https://minio.example.test",
		Region:               "us-east-1",
		Bucket:               "artifacts",
		PathPrefix:           "releases",
		UseTLS:               true,
		AuthorizedProjectIDs: []portainer.PlatformProjectID{project.ID},
		AccessKey:            accessKey,
		SecretKey:            secretKey,
	}, http.StatusCreated)
	require.NotContains(t, recorder.Body.String(), accessKey)
	require.NotContains(t, recorder.Body.String(), secretKey)

	var storage artifactStorageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &storage))
	require.True(t, storage.CredentialsConfigured)
	persisted, err := ctx.handler.DataStore.PlatformArtifactStorage().Read(storage.ID)
	require.NoError(t, err)
	require.NotEqual(t, accessKey, persisted.AccessKeyCipherText)
	require.NotEqual(t, secretKey, persisted.SecretKeyCipherText)
	require.NotEmpty(t, persisted.CredentialHash)

	project = doJSON[portainer.PlatformProject](t, ctx, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{
		ResourceVersion: project.ResourceVersion,
		MemberPolicies:  map[portainer.UserID]portainer.PlatformProjectRole{2: portainer.PlatformProjectRoleDeveloper},
	}, http.StatusOK)
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, "/platform/artifact-storages", nil, http.StatusForbidden)
	available := doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/projects/%d/artifact-storages", project.ID), nil, http.StatusOK)
	require.NotContains(t, available.Body.String(), "minio.example.test")
	require.NotContains(t, available.Body.String(), "artifacts")

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/artifact-storages/%d/test", storage.ID), nil, http.StatusOK)
	require.Equal(t, 1, adapter.testCalls)
	require.Equal(t, accessKey, adapter.lastConnection.AccessKey)
	require.Equal(t, secretKey, adapter.lastConnection.SecretKey)

	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.Action == portainer.PlatformAuditActionArtifactStorageCreated || audit.Action == portainer.PlatformAuditActionArtifactStorageTested
	})
	require.NoError(t, err)
	require.Len(t, audits, 2)
	for _, audit := range audits {
		require.NotContains(t, fmt.Sprint(audit.AfterSummary), accessKey)
		require.NotContains(t, fmt.Sprint(audit.AfterSummary), secretKey)
		require.Contains(t, audit.SensitiveFields, "accessKey")
		require.Contains(t, audit.SensitiveFields, "secretKey")
	}
}

func TestPlatformObjectStorageFetchPersistsTraceableArtifactWithoutLeakingPaths(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Fetch Project", Slug: "fetch-project"})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Fetch App", Slug: "fetch-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Fetch Service", Slug: "fetch-service", Type: portainer.PlatformServiceTypeJavaService})
	content := []byte("PK\x03\x04jar fixture")
	adapter := &fakeArtifactStorageAdapter{objects: map[string][]byte{"releases/orders.jar": content}}
	ctx.handler.ArtifactStorageAdapter = adapter
	storage := createArtifactStorageForProject(t, ctx, project.ID)
	sum := sha256.Sum256(content)

	recorder := doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, "/platform/artifacts/from-storage", fetchObjectStorageArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		StorageID:           storage.ID,
		ObjectPath:          "orders.jar",
		Name:                "orders",
		Version:             "1.0.0",
		Type:                portainer.PlatformArtifactTypeJavaJar,
		ExpectedSHA256:      hex.EncodeToString(sum[:]),
	}, http.StatusCreated)
	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &artifact))
	require.Equal(t, portainer.PlatformArtifactStatusFetched, artifact.Status)
	require.Empty(t, artifact.StoragePath)
	require.Empty(t, artifact.SourcePath)
	persisted, err := ctx.handler.DataStore.PlatformArtifact().Read(artifact.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformArtifactSourceObjectStorage, persisted.SourceType)
	require.Equal(t, storage.ID, persisted.StorageID)
	require.Equal(t, "releases/orders.jar", persisted.SourcePath)
	require.True(t, strings.HasPrefix(persisted.StoragePath, "uploads/"))
	_, err = os.Stat(filepath.Join(ctx.fileService.GetDatastorePath(), "platform-artifacts", filepath.FromSlash(persisted.StoragePath)))
	require.NoError(t, err)

	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool { return audit.ArtifactID == artifact.ID })
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, portainer.PlatformAuditActionArtifactFetched, audits[0].Action)
	require.NotContains(t, fmt.Sprint(audits[0].AfterSummary), "releases/orders.jar")
	require.NotContains(t, fmt.Sprint(audits[0].AfterSummary), ctx.fileService.GetDatastorePath())

	objects := doRawJSON(t, ctx, ctx.adminJWT, http.MethodGet, fmt.Sprintf("/platform/artifact-storages/%d/objects?projectId=%d", storage.ID, project.ID), nil, http.StatusOK)
	require.Contains(t, objects.Body.String(), "orders.jar")
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodGet, fmt.Sprintf("/platform/artifact-storages/%d/objects?projectId=%d&prefix=../escape", storage.ID, project.ID), nil, http.StatusBadRequest)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodGet, fmt.Sprintf("/platform/artifact-storages/%d/objects?projectId=%d&prefix=/escape", storage.ID, project.ID), nil, http.StatusBadRequest)
}

func TestPlatformObjectStorageFetchCleansUpOnHashMismatchAndRejectsUnauthorizedProject(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Fetch Failure Project", Slug: "fetch-failure-project"})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Fetch Failure App", Slug: "fetch-failure-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Fetch Failure Service", Slug: "fetch-failure-service"})
	adapter := &fakeArtifactStorageAdapter{objects: map[string][]byte{"releases/orders.jar": []byte("PK\x03\x04jar fixture")}}
	ctx.handler.ArtifactStorageAdapter = adapter
	storage := createArtifactStorageForProject(t, ctx, project.ID)

	payload := fetchObjectStorageArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		StorageID:           storage.ID,
		ObjectPath:          "orders.jar",
		Name:                "orders",
		Version:             "1.0.0",
		Type:                portainer.PlatformArtifactTypeJavaJar,
		ExpectedSHA256:      strings.Repeat("0", 64),
	}
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, "/platform/artifacts/object-storage", payload, http.StatusBadRequest)
	artifacts, err := ctx.handler.DataStore.PlatformArtifact().ReadAll()
	require.NoError(t, err)
	require.Empty(t, artifacts)
	entries, err := os.ReadDir(filepath.Join(ctx.fileService.GetDatastorePath(), "platform-artifacts", "uploads"))
	if !os.IsNotExist(err) {
		require.NoError(t, err)
		require.Empty(t, entries)
	}
	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool { return audit.ProjectID == project.ID })
	require.NoError(t, err)
	require.Len(t, audits, 1)
	var fetchFailure *portainer.PlatformAuditLog
	for i := range audits {
		if audits[i].Action == portainer.PlatformAuditActionArtifactFetchFailed {
			fetchFailure = &audits[i]
			break
		}
	}
	require.NotNil(t, fetchFailure)
	require.Equal(t, "SHA256_MISMATCH", fetchFailure.FailureReason)
	require.NotContains(t, fmt.Sprint(fetchFailure.AfterSummary), "0000000000000000")

	headCalls := adapter.headCalls
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodPost, "/platform/artifacts/object-storage", payload, http.StatusForbidden)
	require.Equal(t, headCalls, adapter.headCalls)
}

func createArtifactStorageForProject(t *testing.T, ctx platformTestContext, projectID portainer.PlatformProjectID) artifactStorageResponse {
	t.Helper()
	return doJSON[artifactStorageResponse](t, ctx, http.MethodPost, "/platform/artifact-storages", createArtifactStoragePayload{
		Name:                 "test-minio-" + fmt.Sprint(projectID),
		Endpoint:             "https://minio.example.test",
		Bucket:               "artifacts",
		PathPrefix:           "releases",
		UseTLS:               true,
		AuthorizedProjectIDs: []portainer.PlatformProjectID{projectID},
		AccessKey:            "test-access-key",
		SecretKey:            "test-secret-key",
	}, http.StatusCreated)
}
