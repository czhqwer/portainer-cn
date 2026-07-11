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

func (handler *Handler) serviceDefinitionList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "applicationId")
	if handlerErr != nil {
		return handlerErr
	}

	if _, err := handler.DataStore.PlatformApplication().Read(portainer.PlatformApplicationID(id)); err != nil {
		return handler.convertError(err)
	}

	withArchived := includeArchived(r)
	services, err := handler.DataStore.PlatformServiceDefinition().ReadAll(func(service portainer.PlatformServiceDefinition) bool {
		return service.ApplicationID == portainer.PlatformApplicationID(id) && (withArchived || isActive(service.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, services)
}

func (handler *Handler) serviceDefinitionInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "serviceDefinitionId")
	if handlerErr != nil {
		return handlerErr
	}

	service, err := handler.DataStore.PlatformServiceDefinition().Read(portainer.PlatformServiceDefinitionID(id))
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, service)
}

func (handler *Handler) serviceDefinitionCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	applicationID, handlerErr := handler.routeID(r, "applicationId")
	if handlerErr != nil {
		return handlerErr
	}

	var payload createServiceDefinitionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	service := &portainer.PlatformServiceDefinition{
		ApplicationID:     portainer.PlatformApplicationID(applicationID),
		Name:              payload.Name,
		Slug:              payload.Slug,
		Type:              payload.Type,
		Description:       payload.Description,
		PlatformLifecycle: newLifecycle(now),
	}
	normalizeServiceDefinition(service)

	// 逻辑服务仅描述可部署单元，不携带环境差异；
	// 环境差异全部下沉到 ServiceDeployment，保证后续发布向导能按环境独立维护。
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		application, err := readActiveApplication(tx, service.ApplicationID)
		if err != nil {
			return err
		}
		service.ProjectID = application.ProjectID

		existingServices, err := tx.PlatformServiceDefinition().ReadAll(func(existing portainer.PlatformServiceDefinition) bool {
			return existing.ApplicationID == service.ApplicationID &&
				isActive(existing.PlatformLifecycle) &&
				existing.Slug == service.Slug
		})
		if err != nil {
			return err
		}
		if len(existingServices) > 0 {
			return duplicateError("Service slug already exists in application")
		}

		return tx.PlatformServiceDefinition().Create(service)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, service, http.StatusCreated)
}

func (handler *Handler) serviceDefinitionUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "serviceDefinitionId")
	if handlerErr != nil {
		return handlerErr
	}

	var payload updateServiceDefinitionPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	var service *portainer.PlatformServiceDefinition

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		service, err = readActiveServiceDefinition(tx, portainer.PlatformServiceDefinitionID(id))
		if err != nil {
			return err
		}
		if err := requireResourceVersion(service.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}

		if payload.Name != nil {
			service.Name = *payload.Name
		}
		if payload.Type != nil {
			service.Type = *payload.Type
		}
		if payload.Description != nil {
			service.Description = *payload.Description
		}
		normalizeServiceDefinition(service)
		touchLifecycle(&service.PlatformLifecycle, now)

		return tx.PlatformServiceDefinition().Update(service.ID, service)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, service)
}

func (handler *Handler) serviceDefinitionArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "serviceDefinitionId")
	if handlerErr != nil {
		return handlerErr
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		service, err := readActiveServiceDefinition(tx, portainer.PlatformServiceDefinitionID(id))
		if err != nil {
			return err
		}

		deployments, err := tx.PlatformServiceDeployment().ReadAll(func(deployment portainer.PlatformServiceDeployment) bool {
			return deployment.ServiceDefinitionID == service.ID && isActive(deployment.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(deployments) > 0 {
			return duplicateError("Service has active deployments")
		}

		archiveLifecycle(&service.PlatformLifecycle, now, userID)

		return tx.PlatformServiceDefinition().Update(service.ID, service)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}
