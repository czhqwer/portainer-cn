package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

type fakeDatabaseResourceProbe struct {
	password string
	err      error
}

func (probe *fakeDatabaseResourceProbe) Probe(_ context.Context, _ portainer.PlatformDatabaseResource, password string) error {
	probe.password = password
	return probe.err
}

func TestPlatformDatabaseResourceBindingKeepsCredentialPrivate(t *testing.T) {
	ctx := newPlatformTestContext(t)
	probe := &fakeDatabaseResourceProbe{}
	ctx.handler.DatabaseResourceProbe = probe
	project := createProject(t, ctx, createProjectPayload{Name: "Database Project", Slug: "database-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{
		Name: "Database", Slug: "database", Type: portainer.PlatformEnvironmentTypeDev,
		Targets: []portainer.PlatformDeploymentTarget{{EndpointID: 1, Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}},
	})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "Database App", Slug: "database-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "Database API", Slug: "database-api"})
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/database-api:1.0.0"
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{EnvironmentID: environment.ID, DesiredSpec: &spec})

	password := "p@ssw0rd"
	resourceRecorder := doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/projects/%d/database-resources", project.ID), createDatabaseResourcePayload{
		EnvironmentID: environment.ID, EndpointID: 1, Name: "orders-db", Type: portainer.PlatformDatabaseTypePostgres,
		Host: "orders-db.internal", Port: 5432, Database: "orders", Username: "orders_app", Password: &password,
	}, http.StatusCreated)
	require.NotContains(t, resourceRecorder.Body.String(), password)
	var resource portainer.PlatformDatabaseResource
	require.NoError(t, json.Unmarshal(resourceRecorder.Body.Bytes(), &resource))
	require.Empty(t, resource.PasswordCipherText)
	require.True(t, resource.HasPassword)
	persisted, err := ctx.handler.DataStore.PlatformDatabaseResource().Read(resource.ID)
	require.NoError(t, err)
	require.NotEmpty(t, persisted.PasswordCipherText)
	require.NotContains(t, persisted.PasswordCipherText, password)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/database-resources/%d/test", resource.ID), nil, http.StatusOK)
	require.Equal(t, password, probe.password)

	binding := doJSON[portainer.PlatformServiceDatabaseBinding](t, ctx, http.MethodPost, fmt.Sprintf("/platform/service-deployments/%d/database-bindings", deployment.ID), createDatabaseBindingPayload{DatabaseResourceID: resource.ID}, http.StatusCreated)
	bindings := doJSON[[]portainer.PlatformServiceDatabaseBinding](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/database-bindings", deployment.ID), nil, http.StatusOK)
	require.Len(t, bindings, 1)
	require.Equal(t, resource.ID, bindings[0].DatabaseResourceID)
	workbench := doJSON[databaseWorkbenchContext](t, ctx, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/database-workbench-context", deployment.ID), nil, http.StatusOK)
	require.Equal(t, resource.ID, workbench.DatabaseResourceID)
	require.Equal(t, "orders-db.internal", workbench.Host)

	// 活跃服务绑定必须阻止资源归档，防止下一次发布在运行期才发现凭据被移除。
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/database-resources/%d", resource.ID), nil, http.StatusConflict)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/database-bindings/%d", binding.ID), nil, http.StatusNoContent)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/database-resources/%d", resource.ID), nil, http.StatusNoContent)
}

func TestPlatformDatabaseResourceTestUsesStableFailureReason(t *testing.T) {
	ctx := newPlatformTestContext(t)
	probe := &fakeDatabaseResourceProbe{err: errors.New("dial tcp internal topology must not leak")}
	ctx.handler.DatabaseResourceProbe = probe
	project := createProject(t, ctx, createProjectPayload{Name: "Database Probe", Slug: "database-probe"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{
		Name: "Database", Slug: "database", Type: portainer.PlatformEnvironmentTypeDev,
		Targets: []portainer.PlatformDeploymentTarget{{EndpointID: 1, Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}},
	})
	password := "probe-secret"
	resource := doJSON[portainer.PlatformDatabaseResource](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/database-resources", project.ID), createDatabaseResourcePayload{
		EnvironmentID: environment.ID, EndpointID: 1, Name: "redis", Type: portainer.PlatformDatabaseTypeRedis,
		Host: "redis.internal", Password: &password,
	}, http.StatusCreated)

	result := doJSON[databaseResourceTestResponse](t, ctx, http.MethodPost, fmt.Sprintf("/platform/database-resources/%d/test", resource.ID), nil, http.StatusOK)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "DATABASE_UNREACHABLE", result.Reason)
}
