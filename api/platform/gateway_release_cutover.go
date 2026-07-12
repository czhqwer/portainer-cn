package platform

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

// GatewayReleaseCutover 仅从已持久化的路由、环境目标和 Release 运行快照构造 upstream。
// 因此发布请求不能借由网关路径注入任意主机、端口、Nginx 指令或受控文件系统路径。
type GatewayReleaseCutover struct {
	dataStore     dataservices.DataStore
	datastorePath string
	runtime       GatewayRuntime
}

func NewGatewayReleaseCutover(dataStore dataservices.DataStore, datastorePath string, runtime GatewayRuntime) *GatewayReleaseCutover {
	return &GatewayReleaseCutover{dataStore: dataStore, datastorePath: datastorePath, runtime: runtime}
}

func (cutover *GatewayReleaseCutover) Cutover(ctx context.Context, request ReleaseExecutionRequest, release portainer.PlatformRelease) (portainer.PlatformGatewaySnapshot, error) {
	if cutover == nil || cutover.dataStore == nil || cutover.runtime == nil || strings.TrimSpace(cutover.datastorePath) == "" {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway cutover is unavailable"}
	}
	if request.Environment.TargetMode == portainer.PlatformTargetModeMulti {
		return cutover.multiCutover(ctx, request, release)
	}
	gateway, routes, workload, err := cutover.activeGatewayRoutes(request.Environment)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, err
	}
	if gateway == nil || len(routes) == 0 {
		return portainer.PlatformGatewaySnapshot{}, nil
	}
	targets, err := cutover.routeTargets(request, release, routes, workload)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, err
	}
	config, hash, err := RenderGatewayConfig(targets)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway config is invalid"}
	}
	store, err := NewGatewayConfigStore(cutover.datastorePath)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway config store is unavailable"}
	}
	publisher, err := NewGatewayConfigPublisher(store, cutover.runtime)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway publisher is unavailable"}
	}
	result, err := publisher.Publish(ctx, *gateway, config)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway reload failed"}
	}
	snapshot := gatewaySnapshotFromTargets(targets, result.PreviousHash, result.CandidateHash)
	snapshot.ConfigHash = hash
	return snapshot, nil
}

func (cutover *GatewayReleaseCutover) multiCutover(ctx context.Context, request ReleaseExecutionRequest, release portainer.PlatformRelease) (portainer.PlatformGatewaySnapshot, error) {
	gateways, err := cutover.dataStore.PlatformGateway().ReadAll(func(item portainer.PlatformGateway) bool {
		return item.EnvironmentID == request.Environment.ID && item.LifecycleStatus == portainer.PlatformLifecycleStatusActive
	})
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, err
	}
	if len(gateways) == 0 {
		return portainer.PlatformGatewaySnapshot{}, nil
	}
	if len(gateways) != 1 || gateways[0].ManagedContainerID == "" {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway is unavailable"}
	}
	routes, err := cutover.dataStore.PlatformGatewayRoute().ReadAll(func(route portainer.PlatformGatewayRoute) bool {
		return route.GatewayID == gateways[0].ID && route.LifecycleStatus == portainer.PlatformLifecycleStatusActive
	})
	if err != nil || len(routes) == 0 {
		return portainer.PlatformGatewaySnapshot{}, err
	}
	hosts := make(map[portainer.EndpointID]string)
	for _, target := range request.Environment.Targets {
		if target.Enabled && target.Role == portainer.PlatformDeploymentTargetRoleWorkload && strings.TrimSpace(target.HostAddress) != "" {
			hosts[target.EndpointID] = target.HostAddress
		}
	}
	targets, err := cutover.multiRouteTargets(request, release, routes, hosts)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, err
	}
	config, hash, err := RenderGatewayConfig(targets)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway config is invalid"}
	}
	store, err := NewGatewayConfigStore(cutover.datastorePath)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway config store is unavailable"}
	}
	publisher, err := NewGatewayConfigPublisher(store, cutover.runtime)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway publisher is unavailable"}
	}
	result, err := publisher.Publish(ctx, gateways[0], config)
	if err != nil {
		return portainer.PlatformGatewaySnapshot{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway reload failed"}
	}
	snapshot := gatewaySnapshotFromTargets(targets, result.PreviousHash, result.CandidateHash)
	snapshot.ConfigHash = hash
	return snapshot, nil
}

func (cutover *GatewayReleaseCutover) multiRouteTargets(request ReleaseExecutionRequest, currentRelease portainer.PlatformRelease, routes []portainer.PlatformGatewayRoute, hosts map[portainer.EndpointID]string) ([]GatewayRouteTarget, error) {
	targets := make([]GatewayRouteTarget, 0, len(routes))
	for _, route := range routes {
		release := currentRelease
		if route.ServiceDeploymentID != request.Deployment.ID {
			deployment, err := cutover.dataStore.PlatformServiceDeployment().Read(route.ServiceDeploymentID)
			if err != nil || deployment.CurrentServingReleaseID <= 0 {
				return nil, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "routed deployment is unavailable"}
			}
			stored, err := cutover.dataStore.PlatformRelease().Read(deployment.CurrentServingReleaseID)
			if err != nil || stored.Status != portainer.PlatformReleaseStatusSucceeded {
				return nil, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "routed release is unavailable"}
			}
			release = *stored
		}
		upstreams := make([]GatewayUpstreamTarget, 0)
		for _, result := range release.TargetResults {
			if result.Status != portainer.PlatformReleaseTargetStatusSucceeded {
				continue
			}
			port := publishedPortForContainerPort(result.PublishedPorts, route.TargetPort)
			if host := hosts[result.EndpointID]; host != "" && port > 0 {
				upstreams = append(upstreams, GatewayUpstreamTarget{Host: host, Port: port})
			}
		}
		if len(upstreams) == 0 {
			return nil, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "routed target is unavailable"}
		}
		targets = append(targets, GatewayRouteTarget{Route: route, Upstreams: upstreams})
	}
	return targets, nil
}

