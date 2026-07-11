package platform

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/http/security"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
)

// Handler manages the private deployment platform control-plane endpoints.
type Handler struct {
	*mux.Router
	DataStore               dataservices.DataStore
	ReleaseExecutor         platformservice.ReleaseExecutor
	ReleaseRecoveryExecutor platformservice.ReleaseRecoveryExecutor
	RuntimeInspector        platformservice.RuntimeInspector
}

// NewHandler 注册阶段 2 平台接口。路由层只完成认证和受限上下文注入，
// 资源级项目角色与 Endpoint 权限交集由各 handler 按实际对象强制校验。
func NewHandler(bouncer security.BouncerService) *Handler {
	h := &Handler{
		Router: mux.NewRouter(),
	}

	h.Handle("/platform/projects",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.projectList))).Methods(http.MethodGet)
	h.Handle("/platform/projects",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.projectCreate))).Methods(http.MethodPost)
	h.Handle("/platform/projects/{projectId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.projectInspect))).Methods(http.MethodGet)
	h.Handle("/platform/projects/{projectId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.projectUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/projects/{projectId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.projectArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/projects/{projectId}/environments",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.environmentList))).Methods(http.MethodGet)
	h.Handle("/platform/projects/{projectId}/environments",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.environmentCreate))).Methods(http.MethodPost)
	h.Handle("/platform/environments/{environmentId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.environmentInspect))).Methods(http.MethodGet)
	h.Handle("/platform/environments/{environmentId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.environmentUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/environments/{environmentId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.environmentArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/projects/{projectId}/applications",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.applicationList))).Methods(http.MethodGet)
	h.Handle("/platform/projects/{projectId}/applications",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.applicationCreate))).Methods(http.MethodPost)
	h.Handle("/platform/applications/{applicationId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.applicationInspect))).Methods(http.MethodGet)
	h.Handle("/platform/applications/{applicationId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.applicationUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/applications/{applicationId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.applicationArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/applications/{applicationId}/services",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDefinitionList))).Methods(http.MethodGet)
	h.Handle("/platform/applications/{applicationId}/services",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDefinitionCreate))).Methods(http.MethodPost)
	h.Handle("/platform/services/{serviceDefinitionId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDefinitionInspect))).Methods(http.MethodGet)
	h.Handle("/platform/services/{serviceDefinitionId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDefinitionUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/services/{serviceDefinitionId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDefinitionArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/services/{serviceDefinitionId}/deployments",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentList))).Methods(http.MethodGet)
	h.Handle("/platform/services/{serviceDefinitionId}/deployments",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentCreate))).Methods(http.MethodPost)
	h.Handle("/platform/service-deployments/{deploymentId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentInspect))).Methods(http.MethodGet)
	h.Handle("/platform/service-deployments/{deploymentId}/status",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentStatus))).Methods(http.MethodGet)
	h.Handle("/platform/service-deployments/{deploymentId}/logs",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentLogs))).Methods(http.MethodGet)
	h.Handle("/platform/service-deployments/{deploymentId}/effective-config",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentEffectiveConfig))).Methods(http.MethodGet)
	h.Handle("/platform/service-deployments/{deploymentId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/service-deployments/{deploymentId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.serviceDeploymentArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/artifacts",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.artifactList))).Methods(http.MethodGet)

	h.Handle("/platform/config-sets",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSetList))).Methods(http.MethodGet)
	h.Handle("/platform/config-sets",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSetCreate))).Methods(http.MethodPost)
	h.Handle("/platform/config-sets/{configSetId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSetInspect))).Methods(http.MethodGet)
	h.Handle("/platform/config-sets/{configSetId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSetUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/config-sets/{configSetId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSetArchive))).Methods(http.MethodDelete)
	h.Handle("/platform/config-sets/{configSetId}/entries/{key}/reveal",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSecretReveal))).Methods(http.MethodPost)
	h.Handle("/platform/config-sets/{configSetId}/entries/{key}/copy",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.configSecretCopy))).Methods(http.MethodPost)
	h.Handle("/platform/artifacts/image-reference",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.artifactImageReferenceCreate))).Methods(http.MethodPost)
	h.Handle("/platform/artifacts/{artifactId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.artifactInspect))).Methods(http.MethodGet)
	h.Handle("/platform/artifacts/{artifactId}/validate",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.artifactValidate))).Methods(http.MethodPost)
	h.Handle("/platform/artifacts/{artifactId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.artifactArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/releases",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseList))).Methods(http.MethodGet)
	h.Handle("/platform/releases/validate",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseValidate))).Methods(http.MethodPost)
	h.Handle("/platform/releases",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseCreate))).Methods(http.MethodPost)
	h.Handle("/platform/releases/{releaseId}",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseInspect))).Methods(http.MethodGet)
	h.Handle("/platform/releases/{releaseId}/rollback-diff",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseRollbackDiff))).Methods(http.MethodGet)
	h.Handle("/platform/releases/{releaseId}/rollback",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseRollback))).Methods(http.MethodPost)
	h.Handle("/platform/releases/{releaseId}/resolve",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseResolve))).Methods(http.MethodPost)
	h.Handle("/platform/releases/{releaseId}/retry-recovery",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseRetryRecovery))).Methods(http.MethodPost)
	h.Handle("/platform/releases/{releaseId}/cleanup-runtime",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.releaseCleanupRuntime))).Methods(http.MethodPost)

	h.Handle("/platform/audit-logs",
		bouncer.RestrictedAccess(httperror.LoggerHandler(h.auditLogList))).Methods(http.MethodGet)

	return h
}
