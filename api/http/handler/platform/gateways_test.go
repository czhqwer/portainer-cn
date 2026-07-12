package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestGatewayCreateListAndInspect(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Gateway project", Slug: "gateway-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	endpoint := &portainer.Endpoint{ID: 31, Name: "gateway", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))

	created := doJSON[portainer.PlatformGateway](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "prod-gateway"}, http.StatusCreated)
	require.Equal(t, project.ID, created.ProjectID)
	require.Equal(t, endpoint.ID, created.EndpointID)

	items := doJSON[[]portainer.PlatformGateway](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), nil, http.StatusOK)
	require.Len(t, items, 1)
	inspected := doJSON[portainer.PlatformGateway](t, ctx, http.MethodGet, fmt.Sprintf("/platform/gateways/%d", created.ID), nil, http.StatusOK)
	require.Equal(t, created.Name, inspected.Name)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: 999, Name: "missing"}, http.StatusNotFound)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "duplicate"}, http.StatusConflict)
}

func TestGatewayRouteCreateValidatesDeploymentAndConflict(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Route project", Slug: "route-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	endpoint := &portainer.Endpoint{ID: 32, Name: "gateway", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	gateway := doJSON[portainer.PlatformGateway](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "prod-gateway"}, http.StatusCreated)
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "API", Slug: "api"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "API", Slug: "api"})
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/api:1"
	spec.Ports = []portainer.PlatformPortSpec{{Name: "http", ContainerPort: 8080, HostPort: 18080}}
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{EnvironmentID: environment.ID, DesiredSpec: &spec})
	payload := createGatewayRoutePayload{ServiceDeploymentID: deployment.ID, Domain: "api.example.test", Path: "/api", TargetPort: 8080}
	created := doJSON[portainer.PlatformGatewayRoute](t, ctx, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusCreated)
	require.Equal(t, deployment.ID, created.ServiceDeploymentID)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusConflict)
	payload.Path, payload.TargetPort = "/invalid", 9090
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusBadRequest)
}

func TestGatewayCertificateCreateStoresPrivateKeyOutsideAPIAndAudits(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Certificate project", Slug: "certificate-project"})
	certificatePEM, privateKeyPEM := handlerGatewayCertificatePEM(t, "api.example.test")

	created := doJSON[portainer.PlatformGatewayCertificate](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateway-certificates", project.ID), createGatewayCertificatePayload{
		Name:           "api-cert",
		CertificatePEM: string(certificatePEM),
		PrivateKeyPEM:  string(privateKeyPEM),
	}, http.StatusCreated)
	require.True(t, created.HasPrivateKey)
	require.Empty(t, created.MaterialRef)

	items := doJSON[[]portainer.PlatformGatewayCertificate](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/gateway-certificates", project.ID), nil, http.StatusOK)
	require.Len(t, items, 1)
	require.Empty(t, items[0].MaterialRef)

	persisted, err := ctx.handler.DataStore.PlatformGatewayCertificate().Read(created.ID)
	require.NoError(t, err)
	require.NotEmpty(t, persisted.MaterialRef)
	require.NotContains(t, persisted.MaterialRef, string(privateKeyPEM))

	audits, err := ctx.handler.DataStore.PlatformAuditLog().ReadAll()
	require.NoError(t, err)
	require.Contains(t, audits[len(audits)-1].Action, portainer.PlatformAuditActionGatewayCertificateCreated)
}

func TestGatewayRouteCreateRequiresAvailableCertificate(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "TLS route project", Slug: "tls-route-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	endpoint := &portainer.Endpoint{ID: 33, Name: "gateway", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	gateway := doJSON[portainer.PlatformGateway](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "prod-gateway"}, http.StatusCreated)
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "API", Slug: "api"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "API", Slug: "api"})
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/api:1"
	spec.Ports = []portainer.PlatformPortSpec{{Name: "http", ContainerPort: 8080, HostPort: 18080}}
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{EnvironmentID: environment.ID, DesiredSpec: &spec})

	certificatePEM, privateKeyPEM := handlerGatewayCertificatePEM(t, "api.example.test")
	certificate := doJSON[portainer.PlatformGatewayCertificate](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateway-certificates", project.ID), createGatewayCertificatePayload{Name: "api-cert", CertificatePEM: string(certificatePEM), PrivateKeyPEM: string(privateKeyPEM)}, http.StatusCreated)
	payload := createGatewayRoutePayload{ServiceDeploymentID: deployment.ID, Domain: "api.example.test", Path: "/", TargetPort: 8080, EnableTLS: true, CertificateID: certificate.ID}
	doJSON[portainer.PlatformGatewayRoute](t, ctx, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusCreated)

	payload.Domain = "not-covered.example.test"
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), payload, http.StatusBadRequest)
}

