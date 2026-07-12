package platform

import (
	"crypto/sha256"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

// GatewayRouteTarget 是网关渲染阶段已经解析完成的安全输入。
// upstream 只能来自本次发布记录的宿主机发布端口，不能接受用户填写的 URL 或 Nginx 片段。
type GatewayRouteTarget struct {
	Route        portainer.PlatformGatewayRoute
	UpstreamHost string
	UpstreamPort int
}

// RenderGatewayConfig 将受控路由渲染为确定性 Nginx 配置。
// 这里同时排序和校验所有字段，保证同一组业务事实始终得到同一个 hash，
// 且任何可能形成 Nginx 指令注入的字符都会在写入候选配置前被拒绝。
func RenderGatewayConfig(routes []GatewayRouteTarget) ([]byte, string, error) {
	if len(routes) == 0 {
		return nil, "", fmt.Errorf("gateway config requires routes")
	}

	normalized := append([]GatewayRouteTarget(nil), routes...)
	seen := make(map[string]struct{}, len(normalized))
	for i := range normalized {
		portainer.NormalizePlatformGatewayRoute(&normalized[i].Route)
		if err := portainer.ValidatePlatformGatewayRoute(normalized[i].Route); err != nil {
			return nil, "", err
		}
		if err := validateGatewayUpstream(normalized[i].UpstreamHost, normalized[i].UpstreamPort); err != nil {
			return nil, "", err
		}
		key := normalized[i].Route.Domain + "\x00" + normalized[i].Route.Path
		if _, exists := seen[key]; exists {
			return nil, "", fmt.Errorf("gateway route domain and path are duplicated")
		}
		seen[key] = struct{}{}
	}

	sort.Slice(normalized, func(i, j int) bool {
		left, right := normalized[i].Route, normalized[j].Route
		if left.Domain != right.Domain {
			return left.Domain < right.Domain
		}
		// 同一域名下先渲染更长路径，避免 / 覆盖 /api 等具体路由。
		if len(left.Path) != len(right.Path) {
			return len(left.Path) > len(right.Path)
		}
		return left.ID < right.ID
	})

	servers := make(map[string]*gatewayServerConfig)
	for _, target := range normalized {
		server, exists := servers[target.Route.Domain]
		if !exists {
			server = &gatewayServerConfig{domain: target.Route.Domain, enableTLS: target.Route.EnableTLS, forceHTTPS: target.Route.ForceHTTPS, certificateID: target.Route.CertificateID}
			servers[target.Route.Domain] = server
		} else if server.enableTLS != target.Route.EnableTLS || server.forceHTTPS != target.Route.ForceHTTPS || server.certificateID != target.Route.CertificateID {
			return nil, "", fmt.Errorf("gateway routes for the same domain must use one TLS policy")
		}
		server.routes = append(server.routes, target)
	}

	serverList := make([]*gatewayServerConfig, 0, len(servers))
	for _, server := range servers {
		serverList = append(serverList, server)
	}
	sort.Slice(serverList, func(i, j int) bool { return serverList[i].domain < serverList[j].domain })

	var builder strings.Builder
	for _, server := range serverList {
		writeGatewayServerBlock(&builder, server)
	}

	config := []byte(builder.String())
	hash := fmt.Sprintf("%x", sha256.Sum256(config))
	return config, hash, nil
}

func validateGatewayUpstream(host string, port int) error {
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, "\r\n\x00/\\@") || port < 1 || port > 65535 {
		return fmt.Errorf("gateway upstream is invalid")
	}
	if net.ParseIP(host) == nil && !isGatewayHostname(host) {
		return fmt.Errorf("gateway upstream host is invalid")
	}
	return nil
}

func isGatewayHostname(host string) bool {
	if len(host) > 253 || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

type gatewayServerConfig struct {
	domain        string
	enableTLS     bool
	forceHTTPS    bool
	certificateID portainer.PlatformGatewayCertificateID
	routes        []GatewayRouteTarget
}

func writeGatewayServerBlock(builder *strings.Builder, server *gatewayServerConfig) {
	if server.enableTLS {
		fmt.Fprintf(builder, "server {\n    listen 443 ssl;\n    server_name %s;\n    ssl_certificate /etc/nginx/portainer/certs/%d.crt;\n    ssl_certificate_key /etc/nginx/portainer/certs/%d.key;\n", server.domain, server.certificateID, server.certificateID)
		for _, target := range server.routes {
			writeGatewayLocation(builder, target.Route, net.JoinHostPort(target.UpstreamHost, strconv.Itoa(target.UpstreamPort)))
		}
		builder.WriteString("}\n\n")
		if server.forceHTTPS {
			fmt.Fprintf(builder, "server {\n    listen 80;\n    server_name %s;\n    return 301 https://$host$request_uri;\n}\n\n", server.domain)
		}
		return
	}

	fmt.Fprintf(builder, "server {\n    listen 80;\n    server_name %s;\n", server.domain)
	for _, target := range server.routes {
		writeGatewayLocation(builder, target.Route, net.JoinHostPort(target.UpstreamHost, strconv.Itoa(target.UpstreamPort)))
	}
	builder.WriteString("}\n\n")
}

func writeGatewayLocation(builder *strings.Builder, route portainer.PlatformGatewayRoute, upstream string) {
	fmt.Fprintf(builder, "    location %s {\n        proxy_pass http://%s;\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_read_timeout %ds;\n", route.Path, upstream, route.ProxyTimeoutSeconds)
	if route.MaxRequestBodyBytes > 0 {
		fmt.Fprintf(builder, "        client_max_body_size %d;\n", route.MaxRequestBodyBytes)
	}
	if route.WebSocket {
		builder.WriteString("        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection \"upgrade\";\n")
	}
	builder.WriteString("    }\n")
}
