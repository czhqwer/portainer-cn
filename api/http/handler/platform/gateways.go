package platform

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
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

type createGatewayCertificatePayload struct {
	Name           string `json:"Name"`
	CertificatePEM string `json:"CertificatePem"`
	PrivateKeyPEM  string `json:"PrivateKeyPem"`
}

func (payload *createGatewayCertificatePayload) Validate(r *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" || payload.CertificatePEM == "" || payload.PrivateKeyPEM == "" {
		return errors.New("name, certificate and private key are required")
	}
	return nil
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
	if handler.GatewayRuntime == nil || handler.FileService == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Gateway runtime is unavailable", "GATEWAY_RUNTIME_UNAVAILABLE", nil)
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
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}
	configStore, err := platformservice.NewGatewayConfigStore(handler.FileService.GetDatastorePath())
	if err == nil {
		_, err = configStore.EnsureActive(gateway.ID)
	}
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		gateway.ManagedContainerID, err = handler.GatewayRuntime.Ensure(ctx, gateway, handler.FileService.GetDatastorePath())
	}
	if err != nil {
		_ = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
			_ = tx.PlatformGateway().Delete(gateway.ID)
			return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionGatewayCreated, Result: portainer.PlatformAuditResultFailed, ProjectID: gateway.ProjectID, EnvironmentID: gateway.EnvironmentID, FailureReason: "GATEWAY_RUNTIME_CREATE_FAILED", AfterSummary: map[string]any{"gatewayId": gateway.ID, "endpointId": gateway.EndpointID}})
		})
		return writePlatformError(w, http.StatusBadGateway, errPlatformValidationFailed, "Gateway could not be created", "GATEWAY_RUNTIME_CREATE_FAILED", nil)
	}
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformGateway().Read(gateway.ID)
		if err != nil {
			return err
		}
		current.ManagedContainerID = gateway.ManagedContainerID
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformGateway().Update(current.ID, current); err != nil {
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
	if handlerErr := handler.requireGatewayRouteCertificate(payload, gateway); handlerErr != nil {
		return handlerErr
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

// requireGatewayRouteCertificate 在保存路由前验证证书仍归属同一项目且已具备受控私钥材料。
// 这能阻止失效、跨项目或仅有元数据的证书在 reload 时才暴露为不可恢复的 TLS 配置失败。
func (handler *Handler) requireGatewayRouteCertificate(payload createGatewayRoutePayload, gateway *portainer.PlatformGateway) *httperror.HandlerError {
	if !payload.EnableTLS {
		return nil
	}
	certificate, err := handler.DataStore.PlatformGatewayCertificate().Read(payload.CertificateID)
	if err != nil {
		return handler.convertError(err)
	}
	if certificate.ProjectID != gateway.ProjectID {
		return platformAccessDenied()
	}
	if !isActive(certificate.PlatformLifecycle) || !certificate.HasPrivateKey || certificate.NotAfter <= time.Now().Unix() {
		return validationFailedError("Certificate is unavailable")
	}
	routeDomain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(payload.Domain), "."))
	for _, domain := range certificate.Domains {
		if domain == routeDomain {
			return nil
		}
	}
	return validationFailedError("Certificate does not cover the route domain")
}

