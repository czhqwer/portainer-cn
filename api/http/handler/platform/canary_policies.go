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

type createCanaryPolicyPayload struct {
	EnvironmentID   portainer.PlatformEnvironmentID  `json:"EnvironmentId"`
	GatewayRouteID  portainer.PlatformGatewayRouteID `json:"GatewayRouteId"`
	StableReleaseID portainer.PlatformReleaseID      `json:"StableReleaseId"`
	CanaryReleaseID portainer.PlatformReleaseID      `json:"CanaryReleaseId"`
}
type canaryWeightPayload struct {
	ResourceVersion int `json:"ResourceVersion"`
	Weight          int `json:"Weight"`
}
type canaryRollbackPayload struct {
	ResourceVersion int `json:"ResourceVersion"`
}

func (p *createCanaryPolicyPayload) Validate(*http.Request) error {
	if p.EnvironmentID <= 0 || p.GatewayRouteID <= 0 || p.StableReleaseID <= 0 || p.CanaryReleaseID <= 0 || p.StableReleaseID == p.CanaryReleaseID {
		return errors.New("environment, route and two releases are required")
	}
	return nil
}
func (p *canaryWeightPayload) Validate(*http.Request) error {
	if err := validateResourceVersion(p.ResourceVersion); err != nil {
		return err
	}
	return portainer.ValidatePlatformCanaryWeight(p.Weight)
}
func (p *canaryRollbackPayload) Validate(*http.Request) error {
	return validateResourceVersion(p.ResourceVersion)
}

