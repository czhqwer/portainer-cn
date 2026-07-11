package platform

import (
	"errors"
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) serviceDeploymentList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "serviceDefinitionId")
	if handlerErr != nil {
		return handlerErr
	}

	if _, handlerErr := handler.requireServiceDefinitionPermission(r, portainer.PlatformServiceDefinitionID(id), platformPermissionView); handlerErr != nil {
		return handlerErr
	}

	withArchived := includeArchived(r)
	deployments, err := handler.DataStore.PlatformServiceDeployment().ReadAll(func(deployment portainer.PlatformServiceDeployment) bool {
		return deployment.ServiceDefinitionID == portainer.PlatformServiceDefinitionID(id) && (withArchived || isActive(deployment.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, deployments)
}

func (handler *Handler) serviceDeploymentInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return handlerErr
	}

	deployment, handlerErr := handler.requireServiceDeploymentPermission(r, portainer.PlatformServiceDeploymentID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}

	return response.JSON(w, deployment)
}

func (handler *Handler) serviceDeploymentCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	serviceID, handlerErr := handler.routeID(r, "serviceDefinitionId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireServiceDefinitionPermission(r, portainer.PlatformServiceDefinitionID(serviceID), platformPermissionDeployment); handlerErr != nil {
		return handlerErr
	}

	var payload createServiceDeploymentPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	environment, err := handler.DataStore.PlatformEnvironment().Read(payload.EnvironmentID)
	if err != nil {
		return handler.convertError(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, environment.ProjectID, platformPermissionDeployment); handlerErr != nil {
		return handlerErr
	}

	desiredSpec := portainer.NewPlatformDeploymentDesiredSpec()
	if payload.DesiredSpec != nil {
		desiredSpec = *payload.DesiredSpec
	}
	portainer.NormalizePlatformDeploymentDesiredSpec(&desiredSpec)
	if err := portainer.ValidatePlatformDeploymentDesiredSpecV01(desiredSpec); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	deployment := &portainer.PlatformServiceDeployment{
		ServiceDefinitionID: serviceDefinitionID(serviceID),
		EnvironmentID:       payload.EnvironmentID,
		DesiredSpec:         desiredSpec,
		SpecRevision:        1,
		DriftStatus:         portainer.PlatformDeploymentDriftNone,
		PlatformLifecycle:   newLifecycle(now),
	}

	// ServiceDeployment 是阶段 1 前半段最接近运行态的控制面资源；
	// 此处只保存期望配置和版本号，真实容器验证/切换必须等待 Gate 0B。
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		service, err := readActiveServiceDefinition(tx, deployment.ServiceDefinitionID)
		if err != nil {
			return err
		}
		environment, err := readActiveEnvironment(tx, deployment.EnvironmentID)
		if err != nil {
			return err
		}
		if environment.ProjectID != service.ProjectID {
			return validationFailedError("Environment and service belong to different projects")
		}

		existingDeployments, err := tx.PlatformServiceDeployment().ReadAll(func(existing portainer.PlatformServiceDeployment) bool {
			return existing.ServiceDefinitionID == service.ID &&
				existing.EnvironmentID == environment.ID &&
				isActive(existing.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(existingDeployments) > 0 {
			return duplicateError("Service deployment already exists for environment")
		}

		deployment.ProjectID = service.ProjectID
		deployment.ApplicationID = service.ApplicationID

		return tx.PlatformServiceDeployment().Create(deployment)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, deployment, http.StatusCreated)
}

func (handler *Handler) serviceDeploymentUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireServiceDeploymentPermission(r, portainer.PlatformServiceDeploymentID(id), platformPermissionDeployment); handlerErr != nil {
		return handlerErr
	}

	var payload updateServiceDeploymentPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	var deployment *portainer.PlatformServiceDeployment

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		deployment, err = readActiveServiceDeployment(tx, portainer.PlatformServiceDeploymentID(id))
		if err != nil {
			return err
		}
		if err := requireResourceVersion(deployment.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}

		deployment.DesiredSpec = *payload.DesiredSpec
		deployment.SpecRevision++
		if deployment.LastDeployedSpecRevision > 0 && deployment.LastDeployedSpecRevision != deployment.SpecRevision {
			deployment.DriftStatus = portainer.PlatformDeploymentDriftConfigChanged
		}
		touchLifecycle(&deployment.PlatformLifecycle, now)

		return tx.PlatformServiceDeployment().Update(deployment.ID, deployment)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, deployment)
}

func (handler *Handler) serviceDeploymentArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireServiceDeploymentPermission(r, portainer.PlatformServiceDeploymentID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		deployment, err := readActiveServiceDeployment(tx, portainer.PlatformServiceDeploymentID(id))
		if err != nil {
			return err
		}

		archiveLifecycle(&deployment.PlatformLifecycle, now, userID)

		return tx.PlatformServiceDeployment().Update(deployment.ID, deployment)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}

func serviceDefinitionID(id int) portainer.PlatformServiceDefinitionID {
	return portainer.PlatformServiceDefinitionID(id)
}

func validationFailedError(message string) *httperror.HandlerError {
	return httperror.BadRequest(errPlatformValidationFailed, errors.New(message))
}
