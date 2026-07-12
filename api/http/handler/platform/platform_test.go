package platform

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/apikey"
	"github.com/portainer/portainer/api/datastore"
	"github.com/portainer/portainer/api/filesystem"
	"github.com/portainer/portainer/api/http/security"
	"github.com/portainer/portainer/api/internal/testhelpers"
	"github.com/portainer/portainer/api/jwt"

	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

type platformTestContext struct {
	handler     *Handler
	adminJWT    string
	standardJWT string
	fileService *filesystem.Service
}

func newPlatformTestContext(t *testing.T) platformTestContext {
	t.Helper()

	_, store := datastore.MustNewTestStore(t, true, true)

	adminUser := &portainer.User{ID: 1, Username: "admin", Role: portainer.AdministratorRole}
	require.NoError(t, store.User().Create(adminUser))

	standardUser := &portainer.User{ID: 2, Username: "standard", Role: portainer.StandardUserRole}
	require.NoError(t, store.User().Create(standardUser))

	jwtService, err := jwt.NewService("1h", store)
	require.NoError(t, err)

	apiKeyService := apikey.NewAPIKeyService(store.APIKeyRepository(), store.User())
	requestBouncer := security.NewRequestBouncer(t.Context(), store, jwtService, apiKeyService)

	handler := NewHandler(requestBouncer)
	handler.DataStore = store
	fileService, err := filesystem.NewService(t.TempDir(), "")
	require.NoError(t, err)
	handler.FileService = fileService
	handler.GatewayRuntime = platformGatewayRuntimeFake{}

	adminJWT, _, err := jwtService.GenerateToken(&portainer.TokenData{ID: adminUser.ID, Username: adminUser.Username, Role: adminUser.Role})
	require.NoError(t, err)

	standardJWT, _, err := jwtService.GenerateToken(&portainer.TokenData{ID: standardUser.ID, Username: standardUser.Username, Role: standardUser.Role})
	require.NoError(t, err)

	return platformTestContext{
		handler:     handler,
		adminJWT:    adminJWT,
		standardJWT: standardJWT,
		fileService: fileService,
	}
}

// platformGatewayRuntimeFake 让 handler 测试覆盖控制面状态机，不依赖本机 Docker daemon。
// Docker/Agent 行为由 api/platform 的运行时适配器单测覆盖，避免测试误操作真实容器。
type platformGatewayRuntimeFake struct{}

func (platformGatewayRuntimeFake) Ensure(context.Context, portainer.PlatformGateway, string) (string, error) {
	return "platform-test-gateway", nil
}

func (platformGatewayRuntimeFake) Test(context.Context, portainer.PlatformGateway, string) error {
	return nil
}

func (platformGatewayRuntimeFake) Reload(context.Context, portainer.PlatformGateway) error {
	return nil
}

func TestPlatformCRUDAdminCreatesFullChain(t *testing.T) {
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

	require.Equal(t, project.ID, deployment.ProjectID)
	require.Equal(t, application.ID, deployment.ApplicationID)
	require.Equal(t, service.ID, deployment.ServiceDefinitionID)
	require.Equal(t, environment.ID, deployment.EnvironmentID)
	require.Equal(t, 1, deployment.SpecRevision)
	require.Equal(t, portainer.PlatformDeploymentDriftNone, deployment.DriftStatus)

	deployments := doJSON[[]portainer.PlatformServiceDeployment](t, ctx, http.MethodGet, fmt.Sprintf("/platform/services/%d/deployments", service.ID), nil, http.StatusOK)
	require.Len(t, deployments, 1)
	require.Equal(t, deployment.ID, deployments[0].ID)

	projects := doJSON[[]portainer.PlatformProject](t, ctx, http.MethodGet, "/platform/projects", nil, http.StatusOK)
	require.Len(t, projects, 1)

	updatedProject := doJSON[portainer.PlatformProject](t, ctx, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{
		ResourceVersion: project.ResourceVersion,
		Name:            stringPtr("Customer A Renamed"),
	}, http.StatusOK)
	require.Equal(t, "Customer A Renamed", updatedProject.Name)
	require.Equal(t, project.ResourceVersion+1, updatedProject.ResourceVersion)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{
		ResourceVersion: project.ResourceVersion,
		Name:            stringPtr("stale"),
	}, http.StatusConflict)

	spec.Image.Image = "registry.example.com/orders-api:2.0.0"
	updatedDeployment := doJSON[portainer.PlatformServiceDeployment](t, ctx, http.MethodPut, fmt.Sprintf("/platform/service-deployments/%d", deployment.ID), updateServiceDeploymentPayload{
		ResourceVersion: deployment.ResourceVersion,
		DesiredSpec:     &spec,
	}, http.StatusOK)
	require.Equal(t, 2, updatedDeployment.SpecRevision)
	require.Equal(t, deployment.ResourceVersion+1, updatedDeployment.ResourceVersion)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/service-deployments/%d", deployment.ID), nil, http.StatusNoContent)
	archivedDeployment := doJSON[portainer.PlatformServiceDeployment](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d", deployment.ID), nil, http.StatusOK)
	require.Equal(t, portainer.PlatformLifecycleStatusArchived, archivedDeployment.LifecycleStatus)
}

