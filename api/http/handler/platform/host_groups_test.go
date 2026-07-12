package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestHostGroupCreateUpdateArchive(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Host group project", Slug: "host-group-project"})
	endpoint := &portainer.Endpoint{ID: 41, Name: "worker-a", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	payload := createHostGroupPayload{EnvironmentID: environment.ID, Name: "production-a", Targets: []portainer.PlatformDeploymentTarget{{EndpointID: endpoint.ID, NodeName: "node-a", HostAddress: "10.0.0.11", Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}}}

	created := doJSON[portainer.PlatformHostGroup](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/host-groups", project.ID), payload, http.StatusCreated)
	require.Equal(t, "production-a", created.Name)
	items := doJSON[[]portainer.PlatformHostGroup](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/host-groups", project.ID), nil, http.StatusOK)
	require.Len(t, items, 1)

	updated := doJSON[portainer.PlatformHostGroup](t, ctx, http.MethodPut, fmt.Sprintf("/platform/host-groups/%d", created.ID), updateHostGroupPayload{ResourceVersion: created.ResourceVersion, Name: "production-b", Targets: payload.Targets}, http.StatusOK)
	require.Equal(t, "production-b", updated.Name)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/host-groups/%d", created.ID), nil, http.StatusNoContent)
	items = doJSON[[]portainer.PlatformHostGroup](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/host-groups", project.ID), nil, http.StatusOK)
	require.Empty(t, items)
}
