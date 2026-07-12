package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestGatewayCreateListAndInspect(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Gateway project", Slug: "gateway-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	endpoint := &portainer.Endpoint{ID: 31, Name: "gateway", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))

	created := doJSON[portainer.PlatformGateway](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "prod-gateway"}, http.StatusCreated)
	require.Equal(t, project.ID, created.ProjectID)
	require.Equal(t, endpoint.ID, created.EndpointID)

	items := doJSON[[]portainer.PlatformGateway](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), nil, http.StatusOK)
	require.Len(t, items, 1)
	inspected := doJSON[portainer.PlatformGateway](t, ctx, http.MethodGet, fmt.Sprintf("/platform/gateways/%d", created.ID), nil, http.StatusOK)
	require.Equal(t, created.Name, inspected.Name)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: 999, Name: "missing"}, http.StatusNotFound)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "duplicate"}, http.StatusConflict)
}

func TestGatewayRouteCreateValidatesDeploymentAndConflict(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Route project", Slug: "route-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	endpoint := &portainer.Endpoint{ID: 32, Name: "gateway", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	gateway := doJSON[portainer.PlatformGateway](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "prod-gateway"}, http.StatusCreated)
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "API", Slug: "api"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "API", Slug: "api"})
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/api:1"
	spec.Ports = []portainer.PlatformPortSpec{{Name: "http", ContainerPort: 8080, HostPort: 18080}}
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{EnvironmentID: environment.ID, DesiredSpec: &spec})
	payload := createGatewayRoutePayload{ServiceDeploymentID: deployment.ID, Domain: "api.example.test", Path: "/api", TargetPort: 8080}
	created := doJSON[portainer.PlatformGatewayRoute](t, ctx, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusCreated)
	require.Equal(t, deployment.ID, created.ServiceDeploymentID)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusConflict)
	payload.Path, payload.TargetPort = "/invalid", 9090
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusBadRequest)
}
