package platform

import (
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) applicationList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}

	if _, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(id), platformPermissionView); handlerErr != nil {
		return handlerErr
	}

	withArchived := includeArchived(r)
	applications, err := handler.DataStore.PlatformApplication().ReadAll(func(application portainer.PlatformApplication) bool {
		return application.ProjectID == portainer.PlatformProjectID(id) && (withArchived || isActive(application.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, applications)
}

func (handler *Handler) applicationInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "applicationId")
	if handlerErr != nil {
		return handlerErr
	}

	application, handlerErr := handler.requireApplicationPermission(r, portainer.PlatformApplicationID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}

	return response.JSON(w, application)
}

func (handler *Handler) applicationCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	var payload createApplicationPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	application := &portainer.PlatformApplication{
		ProjectID:         portainer.PlatformProjectID(projectID),
		Name:              payload.Name,
		Slug:              payload.Slug,
		Description:       payload.Description,
		OwnerUserIDs:      payload.OwnerUserIDs,
		PlatformLifecycle: newLifecycle(now),
	}

	// 应用是业务分组元数据，创建时只占用控制面 ID；
	// 后续部署配置和发布任务会分别在 ServiceDeployment 与 Release 批次处理。
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if _, err := readActiveProject(tx, application.ProjectID); err != nil {
			return err
		}

		existingApplications, err := tx.PlatformApplication().ReadAll(func(existing portainer.PlatformApplication) bool {
			return existing.ProjectID == application.ProjectID &&
				isActive(existing.PlatformLifecycle) &&
				existing.Slug == application.Slug
		})
		if err != nil {
			return err
		}
		if len(existingApplications) > 0 {
			return duplicateError("Application slug already exists in project")
		}

		return tx.PlatformApplication().Create(application)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, application, http.StatusCreated)
}

func (handler *Handler) applicationUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "applicationId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireApplicationPermission(r, portainer.PlatformApplicationID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	var payload updateApplicationPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	var application *portainer.PlatformApplication

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		application, err = readActiveApplication(tx, portainer.PlatformApplicationID(id))
		if err != nil {
			return err
		}
		if err := requireResourceVersion(application.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}

		if payload.Name != nil {
			application.Name = *payload.Name
		}
		if payload.Description != nil {
			application.Description = *payload.Description
		}
		if payload.OwnerUserIDs != nil {
			application.OwnerUserIDs = *payload.OwnerUserIDs
		}
		touchLifecycle(&application.PlatformLifecycle, now)

		return tx.PlatformApplication().Update(application.ID, application)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, application)
}

func (handler *Handler) applicationArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "applicationId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireApplicationPermission(r, portainer.PlatformApplicationID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		application, err := readActiveApplication(tx, portainer.PlatformApplicationID(id))
		if err != nil {
			return err
		}

		services, err := tx.PlatformServiceDefinition().ReadAll(func(service portainer.PlatformServiceDefinition) bool {
			return service.ApplicationID == application.ID && isActive(service.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(services) > 0 {
			return duplicateError("Application has active services")
		}

		archiveLifecycle(&application.PlatformLifecycle, now, userID)

		return tx.PlatformApplication().Update(application.ID, application)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}