func (handler *Handler) canaryPolicyList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, e := handler.routeID(r, "projectId")
	if e != nil {
		return e
	}
	if _, e = handler.requireProjectPermission(r, portainer.PlatformProjectID(id), platformPermissionView); e != nil {
		return e
	}
	items, err := handler.DataStore.PlatformCanaryPolicy().ReadAll(func(p portainer.PlatformCanaryPolicy) bool {
		return p.ProjectID == portainer.PlatformProjectID(id) && isActive(p.PlatformLifecycle)
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, items)
}
func (handler *Handler) canaryPolicyCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, e := handler.routeID(r, "projectId")
	if e != nil {
		return e
	}
	if _, e = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); e != nil {
		return e
	}
	var p createCanaryPolicyPayload
	if err := request.DecodeAndValidateJSONPayload(r, &p); err != nil {
		return validationFailed(err)
	}
	route, err := handler.DataStore.PlatformGatewayRoute().Read(p.GatewayRouteID)
	if err != nil {
		return handler.convertError(err)
	}
	if route.ProjectID != portainer.PlatformProjectID(projectID) || route.EnvironmentID != p.EnvironmentID {
		return platformAccessDenied()
	}
	gateway, err := handler.DataStore.PlatformGateway().Read(route.GatewayID)
	if err != nil {
		return handler.convertError(err)
	}
	if e := handler.requireEndpointAccess(r, gateway.EndpointID); e != nil {
		return e
	}
	stable, err := handler.DataStore.PlatformRelease().Read(p.StableReleaseID)
	if err != nil {
		return handler.convertError(err)
	}
	canary, err := handler.DataStore.PlatformRelease().Read(p.CanaryReleaseID)
	if err != nil {
		return handler.convertError(err)
	}
	if stable.Status != portainer.PlatformReleaseStatusSucceeded || canary.Status != portainer.PlatformReleaseStatusSucceeded || canary.HealthCheckResult.Status != portainer.PlatformHealthCheckStatusPassed || stable.ServiceDeploymentID != route.ServiceDeploymentID || canary.ServiceDeploymentID != route.ServiceDeploymentID {
		return validationFailedError("Canary releases must be successful and healthy")
	}
	policy := portainer.NewPlatformCanaryPolicy()
	policy.ProjectID = portainer.PlatformProjectID(projectID)
	policy.EnvironmentID = p.EnvironmentID
	policy.GatewayRouteID = p.GatewayRouteID
	policy.StableReleaseID = p.StableReleaseID
	policy.CanaryReleaseID = p.CanaryReleaseID
	policy.PlatformLifecycle = newLifecycle(time.Now().Unix())
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		items, err := tx.PlatformCanaryPolicy().ReadAll(func(x portainer.PlatformCanaryPolicy) bool {
			return x.GatewayRouteID == policy.GatewayRouteID && isActive(x.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(items) > 0 {
			return conflictError("Gateway route already has an active canary policy")
		}
		if err := tx.PlatformCanaryPolicy().Create(&policy); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionCanaryCreated, Result: portainer.PlatformAuditResultSuccess, ProjectID: policy.ProjectID, EnvironmentID: policy.EnvironmentID, AfterSummary: map[string]any{"canaryPolicyId": policy.ID, "gatewayRouteId": policy.GatewayRouteID}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSONWithStatus(w, policy, http.StatusCreated)
}
func allowedCanaryTransition(current, next int) bool {
	return map[int]map[int]bool{0: {5: true}, 5: {0: true, 25: true}, 25: {0: true, 5: true, 50: true}, 50: {0: true, 25: true, 100: true}, 100: {0: true, 50: true}}[current][next]
}

func (handler *Handler) canaryPolicyInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	p, e := handler.canaryPolicyFromRequest(r, platformPermissionView)
	if e != nil {
		return e
	}
	return response.JSON(w, p)
}
func (handler *Handler) canaryPolicyWeightChange(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	p, e := handler.canaryPolicyFromRequest(r, platformPermissionManage)
	if e != nil {
		return e
	}
	var body canaryWeightPayload
	if err := request.DecodeAndValidateJSONPayload(r, &body); err != nil {
		return validationFailed(err)
	}
	return handler.changeCanaryWeight(w, r, p, body.ResourceVersion, body.Weight)
}
func (handler *Handler) canaryPolicyRollback(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	p, e := handler.canaryPolicyFromRequest(r, platformPermissionManage)
	if e != nil {
		return e
	}
	var body canaryRollbackPayload
	if err := request.DecodeAndValidateJSONPayload(r, &body); err != nil {
		return validationFailed(err)
	}
	return handler.changeCanaryWeight(w, r, p, body.ResourceVersion, 0)
}
func (handler *Handler) canaryPolicyFromRequest(r *http.Request, permission platformPermission) (*portainer.PlatformCanaryPolicy, *httperror.HandlerError) {
	id, e := handler.routeID(r, "canaryPolicyId")
	if e != nil {
		return nil, e
	}
	p, err := handler.DataStore.PlatformCanaryPolicy().Read(portainer.PlatformCanaryPolicyID(id))
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, e = handler.requireProjectPermission(r, p.ProjectID, permission); e != nil {
		return nil, e
	}
	route, err := handler.DataStore.PlatformGatewayRoute().Read(p.GatewayRouteID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	gateway, err := handler.DataStore.PlatformGateway().Read(route.GatewayID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if e = handler.requireEndpointAccess(r, gateway.EndpointID); e != nil {
		return nil, e
	}
	return p, nil
}
func (handler *Handler) changeCanaryWeight(w http.ResponseWriter, r *http.Request, p *portainer.PlatformCanaryPolicy, version, next int) *httperror.HandlerError {
	if err := requireResourceVersion(p.ResourceVersion, version); err != nil {
		return handler.convertError(err)
	}
	if !allowedCanaryTransition(p.CurrentWeight, next) {
		return validationFailedError("Canary weight transition is invalid")
	}
	if next > 0 {
		candidate, err := handler.DataStore.PlatformRelease().Read(p.CanaryReleaseID)
		if err != nil || candidate.Status != portainer.PlatformReleaseStatusSucceeded || candidate.HealthCheckResult.Status != portainer.PlatformHealthCheckStatusPassed {
			_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
				return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionCanaryHealthFailed, Result: portainer.PlatformAuditResultFailed, FailureReason: "CANARY_HEALTH_FAILED", ProjectID: p.ProjectID, EnvironmentID: p.EnvironmentID, AfterSummary: map[string]any{"canaryPolicyId": p.ID, "canaryReleaseId": p.CanaryReleaseID}})
			})
			return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Canary release is not healthy", "CANARY_HEALTH_FAILED", nil)
		}
	}
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformCanaryPolicy().Read(p.ID)
		if err != nil {
			return err
		}
		if err := requireResourceVersion(current.ResourceVersion, version); err != nil {
			return err
		}
		current.CurrentWeight = next
		current.LastFailureReason = ""
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		return tx.PlatformCanaryPolicy().Update(current.ID, current)
	})
	if err != nil {
		return handler.convertError(err)
	}
	route, err := handler.DataStore.PlatformGatewayRoute().Read(p.GatewayRouteID)
	if err != nil {
		return handler.convertError(err)
	}
	gateway, err := handler.DataStore.PlatformGateway().Read(route.GatewayID)
	if err != nil {
		return handler.convertError(err)
	}
	if _, reason, err := handler.applyGatewayConfiguration(r, gateway); err != nil {
		_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
			current, readErr := tx.PlatformCanaryPolicy().Read(p.ID)
			if readErr != nil {
				return readErr
			}
			// 发布失败时不能继续让候选版本承接旧流量；稳定 Release 仍由渲染器的默认 upstream 保持服务。
			current.CurrentWeight = 0
			current.LastFailureReason = reason
			touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
			return tx.PlatformCanaryPolicy().Update(current.ID, current)
		})
		return writePlatformError(w, http.StatusBadGateway, errPlatformValidationFailed, "Canary gateway publish failed", reason, nil)
	}
	updated, err := handler.DataStore.PlatformCanaryPolicy().Read(p.ID)
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, updated)
}
