package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	portainer "github.com/portainer/portainer/api"
)

// GatewayCertificateStore 将 PEM 材料放到平台数据目录，供受控 Nginx 只读挂载。
// 证书 ID 由 datastore 分配，调用方没有机会注入文件名或越过证书根目录。
type GatewayCertificateStore struct{ root string }

func NewGatewayCertificateStore(datastorePath string) (*GatewayCertificateStore, error) {
	if datastorePath == "" {
		return nil, errors.New("certificate datastore path is required")
	}
	return &GatewayCertificateStore{root: filepath.Join(datastorePath, "platform-gateways", "certificates")}, nil
}

func (store *GatewayCertificateStore) Store(certificateID portainer.PlatformGatewayCertificateID, certificatePEM, privateKeyPEM []byte) (string, error) {
	if store == nil || certificateID <= 0 || len(certificatePEM) == 0 || len(privateKeyPEM) == 0 {
		return "", errors.New("certificate store input is invalid")
	}
	directory := filepath.Join(store.root, fmt.Sprintf("%d", certificateID))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	certificatePath := filepath.Join(directory, "cert.pem")
	if err := replacePrivateFile(certificatePath, certificatePEM); err != nil {
		return "", err
	}
	if err := replacePrivateFile(filepath.Join(directory, "key.pem"), privateKeyPEM); err != nil {
		_ = os.Remove(certificatePath)
		return "", err
	}
	return directory, nil
}

func (store *GatewayCertificateStore) Paths(certificateID portainer.PlatformGatewayCertificateID) (string, string, error) {
	if store == nil || certificateID <= 0 {
		return "", "", errors.New("certificate ID is invalid")
	}
	directory := filepath.Join(store.root, fmt.Sprintf("%d", certificateID))
	return filepath.Join(directory, "cert.pem"), filepath.Join(directory, "key.pem"), nil
}
