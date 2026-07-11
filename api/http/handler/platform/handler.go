package platform

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
)

// Handler manages the private deployment platform control-plane endpoints.
type Handler struct {
	*mux.Router
	DataStore dataservices.DataStore
}

// NewHandler registers only Gate 0A-safe CRUD endpoints.
func NewHandler(bouncer security.BouncerService) *Handler {
	h := &Handler{
		Router: mux.NewRouter(),
	}

	// V0.1 明确要求 /api/platform 全部业务接口仅管理员可访问；
	// 项目级 RBAC 字段先随模型预留，但不能在技术预览阶段放开读取接口。
	h.Handle("/platform/projects",
		bouncer.AdminAccess(httperror.LoggerHandler(h.projectList))).Methods(http.MethodGet)
	h.Handle("/platform/projects",
		bouncer.AdminAccess(httperror.LoggerHandler(h.projectCreate))).Methods(http.MethodPost)
	h.Handle("/platform/projects/{projectId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.projectInspect))).Methods(http.MethodGet)
	h.Handle("/platform/projects/{projectId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.projectUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/projects/{projectId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.projectArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/projects/{projectId}/environments",
		bouncer.AdminAccess(httperror.LoggerHandler(h.environmentList))).Methods(http.MethodGet)
	h.Handle("/platform/projects/{projectId}/environments",
		bouncer.AdminAccess(httperror.LoggerHandler(h.environmentCreate))).Methods(http.MethodPost)
	h.Handle("/platform/environments/{environmentId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.environmentInspect))).Methods(http.MethodGet)
	h.Handle("/platform/environments/{environmentId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.environmentUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/environments/{environmentId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.environmentArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/projects/{projectId}/applications",
		bouncer.AdminAccess(httperror.LoggerHandler(h.applicationList))).Methods(http.MethodGet)
	h.Handle("/platform/projects/{projectId}/applications",
		bouncer.AdminAccess(httperror.LoggerHandler(h.applicationCreate))).Methods(http.MethodPost)
	h.Handle("/platform/applications/{applicationId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.applicationInspect))).Methods(http.MethodGet)
	h.Handle("/platform/applications/{applicationId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.applicationUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/applications/{applicationId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.applicationArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/applications/{applicationId}/services",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDefinitionList))).Methods(http.MethodGet)
	h.Handle("/platform/applications/{applicationId}/services",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDefinitionCreate))).Methods(http.MethodPost)
	h.Handle("/platform/services/{serviceDefinitionId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDefinitionInspect))).Methods(http.MethodGet)
	h.Handle("/platform/services/{serviceDefinitionId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDefinitionUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/services/{serviceDefinitionId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDefinitionArchive))).Methods(http.MethodDelete)

	h.Handle("/platform/services/{serviceDefinitionId}/deployments",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDeploymentList))).Methods(http.MethodGet)
	h.Handle("/platform/services/{serviceDefinitionId}/deployments",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDeploymentCreate))).Methods(http.MethodPost)
	h.Handle("/platform/service-deployments/{deploymentId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDeploymentInspect))).Methods(http.MethodGet)
	h.Handle("/platform/service-deployments/{deploymentId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDeploymentUpdate))).Methods(http.MethodPut)
	h.Handle("/platform/service-deployments/{deploymentId}",
		bouncer.AdminAccess(httperror.LoggerHandler(h.serviceDeploymentArchive))).Methods(http.MethodDelete)

	return h
}
