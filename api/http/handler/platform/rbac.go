package platform

import (
	"errors"
	"net/http"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
)

// platformPermission 把阶段 2 的项目角色能力收敛为少量服务端授权点，避免前端隐藏
// 或某个 handler 遗漏检查而绕过权限边界。
type platformPermission int

const (
	platformPermissionView platformPermission = iota
	platformPermissionManage
	platformPermissionDeployment
	platformPermissionArtifactRelease
	platformPermissionSensitiveConfig
)

// platformProjectPermissions 是面向页面的只读权限摘要。团队和用户策略仍只在服务端
// 解析，前端用它改善交互而不能替代任何 API 的授权判断。
type platformProjectPermissions struct {
	Role               portainer.PlatformProjectRole `json:"Role,omitempty"`
	CanManageProject   bool                          `json:"CanManageProject"`
	CanManageResources bool                          `json:"CanManageResources"`
	CanDeploy          bool                          `json:"CanDeploy"`
	CanRevealSensitive bool                          `json:"CanRevealSensitive"`
}

type platformProjectResponse struct {
	portainer.PlatformProject
	Permissions platformProjectPermissions `json:"Permissions"`
}

func (handler *Handler) projectResponse(r *http.Request, project portainer.PlatformProject) platformProjectResponse {
	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return platformProjectResponse{PlatformProject: project}
	}
	role := projectRoleForContext(&project, context)

	return platformProjectResponse{
		PlatformProject: project,
		Permissions: platformProjectPermissions{
			Role:               role,
			CanManageProject:   projectRoleAllows(role, platformPermissionManage),
			CanManageResources: projectRoleAllows(role, platformPermissionManage),
			CanDeploy:          projectRoleAllows(role, platformPermissionArtifactRelease),
			CanRevealSensitive: projectRoleAllows(role, platformPermissionSensitiveConfig),
		},
	}
}

func (handler *Handler) projectResponses(r *http.Request, projects []portainer.PlatformProject) []platformProjectResponse {
	responses := make([]platformProjectResponse, len(projects))
	for i := range projects {
		responses[i] = handler.projectResponse(r, projects[i])
	}

	return responses
}

func (handler *Handler) requireGlobalAdmin(r *http.Request) *httperror.HandlerError {
	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return handler.convertError(err)
	}
	if !context.IsAdmin {
		handler.recordPlatformDeniedAudit(r, 0, 0, "global-admin", "global administrator permission is required")
		return platformAccessDenied()
	}

	return nil
}

// requireProjectPermission 返回项目本身，调用方可避免在授权成功后重复读取。
// 读取没有项目可见性时统一伪装为 404，写操作则明确返回 403，避免泄露对象存在性。
func (handler *Handler) requireProjectPermission(r *http.Request, projectID portainer.PlatformProjectID, permission platformPermission) (*portainer.PlatformProject, *httperror.HandlerError) {
	project, err := handler.DataStore.PlatformProject().Read(projectID)
	if err != nil {
		return nil, handler.convertError(err)
	}

	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return nil, handler.convertError(err)
	}
	role := projectRoleForContext(project, context)
	if projectRoleAllows(role, permission) {
		return project, nil
	}
	handler.recordPlatformDeniedAudit(r, project.ID, 0, platformPermissionName(permission), "project role does not grant this operation")
	if permission == platformPermissionView {
		return nil, notFoundError("Platform project is unavailable")
	}

	return nil, platformAccessDenied()
}

// visibleProjectIDs 为无 projectId 过滤条件的集合接口建立可见项目集合。
// 管理员保留全量视图，普通用户只能看到至少拥有 viewer 角色的项目。
func (handler *Handler) visibleProjectIDs(r *http.Request) (map[portainer.PlatformProjectID]bool, *httperror.HandlerError) {
	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if context.IsAdmin {
		return nil, nil
	}

	projects, err := handler.DataStore.PlatformProject().ReadAll(nil)
	if err != nil {
		return nil, handler.convertError(err)
	}
	visible := make(map[portainer.PlatformProjectID]bool)
	for i := range projects {
		if projectRoleAllows(projectRoleForContext(&projects[i], context), platformPermissionView) {
			visible[projects[i].ID] = true
		}
	}

	return visible, nil
}

