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
