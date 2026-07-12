package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestParseGatewayCertificateReturnsPublicMetadataOnly(t *testing.T) {
	certificatePEM, privateKeyPEM := testGatewayCertificatePEM(t, "api.example.test")

	metadata, err := ParseGatewayCertificate("api-cert", 7, certificatePEM, privateKeyPEM)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformProjectID(7), metadata.ProjectID)
	require.Equal(t, []string{"api.example.test"}, metadata.Domains)
	require.NotEmpty(t, metadata.SHA256)
	require.False(t, metadata.HasPrivateKey)
	require.Empty(t, metadata.MaterialRef)
	require.NoError(t, portainer.ValidatePlatformGatewayCertificate(metadata))
}

func TestGatewayCertificateStoreWritesPrivateMaterialOutsideDatastore(t *testing.T) {
	certificatePEM, privateKeyPEM := testGatewayCertificatePEM(t, "api.example.test")
	store, err := NewGatewayCertificateStore(t.TempDir())
	require.NoError(t, err)

	ref, err := store.Store(3, certificatePEM, privateKeyPEM)
	require.NoError(t, err)
	require.NotEmpty(t, ref)
	certificatePath, keyPath, err := store.Paths(3)
	require.NoError(t, err)
	require.FileExists(t, certificatePath)
	require.FileExists(t, keyPath)
	require.NoError(t, store.Remove(3))
	require.NoDirExists(t, ref)
}

func testGatewayCertificatePEM(t *testing.T, domain string) ([]byte, []byte) {
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
