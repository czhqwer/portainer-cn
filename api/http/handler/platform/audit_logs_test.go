package platform

import (
	"fmt"
	"net/http"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestPlatformAuditLogsCoverConfigPermissionsDeniedAndFilters(t *testing.T) {
	ctx, project, _, _, _ := createPlatformReleaseFixture(t)

	configSet := createConfigSet(t, ctx, createConfigSetPayload{
		ProjectID: project.ID,
		ScopeType: portainer.PlatformConfigScopeProject,
		ScopeID:   int(project.ID),
		Entries: []portainer.PlatformConfigEntry{
			{Key: "PUBLIC_URL", Value: "https://example.test"},
			{Key: "AUDIT_SECRET", Sensitive: true},
		},
	})
	configSet = doJSON[portainer.PlatformConfigSet](t, ctx, http.MethodPut, fmt.Sprintf("/platform/config-sets/%d", configSet.ID), updateConfigSetPayload{
		ResourceVersion: configSet.ResourceVersion,
		Entries: &[]portainer.PlatformConfigEntry{
			{Key: "PUBLIC_URL", Value: "https://example.test"},
			{Key: "AUDIT_SECRET", Sensitive: true, Required: true},
			{Key: "UPDATED_SECRET", Sensitive: true},
		},
	}, http.StatusOK)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/config-sets/%d", configSet.ID), nil, http.StatusNoContent)

	project = doJSON[portainer.PlatformProject](t, ctx, http.MethodPut, fmt.Sprintf("/platform/projects/%d", project.ID), updateProjectPayload{
		ResourceVersion: project.ResourceVersion,
		MemberPolicies:  map[portainer.UserID]portainer.PlatformProjectRole{2: portainer.PlatformProjectRoleViewer},
	}, http.StatusOK)
	// 一个没有对象可见性的读取也必须保留 denied 事实，且响应继续保持 404。
	otherProject := createProject(t, ctx, createProjectPayload{Name: "Denied Audit", Slug: "denied-audit"})
	doRawJSON(t, ctx, ctx.standardJWT, http.MethodGet, fmt.Sprintf("/platform/projects/%d", otherProject.ID), nil, http.StatusNotFound)

	configCreated := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?projectId=%d&action=%s&result=success&operatorUserId=1&from=1&to=4102444800", project.ID, portainer.PlatformAuditActionConfigSetCreated), nil, http.StatusOK)
	require.Len(t, configCreated, 1)
	require.Equal(t, portainer.PlatformAuditActionConfigSetCreated, configCreated[0].Action)

	allProjectLogs := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?projectId=%d", project.ID), nil, http.StatusOK)
	actions := make(map[portainer.PlatformAuditAction]bool)
	for _, log := range allProjectLogs {
		actions[log.Action] = true
		require.NotContains(t, fmt.Sprint(log.BeforeSummary, log.AfterSummary), "AUDIT_SECRET=")
	}
	require.True(t, actions[portainer.PlatformAuditActionConfigSetCreated])
	require.True(t, actions[portainer.PlatformAuditActionConfigSetUpdated])
	require.True(t, actions[portainer.PlatformAuditActionConfigSetArchived])
	require.True(t, actions[portainer.PlatformAuditActionSecretCreated])
	require.True(t, actions[portainer.PlatformAuditActionSecretUpdated])
	require.True(t, actions[portainer.PlatformAuditActionSecretDeleted])
	require.True(t, actions[portainer.PlatformAuditActionProjectPermissionsUpdated])

	deniedLogs := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet, fmt.Sprintf("/platform/audit-logs?projectId=%d&action=%s&result=denied", otherProject.ID, portainer.PlatformAuditActionAccessDenied), nil, http.StatusOK)
	require.Len(t, deniedLogs, 1)
	require.Equal(t, portainer.PlatformAuditResultDenied, deniedLogs[0].Result)
	require.Equal(t, "view", deniedLogs[0].AfterSummary["permission"])
}

func TestPlatformAuditLogListFiltersAllStructuredFields(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Audit filters", Slug: "audit-filters"})
	require.NoError(t, ctx.handler.DataStore.PlatformAuditLog().Create(&portainer.PlatformAuditLog{
		Timestamp:           100,
		OperatorUserID:      1,
		ProjectID:           project.ID,
		EnvironmentID:       11,
		ApplicationID:       12,
		ServiceDefinitionID: 13,
		ServiceDeploymentID: 14,
		ArtifactID:          15,
		ReleaseID:           16,
		Action:              portainer.PlatformAuditActionConfigSetUpdated,
		Result:              portainer.PlatformAuditResultSuccess,
	}))
	require.NoError(t, ctx.handler.DataStore.PlatformAuditLog().Create(&portainer.PlatformAuditLog{
		Timestamp:      200,
		OperatorUserID: 2,
		ProjectID:      project.ID,
		Action:         portainer.PlatformAuditActionAccessDenied,
		Result:         portainer.PlatformAuditResultDenied,
	}))

	logs := doJSON[[]portainer.PlatformAuditLog](t, ctx, http.MethodGet,
		fmt.Sprintf("/platform/audit-logs?projectId=%d&environmentId=11&applicationId=12&serviceDefinitionId=13&serviceDeploymentId=14&artifactId=15&releaseId=16&operatorUserId=1&action=%s&result=success&from=100&to=100", project.ID, portainer.PlatformAuditActionConfigSetUpdated), nil, http.StatusOK)
	require.Len(t, logs, 1)
	require.Equal(t, portainer.PlatformAuditActionConfigSetUpdated, logs[0].Action)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodGet, fmt.Sprintf("/platform/audit-logs?projectId=%d&from=200&to=100", project.ID), nil, http.StatusBadRequest)
}
