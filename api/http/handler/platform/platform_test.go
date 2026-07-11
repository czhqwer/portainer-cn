package platform

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/apikey"
	"github.com/portainer/portainer/api/datastore"
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

	adminJWT, _, err := jwtService.GenerateToken(&portainer.TokenData{ID: adminUser.ID, Username: adminUser.Username, Role: adminUser.Role})
	require.NoError(t, err)

	standardJWT, _, err := jwtService.GenerateToken(&portainer.TokenData{ID: standardUser.ID, Username: standardUser.Username, Role: standardUser.Role})
	require.NoError(t, err)

	return platformTestContext{
		handler:     handler,
		adminJWT:    adminJWT,
		standardJWT: standardJWT,
	}
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

func stringPtr(value string) *string {
	return &value
}