func TestGatewayRouteUpdateAndArchive(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Route update project", Slug: "route-update-project"})
	environment := createEnvironment(t, ctx, project.ID, createEnvironmentPayload{Name: "Prod", Slug: "prod", Type: portainer.PlatformEnvironmentTypeProd})
	endpoint := &portainer.Endpoint{ID: 34, Name: "gateway", Type: portainer.DockerEnvironment}
	require.NoError(t, ctx.handler.DataStore.Endpoint().Create(endpoint))
	gateway := doJSON[portainer.PlatformGateway](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateways", project.ID), createGatewayPayload{EnvironmentID: environment.ID, EndpointID: endpoint.ID, Name: "prod-gateway"}, http.StatusCreated)
	application := createApplication(t, ctx, project.ID, createApplicationPayload{Name: "API", Slug: "api"})
	service := createServiceDefinition(t, ctx, application.ID, createServiceDefinitionPayload{Name: "API", Slug: "api"})
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/api:1"
	spec.Ports = []portainer.PlatformPortSpec{{Name: "http", ContainerPort: 8080, HostPort: 18080}}
	deployment := createServiceDeployment(t, ctx, service.ID, createServiceDeploymentPayload{EnvironmentID: environment.ID, DesiredSpec: &spec})
	route := doJSON[portainer.PlatformGatewayRoute](t, ctx, http.MethodPost, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), createGatewayRoutePayload{ServiceDeploymentID: deployment.ID, Domain: "api.example.test", Path: "/", TargetPort: 8080}, http.StatusCreated)

	updated := doJSON[portainer.PlatformGatewayRoute](t, ctx, http.MethodPut, fmt.Sprintf("/platform/gateway-routes/%d", route.ID), updateGatewayRoutePayload{ResourceVersion: route.ResourceVersion, createGatewayRoutePayload: createGatewayRoutePayload{ServiceDeploymentID: deployment.ID, Domain: "api.example.test", Path: "/v1", TargetPort: 8080}}, http.StatusOK)
	require.Equal(t, "/v1", updated.Path)
	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/gateway-routes/%d", route.ID), nil, http.StatusNoContent)

	items := doJSON[[]portainer.PlatformGatewayRoute](t, ctx, http.MethodGet, fmt.Sprintf("/platform/gateways/%d/routes", gateway.ID), nil, http.StatusOK)
	require.Empty(t, items)
}

func TestGatewayCertificateArchiveRequiresNoActiveRoute(t *testing.T) {
	ctx := newPlatformTestContext(t)
	project := createProject(t, ctx, createProjectPayload{Name: "Certificate archive project", Slug: "certificate-archive-project"})
	certificatePEM, privateKeyPEM := handlerGatewayCertificatePEM(t, "api.example.test")
	certificate := doJSON[portainer.PlatformGatewayCertificate](t, ctx, http.MethodPost, fmt.Sprintf("/platform/projects/%d/gateway-certificates", project.ID), createGatewayCertificatePayload{Name: "api-cert", CertificatePEM: string(certificatePEM), PrivateKeyPEM: string(privateKeyPEM)}, http.StatusCreated)

	doRawJSON(t, ctx, ctx.adminJWT, http.MethodDelete, fmt.Sprintf("/platform/gateway-certificates/%d", certificate.ID), nil, http.StatusNoContent)
	items := doJSON[[]portainer.PlatformGatewayCertificate](t, ctx, http.MethodGet, fmt.Sprintf("/platform/projects/%d/gateway-certificates", project.ID), nil, http.StatusOK)
	require.Empty(t, items)
}

func handlerGatewayCertificatePEM(t *testing.T, domain string) ([]byte, []byte) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
}
