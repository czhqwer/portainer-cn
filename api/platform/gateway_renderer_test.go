package platform

import (
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestRenderGatewayConfigIsDeterministicAndSortsSpecificPathsFirst(t *testing.T) {
	root := gatewayRoute(1, "/")
	api := gatewayRoute(2, "/api")
	config, hash, err := RenderGatewayConfig([]GatewayRouteTarget{
		{Route: root, UpstreamHost: "127.0.0.1", UpstreamPort: 18080},
		{Route: api, UpstreamHost: "127.0.0.1", UpstreamPort: 18081},
	})
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.Less(t, stringIndex(t, string(config), "location /api"), stringIndex(t, string(config), "location / {"))

	second, secondHash, err := RenderGatewayConfig([]GatewayRouteTarget{
		{Route: api, UpstreamHost: "127.0.0.1", UpstreamPort: 18081},
		{Route: root, UpstreamHost: "127.0.0.1", UpstreamPort: 18080},
	})
	require.NoError(t, err)
	require.Equal(t, config, second)
	require.Equal(t, hash, secondHash)
}

func TestRenderGatewayConfigRejectsUnsafeInputs(t *testing.T) {
	route := gatewayRoute(1, "/")
	route.Domain = "api.example.com\nlisten 9999"
	_, _, err := RenderGatewayConfig([]GatewayRouteTarget{{Route: route, UpstreamHost: "127.0.0.1", UpstreamPort: 8080}})
	require.Error(t, err)

	route = gatewayRoute(1, "/")
	_, _, err = RenderGatewayConfig([]GatewayRouteTarget{{Route: route, UpstreamHost: "127.0.0.1;include /tmp/x", UpstreamPort: 8080}})
	require.Error(t, err)
}

func TestRenderGatewayConfigTLSAndWebSocket(t *testing.T) {
	route := gatewayRoute(3, "/socket")
	route.EnableTLS = true
	route.ForceHTTPS = true
	route.WebSocket = true
	route.CertificateID = 9
	config, _, err := RenderGatewayConfig([]GatewayRouteTarget{{Route: route, UpstreamHost: "gateway.internal", UpstreamPort: 19090}})
	require.NoError(t, err)
	require.Contains(t, string(config), "ssl_certificate /etc/nginx/portainer/certificates/9/cert.pem")
	require.Contains(t, string(config), "events {")
	require.Contains(t, string(config), "return 301 https://$host$request_uri")
	require.Contains(t, string(config), "Connection \"upgrade\"")
}

func TestRenderGatewayConfigUsesDeterministicMultiHostUpstream(t *testing.T) {
	route := gatewayRoute(5, "/")
	config, _, err := RenderGatewayConfig([]GatewayRouteTarget{{
		Route: route,
		Upstreams: []GatewayUpstreamTarget{
			{Host: "10.0.0.12", Port: 18080},
			{Host: "10.0.0.11", Port: 18080},
		},
	}})
	require.NoError(t, err)
	value := string(config)
	require.Contains(t, value, "upstream platform_route_5")
	require.Less(t, stringIndex(t, value, "10.0.0.11:18080"), stringIndex(t, value, "10.0.0.12:18080"))
	require.Contains(t, value, "proxy_pass http://platform_route_5")
}

func gatewayRoute(id portainer.PlatformGatewayRouteID, path string) portainer.PlatformGatewayRoute {
	route := portainer.NewPlatformGatewayRoute()
	route.ID = id
	route.GatewayID = 1
	route.ProjectID = 1
	route.EnvironmentID = 1
	route.ServiceDeploymentID = 1
	route.Domain = "example.com"
	route.Path = path
	route.TargetPort = 8080
	return route
}

func stringIndex(t *testing.T, value, target string) int {
	index := len(value)
	for i := range value {
		if len(value)-i >= len(target) && value[i:i+len(target)] == target {
			return i
		}
	}
	t.Fatalf("%q not found", target)
	return index
}
