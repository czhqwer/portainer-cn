package platform

import (
	"errors"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type createGatewayPayload struct {
	EnvironmentID portainer.PlatformEnvironmentID `json:"EnvironmentId"`
	EndpointID    portainer.EndpointID            `json:"EndpointId"`
	NodeName      string                          `json:"NodeName"`
	Name          string                          `json:"Name"`
}

func (payload *createGatewayPayload) Validate(r *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.NodeName = strings.TrimSpace(payload.NodeName)
	if payload.EnvironmentID <= 0 || payload.EndpointID <= 0 || payload.Name == "" {
		return errors.New("environment, endpoint and name are required")
	}
	return nil
}

func (handler *Handler) gatewayList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	items, err := handler.DataStore.PlatformGateway().ReadAll(func(item portainer.PlatformGateway) bool {
		return item.ProjectID == portainer.PlatformProjectID(projectID) && (includeArchived(r) || isActive(item.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, items)
}

// gatewayCreate 只登记已经授权的网关 Endpoint；容器创建、配置写入和 reload 必须由事务外的运行时批次完成。
func (handler *Handler) gatewayCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}
	var payload createGatewayPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	environment, handlerErr := handler.requireEnvironmentPermission(r, payload.EnvironmentID, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if environment.ProjectID != portainer.PlatformProjectID(projectID) {
		return platformAccessDenied()
	}
	if _, err := handler.DataStore.Endpoint().Endpoint(payload.EndpointID); err != nil {
		return handler.convertError(err)
	}
	if handlerErr = handler.requireEndpointAccess(r, payload.EndpointID); handlerErr != nil {
		return handlerErr
	}
	gateway := portainer.NewPlatformGateway()
	gateway.ProjectID, gateway.EnvironmentID, gateway.EndpointID = portainer.PlatformProjectID(projectID), payload.EnvironmentID, payload.EndpointID
	gateway.NodeName, gateway.Name, gateway.PlatformLifecycle = payload.NodeName, payload.Name, newLifecycle(time.Now().Unix())
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existing, err := tx.PlatformGateway().ReadAll(func(item portainer.PlatformGateway) bool {
			return item.EnvironmentID == gateway.EnvironmentID && isActive(item.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return duplicateError("environment already has an active gateway")
		}
		if err := tx.PlatformGateway().Create(&gateway); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionGatewayCreated, Result: portainer.PlatformAuditResultSuccess, ProjectID: gateway.ProjectID, EnvironmentID: gateway.EnvironmentID, AfterSummary: map[string]any{"gatewayId": gateway.ID, "endpointId": gateway.EndpointID}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSONWithStatus(w, gateway, http.StatusCreated)
}

func (handler *Handler) gatewayInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "gatewayId")
	if handlerErr != nil {
		return handlerErr
	}
	gateway, err := handler.DataStore.PlatformGateway().Read(portainer.PlatformGatewayID(id))
	if err != nil {
		return handler.convertError(err)
	}
	if _, handlerErr = handler.requireProjectPermission(r, gateway.ProjectID, platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	return response.JSON(w, gateway)
}
