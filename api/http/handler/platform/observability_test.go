package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
)

func TestPlatformObservabilityConfigRequiresGlobalAdmin(t *testing.T) {
	ctx := newPlatformTestContext(t)
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, "/platform/observability-config", nil, http.StatusForbidden)
}

func TestPlatformObservabilityQueryRequiresEndpointPermission(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Observability", Slug: "observability", MemberPolicies: map[portainer.UserID]portainer.PlatformProjectRole{2: portainer.PlatformProjectRoleViewer}})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Development", Slug: "development", Type: portainer.PlatformEnvironmentTypeDev})

	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/projects/%d/observability?environmentId=%d&serviceDeploymentId=1&template=service-availability&start=100&end=160", project.ID, environment.ID), nil, http.StatusForbidden)
}