func TestPlatformProjectListHidesUnassignedProjects(t *testing.T) {
	ctx := newPlatformTestContext(t)

	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, "/platform/projects", nil, http.StatusOK)
}

func TestPlatformProjectArchiveIsHiddenFromDefaultList(t *testing.T) {
	ctx := newPlatformTestContext(t)

	project := createProject(t, ctx, createProjectPayload{Name: "Archive Me", Slug: "archive-me"})
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/projects/%d", project.ID), nil, http.StatusNoContent)

	projects := doJSON[[]portainer.PlatformProject](t, ctx, http.MethodGet, "/platform/projects", nil, http.StatusOK)
	require.Empty(t, projects)

	archivedProjects := doJSON[[]portainer.PlatformProject](t, ctx, http.MethodGet, "/platform/projects?includeArchived=true", nil, http.StatusOK)
	require.Len(t, archivedProjects, 1)
	require.Equal(t, portainer.PlatformLifecycleStatusArchived, archivedProjects[0].LifecycleStatus)
	require.Equal(t, portainer.UserID(1), archivedProjects[0].ArchivedByUserID)
}

func createProject(t *testing.T, ctx platformTestContext, payload createProjectPayload) portainer.PlatformProject {
	t.Helper()

	return doJSON[portainer.PlatformProject](t, ctx, http.MethodPost, "/platform/projects", payload, http.StatusCreated)
}

func createEnvironment(t *testing.T, ctx platformTestContext, projectID portainer.PlatformProjectID, payload createEnvironmentPayload) portainer.PlatformEnvironment {
	t.Helper()

	return doJSON[portainer.PlatformEnvironment](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/environments", projectID), payload, http.StatusCreated)
}

func createApplication(t *testing.T, ctx platformTestContext, projectID portainer.PlatformProjectID, payload createApplicationPayload) portainer.PlatformApplication {
	t.Helper()

	return doJSON[portainer.PlatformApplication](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/applications", projectID), payload, http.StatusCreated)
}

func createServiceDefinition(t *testing.T, ctx platformTestContext, applicationID portainer.PlatformApplicationID, payload createServiceDefinitionPayload) portainer.PlatformServiceDefinition {
	t.Helper()

	return doJSON[portainer.PlatformServiceDefinition](t, ctx, http.MethodPost, fmt.Sprintf("/platform/applications/%d/services", applicationID), payload, http.StatusCreated)
}

func createServiceDeployment(t *testing.T, ctx platformTestContext, serviceID portainer.PlatformServiceDefinitionID, payload createServiceDeploymentPayload) portainer.PlatformServiceDeployment {
	t.Helper()

	return doJSON[portainer.PlatformServiceDeployment](t, ctx, http.MethodPost, fmt.Sprintf("/platform/services/%d/deployments", serviceID), payload, http.StatusCreated)
}

func doJSON[T any](t *testing.T, ctx platformTestContext, method string, target string, payload any, expectedStatus int) T {
	t.Helper()

	recorder := doRawJSON(t, ctx, ctx.adminJWT, method, target, payload, expectedStatus)

	var result T
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))

	return result
}

func doRawJSON(t *testing.T, ctx platformTestContext, token string, method string, target string, payload any, expectedStatus int) *httptest.ResponseRecorder {
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
	testhelpers.AddTestSecurityCookie(req, token)

	recorder := httptest.NewRecorder()
	ctx.handler.ServeHTTP(recorder, req)
	require.Equal(t, expectedStatus, recorder.Code, recorder.Body.String())

	return recorder
}

