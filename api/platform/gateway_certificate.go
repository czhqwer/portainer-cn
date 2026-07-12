package platform

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

// ParseGatewayCertificate 在任何文件写入前验证证书与私钥匹配，并仅生成可安全持久化的公开元数据。
// 私钥始终由调用方写入受控目录；本函数不会把 PEM 内容放入 PlatformGatewayCertificate。
func ParseGatewayCertificate(name string, projectID portainer.PlatformProjectID, certificatePEM, privateKeyPEM []byte) (portainer.PlatformGatewayCertificate, error) {
	if len(certificatePEM) == 0 || len(privateKeyPEM) == 0 {
		return portainer.PlatformGatewayCertificate{}, errors.New("certificate and private key are required")
	}
	pair, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return portainer.PlatformGatewayCertificate{}, errors.New("certificate and private key do not match")
	}
	if len(pair.Certificate) == 0 {
		return portainer.PlatformGatewayCertificate{}, errors.New("certificate chain is empty")
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return portainer.PlatformGatewayCertificate{}, errors.New("certificate is invalid")
	}
	if len(certificate.DNSNames) == 0 {
		return portainer.PlatformGatewayCertificate{}, errors.New("certificate must contain DNS names")
	}
	metadata := portainer.NewPlatformGatewayCertificate()
	metadata.ProjectID, metadata.Name = projectID, name
	metadata.Domains = append([]string(nil), certificate.DNSNames...)
	metadata.Serial = serialString(certificate.SerialNumber)
	metadata.NotBefore, metadata.NotAfter = certificate.NotBefore.Unix(), certificate.NotAfter.Unix()
	digest := sha256.Sum256(certificate.Raw)
	metadata.SHA256, metadata.HasPrivateKey = hex.EncodeToString(digest[:]), true
	portainer.NormalizePlatformGatewayCertificate(&metadata)
	if err := portainer.ValidatePlatformGatewayCertificate(metadata); err != nil {
		return portainer.PlatformGatewayCertificate{}, fmt.Errorf("certificate metadata is invalid: %w", err)
	}
	return metadata, nil
}

func serialString(serial *big.Int) string {
	if serial == nil {
		return ""
	}
	return strings.ToLower(serial.Text(16))
}