func (handler *Handler) gatewayCertificateList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	items, err := handler.DataStore.PlatformGatewayCertificate().ReadAll(func(item portainer.PlatformGatewayCertificate) bool {
		return item.ProjectID == portainer.PlatformProjectID(projectID) && (includeArchived(r) || isActive(item.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}
	for i := range items {
		items[i].MaterialRef = ""
	}
	return response.JSON(w, items)
}

// gatewayCertificateCreate 把私钥放入平台受控目录，而不是 BoltDB；数据库只在材料写入成功后记录不可透出的引用。
// 文件系统与 BoltDB 不能形成同一事务，因此每个失败分支都补偿清理，防止留下孤儿私钥或可见的无材料元数据。
func (handler *Handler) gatewayCertificateCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}
	var payload createGatewayCertificatePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	certificate, err := platformservice.ParseGatewayCertificate(payload.Name, portainer.PlatformProjectID(projectID), []byte(payload.CertificatePEM), []byte(payload.PrivateKeyPEM))
	if err != nil {
		return validationFailed(err)
	}
	certificate.HasPrivateKey, certificate.MaterialRef, certificate.PlatformLifecycle = false, "", newLifecycle(time.Now().Unix())
	if err := handler.DataStore.PlatformGatewayCertificate().Create(&certificate); err != nil {
		return handler.convertError(err)
	}
	if handler.FileService == nil {
		_ = handler.DataStore.PlatformGatewayCertificate().Delete(certificate.ID)
		return handler.convertError(errors.New("platform file service is unavailable"))
	}
	store, err := platformservice.NewGatewayCertificateStore(handler.FileService.GetDatastorePath())
	if err != nil {
		_ = handler.DataStore.PlatformGatewayCertificate().Delete(certificate.ID)
		return handler.convertError(err)
	}
	materialRef, err := store.Store(certificate.ID, []byte(payload.CertificatePEM), []byte(payload.PrivateKeyPEM))
	if err != nil {
		_ = handler.DataStore.PlatformGatewayCertificate().Delete(certificate.ID)
		return handler.convertError(err)
	}
	certificate.MaterialRef, certificate.HasPrivateKey = materialRef, true
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if err := tx.PlatformGatewayCertificate().Update(certificate.ID, &certificate); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{
			Action:    portainer.PlatformAuditActionGatewayCertificateCreated,
			Result:    portainer.PlatformAuditResultSuccess,
			ProjectID: certificate.ProjectID,
			AfterSummary: map[string]any{
				"gatewayCertificateId": certificate.ID,
				"notAfter":             certificate.NotAfter,
			},
		})
	})
	if err != nil {
		_ = store.Remove(certificate.ID)
		_ = handler.DataStore.PlatformGatewayCertificate().Delete(certificate.ID)
		return handler.convertError(err)
	}
	certificate.MaterialRef = ""
	return response.JSONWithStatus(w, certificate, http.StatusCreated)
}

// gatewayConfigApply 从已成功发布的运行快照生成 upstream，再执行候选预检和原子切换。
// 它刻意不接受 upstream、Nginx 文本或文件路径，保证切流只基于控制面已经验证过的发布事实。
func (handler *Handler) gatewayConfigApply(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	gateway, handlerErr := handler.gatewayFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handlerErr = handler.requireEndpointAccess(r, gateway.EndpointID); handlerErr != nil {
		return handlerErr
	}
	if handler.GatewayRuntime == nil || handler.FileService == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Gateway runtime is unavailable", "GATEWAY_RUNTIME_UNAVAILABLE", nil)
	}
	targets, handlerErr := handler.gatewayRouteTargets(gateway)
	if handlerErr != nil {
		return handlerErr
	}
	config, configHash, err := platformservice.RenderGatewayConfig(targets)
	if err != nil {
		return validationFailed(err)
	}
	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	version, err := handler.createGatewayConfigCandidate(gateway.ID, configHash, targets, userID)
	if err != nil {
		return handler.convertError(err)
	}
	store, err := platformservice.NewGatewayConfigStore(handler.FileService.GetDatastorePath())
	if err != nil {
		return handler.convertError(err)
	}
	publisher, err := platformservice.NewGatewayConfigPublisher(store, handler.GatewayRuntime)
	if err != nil {
		return handler.convertError(err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, publishErr := publisher.Publish(ctx, *gateway, config)
	if publishErr != nil {
		reason := gatewayConfigFailureReason(publishErr)
		_ = handler.finishGatewayConfigFailure(r, version.ID, gateway, reason)
		return writePlatformError(w, http.StatusBadGateway, errPlatformValidationFailed, "Gateway configuration was not applied", reason, nil)
	}
	if err := handler.finishGatewayConfigSuccess(r, version.ID, gateway.ID, result.CandidateHash); err != nil {
		return handler.convertError(err)
	}
	version.Status = portainer.PlatformGatewayConfigStatusActive
	return response.JSON(w, version)
}

func (handler *Handler) gatewayRouteTargets(gateway *portainer.PlatformGateway) ([]platformservice.GatewayRouteTarget, *httperror.HandlerError) {
	environment, err := handler.DataStore.PlatformEnvironment().Read(gateway.EnvironmentID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	var workload *portainer.PlatformDeploymentTarget
	for i := range environment.Targets {
		target := &environment.Targets[i]
		if target.Enabled && (target.Role == "" || target.Role == portainer.PlatformDeploymentTargetRoleWorkload) {
			if workload != nil {
				return nil, validationFailedError("Gateway requires exactly one enabled workload target")
			}
			workload = target
		}
	}
	if workload == nil || strings.TrimSpace(workload.HostAddress) == "" {
		return nil, validationFailedError("Gateway workload host address is required")
	}
	routes, err := handler.DataStore.PlatformGatewayRoute().ReadAll(func(route portainer.PlatformGatewayRoute) bool {
		return route.GatewayID == gateway.ID && isActive(route.PlatformLifecycle)
	})
	if err != nil {
		return nil, handler.convertError(err)
	}
	if len(routes) == 0 {
		return nil, validationFailedError("Gateway has no active routes")
	}
	targets := make([]platformservice.GatewayRouteTarget, 0, len(routes))
	for _, route := range routes {
		deployment, err := handler.DataStore.PlatformServiceDeployment().Read(route.ServiceDeploymentID)
		if err != nil {
			return nil, handler.convertError(err)
		}
		if deployment.CurrentServingReleaseID <= 0 || deployment.CurrentRuntimeRef.EndpointID != workload.EndpointID {
			return nil, validationFailedError("Route service is not serving on the workload target")
		}
		release, err := handler.DataStore.PlatformRelease().Read(deployment.CurrentServingReleaseID)
		if err != nil {
			return nil, handler.convertError(err)
		}
		if release.Status != portainer.PlatformReleaseStatusSucceeded || release.ServiceDeploymentID != deployment.ID {
			return nil, validationFailedError("Route service does not have a successful release")
		}
		hostPort := 0
		for _, published := range release.RuntimeSnapshot.PublishedPorts {
			if published.ContainerPort == route.TargetPort && published.Protocol == portainer.PlatformPortProtocolTCP {
				hostPort = published.HostPort
				break
			}
		}
		if hostPort <= 0 {
			return nil, validationFailedError("Route service published port is unavailable")
		}
		targets = append(targets, platformservice.GatewayRouteTarget{Route: route, UpstreamHost: workload.HostAddress, UpstreamPort: hostPort})
	}
	return targets, nil
}

func (handler *Handler) createGatewayConfigCandidate(gatewayID portainer.PlatformGatewayID, configHash string, targets []platformservice.GatewayRouteTarget, userID portainer.UserID) (portainer.PlatformGatewayConfigVersion, error) {
	version := portainer.PlatformGatewayConfigVersion{GatewayID: gatewayID, Status: portainer.PlatformGatewayConfigStatusCandidate, ConfigHash: configHash, CreatedAt: time.Now().Unix(), CreatedByUserID: userID}
	for _, target := range targets {
		version.RouteIDs = append(version.RouteIDs, target.Route.ID)
	}
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		versions, err := tx.PlatformGatewayConfigVersion().ReadAll(func(item portainer.PlatformGatewayConfigVersion) bool { return item.GatewayID == gatewayID })
		if err != nil {
			return err
		}
		for _, item := range versions {
			if item.Revision >= version.Revision {
				version.Revision = item.Revision + 1
			}
		}
		return tx.PlatformGatewayConfigVersion().Create(&version)
	})
	return version, err
}

