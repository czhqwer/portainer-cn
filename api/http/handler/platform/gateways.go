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

type createGatewayRoutePayload struct {
	ServiceDeploymentID portainer.PlatformServiceDeploymentID  `json:"ServiceDeploymentId"`
	Domain              string                                 `json:"Domain"`
	Path                string                                 `json:"Path"`
	TargetPort          int                                    `json:"TargetPort"`
	EnableTLS           bool                                   `json:"EnableTls"`
	ForceHTTPS          bool                                   `json:"ForceHttps"`
	WebSocket           bool                                   `json:"WebSocket"`
	ProxyTimeoutSeconds int                                    `json:"ProxyTimeoutSeconds"`
	MaxRequestBodyBytes int64                                  `json:"MaxRequestBodyBytes"`
	CertificateID       portainer.PlatformGatewayCertificateID `json:"CertificateId"`
}

func (payload *createGatewayRoutePayload) Validate(r *http.Request) error {
	payload.Domain = strings.TrimSpace(payload.Domain)
	payload.Path = strings.TrimSpace(payload.Path)
	if payload.ServiceDeploymentID <= 0 || payload.Domain == "" || payload.TargetPort <= 0 {
		return errors.New("service deployment, domain and target port are required")
	}
	return nil
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

func (handler *Handler) gatewayRouteList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	gateway, handlerErr := handler.gatewayFromRequest(r, platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}
	routes, err := handler.DataStore.PlatformGatewayRoute().ReadAll(func(route portainer.PlatformGatewayRoute) bool {
		return route.GatewayID == gateway.ID && (includeArchived(r) || isActive(route.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, routes)
}

func (handler *Handler) gatewayRouteCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	gateway, handlerErr := handler.gatewayFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	var payload createGatewayRoutePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	deployment, handlerErr := handler.requireServiceDeploymentPermission(r, payload.ServiceDeploymentID, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if deployment.ProjectID != gateway.ProjectID || deployment.EnvironmentID != gateway.EnvironmentID {
		return platformAccessDenied()
	}
	if !declaresPort(*deployment, payload.TargetPort) {
		return validationFailedError("TargetPort must be declared by the service deployment")
	}
	route := portainer.NewPlatformGatewayRoute()
	route.GatewayID, route.ProjectID, route.EnvironmentID, route.ServiceDeploymentID = gateway.ID, gateway.ProjectID, gateway.EnvironmentID, deployment.ID
	route.Domain, route.Path, route.TargetPort = payload.Domain, payload.Path, payload.TargetPort
	route.EnableTLS, route.ForceHTTPS, route.WebSocket, route.ProxyTimeoutSeconds, route.MaxRequestBodyBytes, route.CertificateID = payload.EnableTLS, payload.ForceHTTPS, payload.WebSocket, payload.ProxyTimeoutSeconds, payload.MaxRequestBodyBytes, payload.CertificateID
	route.PlatformLifecycle = newLifecycle(time.Now().Unix())
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existing, err := tx.PlatformGatewayRoute().ReadAll(func(item portainer.PlatformGatewayRoute) bool {
			return item.GatewayID == gateway.ID && isActive(item.PlatformLifecycle) && strings.EqualFold(item.Domain, route.Domain) && item.Path == route.Path
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return duplicateError("gateway domain and path already exist")
		}
		if err := tx.PlatformGatewayRoute().Create(&route); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionGatewayRouteCreated, Result: portainer.PlatformAuditResultSuccess, ProjectID: gateway.ProjectID, EnvironmentID: gateway.EnvironmentID, ServiceDeploymentID: route.ServiceDeploymentID, AfterSummary: map[string]any{"gatewayRouteId": route.ID, "gatewayId": gateway.ID}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSONWithStatus(w, route, http.StatusCreated)
}

func (handler *Handler) gatewayFromRequest(r *http.Request, permission platformPermission) (*portainer.PlatformGateway, *httperror.HandlerError) {
	id, handlerErr := handler.routeID(r, "gatewayId")
	if handlerErr != nil {
		return nil, handlerErr
	}
	gateway, err := handler.DataStore.PlatformGateway().Read(portainer.PlatformGatewayID(id))
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr = handler.requireProjectPermission(r, gateway.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}
	return gateway, nil
}

func declaresPort(deployment portainer.PlatformServiceDeployment, port int) bool {
	for _, declared := range deployment.DesiredSpec.Ports {
		if declared.ContainerPort == port {
			return true
		}
	}
	return false
}
