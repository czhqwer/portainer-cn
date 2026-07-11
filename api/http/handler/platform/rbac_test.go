package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/http/security"
	"github.com/segmentio/encoding/json"
	"github.com/stretchr/testify/require"
)

func TestProjectRoleForContextPrefersUserPolicyAndRanksTeams(t *testing.T) {
	project := &portainer.PlatformProject{
		MemberPolicies: map[portainer.UserID]portainer.PlatformProjectRole{
			2: portainer.PlatformProjectRoleViewer,
		},
		TeamPolicies: map[portainer.TeamID]portainer.PlatformProjectRole{
			10: portainer.PlatformProjectRoleAdmin,
			11: portainer.PlatformProjectRoleDeveloper,
		},
	}
	context := &security.RestrictedRequestContext{
		UserID:          2,
		UserMemberships: []portainer.TeamMembership{{TeamID: 10}, {TeamID: 11}},
	}

	// 用户级 viewer 显式降权必须覆盖团队 admin，避免团队默认权限越权。
	require.Equal(t, portainer.PlatformProjectRoleViewer, projectRoleForContext(project, context))
	require.True(t, projectRoleAllows(portainer.PlatformProjectRoleDeveloper, platformPermissionArtifactRelease))
	require.False(t, projectRoleAllows(portainer.PlatformProjectRoleDeveloper, platformPermissionSensitiveConfig))

	project.MemberPolicies = nil
	require.Equal(t, portainer.PlatformProjectRoleAdmin, projectRoleForContext(project, context))
}

func TestPlatformProjectRoleAndEndpointIntersectionAreEnforced(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "RBAC Project", Slug: "rbac-project"})

	// 未授权用户不能通过直接对象 URL 判断项目是否存在。
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/projects/%d", project.ID), nil, http.StatusNotFound)

	project = doJSON[portainer.PlatformProject](t, ctx, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{
		ResourceVersion: project.ResourceVersion,
		MemberPolicies:  map[portainer.UserID]portainer.PlatformProjectRole{2: portainer.PlatformProjectRoleViewer},
	}, http.StatusOK)
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/projects/%d", project.ID), nil, http.StatusOK)
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{ResourceVersion: project.ResourceVersion}, http.StatusForbidden)

	endpointGroup := &portainer.EndpointGroup{ID: 81, Name: "platform-rbac"}
	require.NoError(t, ctx.handler.DataStore.EndpointGroup().Create(endpointGroup))
	endpoint := &portainer.Endpoint{ID: 81, Name: "platform-rbac", GroupID: endpointGroup.ID, Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))

	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{
		Name: "RBAC", Slug: "rbac", Type: portainer.PlatformEnvironmentTypeDev,
		Targets: []portainer.PlatformDeploymentTarget{{EndpointID: endpoint.ID, Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}},
	})
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "RBAC App", Slug: "rbac-app"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "RBAC Service", Slug: "rbac-service"})
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/rbac:1.0.0"
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{EnvironmentID: environment.ID, DesiredSpec: &spec})

	// 即使具备项目 viewer，未获得 Portainer Endpoint 权限也不能读取运行态。
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/status", deployment.ID), nil, http.StatusForbidden)
	endpoint.UserAccessPolicies = portainer.UserAccessPolicies{2: {}}
	require.NoError(t, ctx.handler.DataStore.Endpoint().UpdateEndpoint(endpoint.ID, endpoint))
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/status", deployment.ID), nil, http.StatusOK)
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/service-deployments/%d/logs", deployment.ID), nil, http.StatusOK)

	// 开发者可登记制品并发起发布，但仍不能改项目成员或读取敏感变量。
	project = doJSON[portainer.PlatformProject](t, ctx, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{
		ResourceVersion: project.ResourceVersion,
		MemberPolicies:  map[portainer.UserID]portainer.PlatformProjectRole{2: portainer.PlatformProjectRoleDeveloper},
	}, http.StatusOK)
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{ResourceVersion: project.ResourceVersion}, http.StatusForbidden)
	artifactRecorder := doRawJSON(t, ctx, ctx.standardJWT, http.MethodPost, "/platform/artifacts/image-reference", createImageReferenceArtifactPayload{
		ProjectID:           project.ID,
		ApplicationID:       application.ID,
		ServiceDefinitionID: service.ID,
		Name:                "rbac-service",
		Version:             "1.0.0",
		ImageRef:            "registry.example.com/rbac:1.0.0",
	}, http.StatusCreated)
	var artifact portainer.PlatformArtifact
	require.NoError(t, json.Unmarshal(artifactRecorder.Body.Bytes(), &artifact))
	ctx.handler.ReleaseExecutor = fakeReleaseExecutor{}
	doRawJSONWithHeaders(t, ctx, ctx.standardJWT, http.MethodPost, "/platform/releases", createReleasePayloadFor(project, application, service, deployment, artifact), map[string]string{idempotencyKeyHeader: "developer-release"}, http.StatusAccepted)
}