func (cutover *GatewayReleaseCutover) activeGatewayRoutes(environment portainer.PlatformEnvironment) (*portainer.PlatformGateway, []portainer.PlatformGatewayRoute, portainer.PlatformDeploymentTarget, error) {
	gateways, err := cutover.dataStore.PlatformGateway().ReadAll(func(item portainer.PlatformGateway) bool {
		return item.EnvironmentID == environment.ID && item.LifecycleStatus == portainer.PlatformLifecycleStatusActive
	})
	if err != nil {
		return nil, nil, portainer.PlatformDeploymentTarget{}, err
	}
	if len(gateways) == 0 {
		// 未登记中心网关的既有环境继续走阶段 1-3 的发布路径；
		// 仅在确有网关路由时要求 HostAddress 和网关运行时可用。
		return nil, nil, portainer.PlatformDeploymentTarget{}, nil
	}
	if len(gateways) != 1 || gateways[0].ManagedContainerID == "" {
		return nil, nil, portainer.PlatformDeploymentTarget{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway is unavailable"}
	}
	routes, err := cutover.dataStore.PlatformGatewayRoute().ReadAll(func(route portainer.PlatformGatewayRoute) bool {
		return route.GatewayID == gateways[0].ID && route.LifecycleStatus == portainer.PlatformLifecycleStatusActive
	})
	if err != nil {
		return nil, nil, portainer.PlatformDeploymentTarget{}, err
	}
	if len(routes) == 0 {
		return &gateways[0], nil, portainer.PlatformDeploymentTarget{}, nil
	}
	workload, err := selectSingleWorkloadTarget(environment)
	if err != nil || strings.TrimSpace(workload.HostAddress) == "" {
		return nil, nil, portainer.PlatformDeploymentTarget{}, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "gateway workload target is invalid"}
	}
	return &gateways[0], routes, workload, nil
}

func (cutover *GatewayReleaseCutover) routeTargets(request ReleaseExecutionRequest, currentRelease portainer.PlatformRelease, routes []portainer.PlatformGatewayRoute, workload portainer.PlatformDeploymentTarget) ([]GatewayRouteTarget, error) {
	targets := make([]GatewayRouteTarget, 0, len(routes))
	for _, route := range routes {
		release := currentRelease
		if route.ServiceDeploymentID != request.Deployment.ID {
			deployment, err := cutover.dataStore.PlatformServiceDeployment().Read(route.ServiceDeploymentID)
			if err != nil || deployment.CurrentServingReleaseID <= 0 {
				return nil, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "routed deployment is unavailable"}
			}
			storedRelease, err := cutover.dataStore.PlatformRelease().Read(deployment.CurrentServingReleaseID)
			if err != nil || storedRelease.Status != portainer.PlatformReleaseStatusSucceeded {
				return nil, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "routed release is unavailable"}
			}
			release = *storedRelease
		}
		port := publishedPortForContainerPort(release.RuntimeSnapshot.PublishedPorts, route.TargetPort)
		if port <= 0 {
			return nil, codedRuntimeError{reason: ReleaseFailureReasonGatewayCutoverFailed, message: "routed port is unavailable"}
		}
		targets = append(targets, GatewayRouteTarget{Route: route, UpstreamHost: workload.HostAddress, UpstreamPort: port})
	}
	return targets, nil
}

func publishedPortForContainerPort(ports []portainer.PlatformPublishedPort, containerPort int) int {
	for _, published := range ports {
		if published.ContainerPort == containerPort && published.Protocol == portainer.PlatformPortProtocolTCP && published.HostPort > 0 {
			return published.HostPort
		}
	}
	return 0
}

func gatewaySnapshotFromTargets(targets []GatewayRouteTarget, beforeHash, afterHash string) portainer.PlatformGatewaySnapshot {
	sorted := append([]GatewayRouteTarget(nil), targets...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Route.Domain != sorted[j].Route.Domain {
			return sorted[i].Route.Domain < sorted[j].Route.Domain
		}
		return sorted[i].Route.Path < sorted[j].Route.Path
	})
	if len(sorted) == 0 {
		return portainer.PlatformGatewaySnapshot{}
	}
	first := sorted[0]
	upstreamHost, upstreamPort := first.UpstreamHost, first.UpstreamPort
	if len(first.Upstreams) > 0 {
		upstreamHost, upstreamPort = first.Upstreams[0].Host, first.Upstreams[0].Port
	}
	snapshot := portainer.PlatformGatewaySnapshot{
		Domain:              first.Route.Domain,
		Path:                first.Route.Path,
		Upstream:            net.JoinHostPort(upstreamHost, fmt.Sprintf("%d", upstreamPort)),
		ReloadBeforeVersion: beforeHash,
		ReloadAfterVersion:  afterHash,
	}
	if first.Route.CertificateID > 0 {
		snapshot.CertificateRef = fmt.Sprintf("gateway-certificate:%d", first.Route.CertificateID)
	}
	return snapshot
}

var _ GatewayCutover = (*GatewayReleaseCutover)(nil)