func (handler *Handler) requireEnvironmentPermission(r *http.Request, environmentID portainer.PlatformEnvironmentID, permission platformPermission) (*portainer.PlatformEnvironment, *httperror.HandlerError) {
	environment, err := handler.DataStore.PlatformEnvironment().Read(environmentID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, environment.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return environment, nil
}

func (handler *Handler) requireApplicationPermission(r *http.Request, applicationID portainer.PlatformApplicationID, permission platformPermission) (*portainer.PlatformApplication, *httperror.HandlerError) {
	application, err := handler.DataStore.PlatformApplication().Read(applicationID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, application.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return application, nil
}

func (handler *Handler) requireServiceDefinitionPermission(r *http.Request, serviceID portainer.PlatformServiceDefinitionID, permission platformPermission) (*portainer.PlatformServiceDefinition, *httperror.HandlerError) {
	service, err := handler.DataStore.PlatformServiceDefinition().Read(serviceID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, service.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return service, nil
}

func (handler *Handler) requireServiceDeploymentPermission(r *http.Request, deploymentID portainer.PlatformServiceDeploymentID, permission platformPermission) (*portainer.PlatformServiceDeployment, *httperror.HandlerError) {
	deployment, err := handler.DataStore.PlatformServiceDeployment().Read(deploymentID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, deployment.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return deployment, nil
}

func (handler *Handler) requireConfigSetPermission(r *http.Request, configSetID portainer.PlatformConfigSetID, permission platformPermission) (*portainer.PlatformConfigSet, *httperror.HandlerError) {
	configSet, err := handler.DataStore.PlatformConfigSet().Read(configSetID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, configSet.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return configSet, nil
}

func (handler *Handler) requireArtifactPermission(r *http.Request, artifactID portainer.PlatformArtifactID, permission platformPermission) (*portainer.PlatformArtifact, *httperror.HandlerError) {
	artifact, err := handler.DataStore.PlatformArtifact().Read(artifactID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, artifact.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return artifact, nil
}

func (handler *Handler) requireReleasePermission(r *http.Request, releaseID portainer.PlatformReleaseID, permission platformPermission) (*portainer.PlatformRelease, *httperror.HandlerError) {
	release, err := handler.DataStore.PlatformRelease().Read(releaseID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, release.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}

	return release, nil
}

// requireEnvironmentEndpointAccess 对会触发运行时访问的动作额外取 Endpoint 权限交集。
// 仅有项目角色不能读取日志、状态或执行发布，防止项目侧授权扩大既有 Portainer 环境权限。
func (handler *Handler) requireEnvironmentEndpointAccess(r *http.Request, environment *portainer.PlatformEnvironment) *httperror.HandlerError {
	if environment == nil {
		return platformAccessDenied()
	}

	handlerErr := handler.requireEndpointAccessForTargets(r, environment.Targets)
	if handlerErr != nil {
		handler.recordPlatformDeniedAudit(r, environment.ProjectID, environment.ID, "endpoint-runtime", "Portainer Endpoint permission is required")
	}

	return handlerErr
}

// requireReleaseRequestPermission 在创建或预检发布前同时确认项目角色、环境归属和
// Endpoint 权限；实际关联完整性仍由事务中的发布引用校验负责。
func (handler *Handler) requireReleaseRequestPermission(r *http.Request, payload createReleasePayload) *httperror.HandlerError {
	if _, handlerErr := handler.requireProjectPermission(r, payload.ProjectID, platformPermissionArtifactRelease); handlerErr != nil {
		return handlerErr
	}
	environment, err := handler.DataStore.PlatformEnvironment().Read(payload.EnvironmentID)
	if err != nil {
		return platformAccessDenied()
	}
	if environment.ProjectID != payload.ProjectID {
		return platformAccessDenied()
	}

	return handler.requireEnvironmentEndpointAccess(r, environment)
}

func (handler *Handler) requireReleaseRuntimePermission(r *http.Request, release *portainer.PlatformRelease) *httperror.HandlerError {
	if release == nil {
		return platformAccessDenied()
	}
	environment, err := handler.DataStore.PlatformEnvironment().Read(release.EnvironmentID)
	if err != nil {
		return platformAccessDenied()
	}

	return handler.requireEnvironmentEndpointAccess(r, environment)
}

func (handler *Handler) requireEndpointAccessForTargets(r *http.Request, targets []portainer.PlatformDeploymentTarget) *httperror.HandlerError {
	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return handler.convertError(err)
	}
	if context.IsAdmin {
		return nil
	}

	hasWorkloadTarget := false
	for _, target := range targets {
		if !target.Enabled || (target.Role != "" && target.Role != portainer.PlatformDeploymentTargetRoleWorkload) {
			continue
		}
		hasWorkloadTarget = true

		endpoint, err := handler.DataStore.Endpoint().Endpoint(target.EndpointID)
		if err != nil {
			// Endpoint 的存在性不应通过平台 API 暴露给无权用户。
			return platformAccessDenied()
		}
		endpointGroup, err := handler.DataStore.EndpointGroup().Read(endpoint.GroupID)
		if err != nil || !security.AuthorizedEndpointAccess(endpoint, endpointGroup, context.UserID, context.UserMemberships) {
			return platformAccessDenied()
		}
	}

	if !hasWorkloadTarget {
		return platformAccessDenied()
	}

	return nil
}

// requireEndpointAccess 保护不依附既有 Deployment 的 Docker 动作，确保归档导入也不能只凭项目角色访问任意 Endpoint。
func (handler *Handler) requireEndpointAccess(r *http.Request, endpointID portainer.EndpointID) *httperror.HandlerError {
	context, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return handler.convertError(err)
	}
	if context.IsAdmin {
		return nil
	}
	endpoint, err := handler.DataStore.Endpoint().Endpoint(endpointID)
	if err != nil {
		return platformAccessDenied()
	}
	group, err := handler.DataStore.EndpointGroup().Read(endpoint.GroupID)
	if err != nil || !security.AuthorizedEndpointAccess(endpoint, group, context.UserID, context.UserMemberships) {
		return platformAccessDenied()
	}
	return nil
}

func projectRoleForContext(project *portainer.PlatformProject, context *security.RestrictedRequestContext) portainer.PlatformProjectRole {
	if project == nil || context == nil {
		return ""
	}
	if context.IsAdmin {
		return portainer.PlatformProjectRoleAdmin
	}

	// 用户级策略优先于团队策略，使显式降权也能覆盖团队默认角色。
	if role, found := project.MemberPolicies[context.UserID]; found {
		if validProjectRole(role) {
			return role
		}
		return ""
	}

	role := portainer.PlatformProjectRole("")
	for _, membership := range context.UserMemberships {
		candidate, found := project.TeamPolicies[membership.TeamID]
		if found && projectRoleRank(candidate) > projectRoleRank(role) {
			role = candidate
		}
	}

	return role
}

func projectRoleAllows(role portainer.PlatformProjectRole, permission platformPermission) bool {
	switch permission {
	case platformPermissionView:
		return projectRoleRank(role) >= projectRoleRank(portainer.PlatformProjectRoleViewer)
	case platformPermissionManage, platformPermissionSensitiveConfig:
		return role == portainer.PlatformProjectRoleAdmin
	case platformPermissionDeployment, platformPermissionArtifactRelease:
		return role == portainer.PlatformProjectRoleAdmin || role == portainer.PlatformProjectRoleDeveloper
	default:
		return false
	}
}

func platformPermissionName(permission platformPermission) string {
	switch permission {
	case platformPermissionView:
		return "view"
	case platformPermissionManage:
		return "manage"
	case platformPermissionDeployment:
		return "deployment"
	case platformPermissionArtifactRelease:
		return "artifact-release"
	case platformPermissionSensitiveConfig:
		return "sensitive-config"
	default:
		return "unknown"
	}
}

func projectRoleRank(role portainer.PlatformProjectRole) int {
	switch role {
	case portainer.PlatformProjectRoleAdmin:
		return 3
	case portainer.PlatformProjectRoleDeveloper:
		return 2
	case portainer.PlatformProjectRoleViewer:
		return 1
	default:
		return 0
	}
}

func validProjectRole(role portainer.PlatformProjectRole) bool {
	return projectRoleRank(role) > 0
}

// validateProjectPolicies 在写入前拒绝未知角色，避免存入无法解释的策略后产生
// “看似已授权但实际没有能力”的权限漂移。
func validateProjectPolicies(memberPolicies map[portainer.UserID]portainer.PlatformProjectRole, teamPolicies map[portainer.TeamID]portainer.PlatformProjectRole) error {
	for _, role := range memberPolicies {
		if !validProjectRole(role) {
			return errors.New("MemberPolicies contains an invalid project role")
		}
	}
	for _, role := range teamPolicies {
		if !validProjectRole(role) {
			return errors.New("TeamPolicies contains an invalid project role")
		}
	}

	return nil
}

func platformAccessDenied() *httperror.HandlerError {
	return httperror.Forbidden(errPlatformAccessDenied, errors.New("Platform project permission is required"))
}