func doRawMultipart(t *testing.T, ctx platformTestContext, token string, fields map[string]string, fileName string, fileContent []byte, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value))
	}
	file, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = file.Write(fileContent)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/platform/artifacts/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	testhelpers.AddTestSecurityCookie(req, token)

	recorder := httptest.NewRecorder()
	ctx.handler.ServeHTTP(recorder, req)
	require.Equal(t, expectedStatus, recorder.Code, recorder.Body.String())

	return recorder
}

func TestPlatformArtifactUploadCreatesTraceableArtifactAndAudit(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Artifact Project", Slug: "artifact-project"})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Artifact App", Slug: "artifact-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Artifact Service", Slug: "artifact-service", Type: portainer.PlatformServiceTypeJavaService})

	recorder := doRawMultipart(t, ctx, ctx.adminJWT, map[string]string{
		"ProjectId":           fmt.Sprint(project.ID),
		"ApplicationId":       fmt.Sprint(application.ID),
		"ServiceDefinitionId": fmt.Sprint(service.ID),
		"Name":                "artifact-service",
		"Version":             "1.0.0",
		"Type":                string(portainer.PlatformArtifactTypeJavaJar),
	}, "service.jar", []byte("PK\x03\x04fixture"), http.StatusCreated)

	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &artifact))
	require.Equal(t, portainer.PlatformArtifactStatusUploaded, artifact.Status)
	require.True(t, artifact.Retained)
	require.False(t, artifact.Cleanable)
	require.NotEmpty(t, artifact.SHA256)
	require.Empty(t, artifact.StoragePath)
	persistedArtifact, err := ctx.handler.DataStore.PlatformArtifact().Read(artifact.ID)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(ctx.fileService.GetDatastorePath(), "platform-artifacts", filepath.FromSlash(persistedArtifact.StoragePath)))
	require.NoError(t, err)

	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ArtifactID == artifact.ID
	})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, portainer.PlatformAuditActionArtifactUploaded, audits[0].Action)
	require.NotContains(t, fmt.Sprint(audits[0].AfterSummary), ctx.fileService.GetDatastorePath())
}

func TestPlatformArtifactUploadRejectsHashMismatchWithoutPersistingFile(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Mismatch Project", Slug: "mismatch-project"})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Mismatch App", Slug: "mismatch-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Mismatch Service", Slug: "mismatch-service"})

	doRawMultipart(t, ctx, ctx.adminJWT, map[string]string{
		"ProjectId":           fmt.Sprint(project.ID),
		"ApplicationId":       fmt.Sprint(application.ID),
		"ServiceDefinitionId": fmt.Sprint(service.ID),
		"Name":                "mismatch-service",
		"Version":             "1.0.0",
		"Type":                string(portainer.PlatformArtifactTypeJavaJar),
		"ExpectedSHA256":      strings.Repeat("0", 64),
	}, "service.jar", []byte("PK\x03\x04fixture"), http.StatusBadRequest)

	artifacts, err := ctx.handler.DataStore.PlatformArtifact().ReadAll()
	require.NoError(t, err)
	require.Empty(t, artifacts)
	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll(func(audit portainer.PlatformAuditLog) bool {
		return audit.ProjectID == project.ID
	})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, portainer.PlatformAuditActionArtifactUploadFailed, audits[0].Action)
	require.Equal(t, "SHA256_MISMATCH", audits[0].FailureReason)
	require.NotContains(t, fmt.Sprint(audits[0].AfterSummary), "0000000000000000")
	entries, err := os.ReadDir(filepath.Join(ctx.fileService.GetDatastorePath(), "platform-artifacts", "uploads"))
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestPlatformArtifactUploadRejectsUnauthorizedRequestBeforeWritingFile(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Private Artifact Project", Slug: "private-artifact-project"})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Private Artifact App", Slug: "private-artifact-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Private Artifact Service", Slug: "private-artifact-service"})

	doRawMultipart(t, ctx, ctx.standardJWT, map[string]string{
		"ProjectId":           fmt.Sprint(project.ID),
		"ApplicationId":       fmt.Sprint(application.ID),
		"ServiceDefinitionId": fmt.Sprint(service.ID),
		"Name":                "private-artifact-service",
		"Version":             "1.0.0",
		"Type":                string(portainer.PlatformArtifactTypeJavaJar),
	}, "service.jar", []byte("PK\x03\x04fixture"), http.StatusForbidden)

	_, err := os.Stat(filepath.Join(ctx.fileService.GetDatastorePath(), "platform-artifacts"))
	require.True(t, os.IsNotExist(err))
}

func stringPtr(value string) *string {
	return &value
}
