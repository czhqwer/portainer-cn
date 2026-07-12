package platform

import (
	"context"
	"errors"
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type targetPreflightPayload struct {
	ServiceDeploymentID portainer.PlatformServiceDeploymentID `json:"ServiceDeploymentId"`
	TargetPort          int                                   `json:"TargetPort"`
}

type targetPreflightResult struct {
	EndpointID portainer.EndpointID `json:"EndpointId"`
	Status     string               `json:"Status"`
	Reason     string               `json:"Reason,omitempty"`
}

func (payload *targetPreflightPayload) Validate(*http.Request) error {
	if payload.ServiceDeploymentID <= 0 || payload.TargetPort < 1 || payload.TargetPort > 65535 {
		return errors.New("service deployment and target port are required")
	}
	return nil
}

// environmentTargetPreflight 让 gateway 容器实际探测各 workload 的发布端口。
// 返回结果只暴露 target 的 Endpoint ID 与稳定 reason，原始网络错误不离开服务端。
func (handler *Handler) environmentTargetPreflight(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "environmentId")
	if handlerErr != nil {
		return handlerErr
	}
	environment, handlerErr := handler.requireEnvironmentPermission(r, portainer.PlatformEnvironmentID(id), platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handler.GatewayRuntime == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Gateway runtime is unavailable", "GATEWAY_RUNTIME_UNAVAILABLE", nil)
	}
	var payload targetPreflightPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	deployment, handlerErr := handler.requireServiceDeploymentPermission(r, payload.ServiceDeploymentID, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if deployment.EnvironmentID != environment.ID || deployment.ProjectID != environment.ProjectID {
		return platformAccessDenied()
	}
	if handlerErr = handler.requireEndpointAccessForTargets(r, environment.Targets); handlerErr != nil {
		return handlerErr
	}
	gateway, err := handler.activeEnvironmentGateway(environment.ID)
	if err != nil {
		return handler.convertError(err)
	}
	if gateway == nil {
		return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Gateway target is unavailable", "GATEWAY_TARGET_UNAVAILABLE", nil)
	}
	if handlerErr = handler.requireEndpointAccess(r, gateway.EndpointID); handlerErr != nil {
		return handlerErr
	}
	results := make([]targetPreflightResult, 0)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	for _, target := range environment.Targets {
		if !target.Enabled || target.Role != portainer.PlatformDeploymentTargetRoleWorkload {
			continue
		}
		result := targetPreflightResult{EndpointID: target.EndpointID, Status: "failed"}
		port := handler.deploymentPublishedPort(*deployment, target.EndpointID, payload.TargetPort)
		if port == 0 {
			result.Reason = "TARGET_PORT_UNAVAILABLE"
		} else if err := handler.GatewayRuntime.ProbeHTTP(ctx, *gateway, target.HostAddress, port); err != nil {
			result.Reason = "TARGET_UNREACHABLE"
		} else {
			result.Status = "succeeded"
		}
		results = append(results, result)
	}
	return response.JSON(w, results)
}

func (handler *Handler) activeEnvironmentGateway(environmentID portainer.PlatformEnvironmentID) (*portainer.PlatformGateway, error) {
	gateways, err := handler.DataStore.PlatformGateway().ReadAll(func(gateway portainer.PlatformGateway) bool {
		return gateway.EnvironmentID == environmentID && isActive(gateway.PlatformLifecycle)
	})
	if err != nil || len(gateways) != 1 {
		return nil, err
	}
	return &gateways[0], nil
}

func (handler *Handler) deploymentPublishedPort(deployment portainer.PlatformServiceDeployment, endpointID portainer.EndpointID, targetPort int) int {
	if deployment.CurrentServingReleaseID <= 0 {
		return 0
	}
	release, err := handler.DataStore.PlatformRelease().Read(deployment.CurrentServingReleaseID)
	if err != nil || release.Status != portainer.PlatformReleaseStatusSucceeded {
		return 0
	}
	for _, target := range release.TargetResults {
		if target.EndpointID != endpointID || target.Status != portainer.PlatformReleaseTargetStatusSucceeded {
			continue
		}
		for _, port := range target.PublishedPorts {
			if port.ContainerPort == targetPort && port.Protocol == portainer.PlatformPortProtocolTCP && port.HostPort > 0 {
				return port.HostPort
			}
		}
	}
	// single Release 兼容路径只允许匹配实际当前 runtime 的 Endpoint，不能把该端口复用给其它 host。
	if deployment.CurrentRuntimeRef.EndpointID != endpointID {
		return 0
	}
	for _, port := range release.RuntimeSnapshot.PublishedPorts {
		if port.ContainerPort == targetPort && port.Protocol == portainer.PlatformPortProtocolTCP && port.HostPort > 0 {
			return port.HostPort
		}
	}
	return 0
}