func (handler *Handler) finishGatewayConfigSuccess(r *http.Request, versionID portainer.PlatformGatewayConfigVersionID, gatewayID portainer.PlatformGatewayID, configHash string) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		version, err := tx.PlatformGatewayConfigVersion().Read(versionID)
		if err != nil {
			return err
		}
		version.Status, version.FailureReason = portainer.PlatformGatewayConfigStatusActive, ""
		if err := tx.PlatformGatewayConfigVersion().Update(version.ID, version); err != nil {
			return err
		}
		gateway, err := tx.PlatformGateway().Read(gatewayID)
		if err != nil {
			return err
		}
		gateway.ActiveConfigHash = configHash
		touchLifecycle(&gateway.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformGateway().Update(gateway.ID, gateway); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionGatewayConfigApplied, Result: portainer.PlatformAuditResultSuccess, ProjectID: gateway.ProjectID, EnvironmentID: gateway.EnvironmentID, AfterSummary: map[string]any{"gatewayId": gateway.ID, "configHash": configHash}})
	})
}

func (handler *Handler) finishGatewayConfigFailure(r *http.Request, versionID portainer.PlatformGatewayConfigVersionID, gateway *portainer.PlatformGateway, reason string) error {
	return handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		version, err := tx.PlatformGatewayConfigVersion().Read(versionID)
		if err != nil {
			return err
		}
		version.Status, version.FailureReason = portainer.PlatformGatewayConfigStatusFailed, reason
		if err := tx.PlatformGatewayConfigVersion().Update(version.ID, version); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionGatewayConfigFailed, Result: portainer.PlatformAuditResultFailed, ProjectID: gateway.ProjectID, EnvironmentID: gateway.EnvironmentID, FailureReason: reason, AfterSummary: map[string]any{"gatewayId": gateway.ID, "configVersionId": version.ID}})
	})
}

func gatewayConfigFailureReason(err error) string {
	var publishErr *platformservice.GatewayConfigPublishError
	if errors.As(err, &publishErr) {
		return publishErr.Reason
	}
	return "GATEWAY_CONFIG_APPLY_FAILED"
}
