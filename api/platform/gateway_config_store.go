package platform

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

const gatewayConfigFileName = "active.conf"

// GatewayConfigStore 将 Nginx 配置限制在数据目录下的网关专用根目录。
// ID 和 hash 均由平台模型生成，调用方不能传入相对路径，因此候选配置不会借由文件系统写到平台目录外。
type GatewayConfigStore struct {
	root string
}

func NewGatewayConfigStore(datastorePath string) (*GatewayConfigStore, error) {
	root := filepath.Join(datastorePath, "platform-gateways")
	if strings.TrimSpace(datastorePath) == "" {
		return nil, errors.New("gateway datastore path is required")
	}
	return &GatewayConfigStore{root: root}, nil
}

func (store *GatewayConfigStore) WriteCandidate(gatewayID portainer.PlatformGatewayID, config []byte) (string, error) {
	if store == nil || gatewayID <= 0 || len(config) == 0 {
		return "", errors.New("gateway candidate input is invalid")
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(config))
	path := store.versionPath(gatewayID, hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) != string(config) {
			return "", errors.New("gateway config hash collision")
		}
		return hash, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := writePrivateFile(path, config); err != nil {
		return "", err
	}
	return hash, nil
}

// Activate 在 Nginx 读取的固定 active.conf 上进行受控切换。
// 版本正文从不覆盖；活动文件始终由已落盘版本复制生成，便于 reload 失败后恢复到旧 hash。
func (store *GatewayConfigStore) Activate(gatewayID portainer.PlatformGatewayID, hash string) (string, error) {
	if store == nil || gatewayID <= 0 || !isGatewayConfigHash(hash) {
		return "", errors.New("gateway activation input is invalid")
	}
	versionPath := store.versionPath(gatewayID, hash)
	config, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}
	activePath := store.activePath(gatewayID)
	if err := os.MkdirAll(filepath.Dir(activePath), 0o700); err != nil {
		return "", err
	}
	previousHash := ""
	if previous, err := os.ReadFile(activePath); err == nil {
		previousHash = fmt.Sprintf("%x", sha256.Sum256(previous))
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := replacePrivateFile(activePath, config); err != nil {
		return "", err
	}
	return previousHash, nil
}

func (store *GatewayConfigStore) ActiveConfig(gatewayID portainer.PlatformGatewayID) ([]byte, string, error) {
	if store == nil || gatewayID <= 0 {
		return nil, "", errors.New("gateway ID is invalid")
	}
	config, err := os.ReadFile(store.activePath(gatewayID))
	if err != nil {
		return nil, "", err
	}
	return config, fmt.Sprintf("%x", sha256.Sum256(config)), nil
}

func (store *GatewayConfigStore) versionPath(gatewayID portainer.PlatformGatewayID, hash string) string {
	return filepath.Join(store.root, fmt.Sprintf("%d", gatewayID), "versions", hash+".conf")
}

func (store *GatewayConfigStore) activePath(gatewayID portainer.PlatformGatewayID) string {
	return filepath.Join(store.root, fmt.Sprintf("%d", gatewayID), gatewayConfigFileName)
}

func isGatewayConfigHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, character := range hash {
		if !(character >= 'a' && character <= 'f' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func writePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return err
	}
	return file.Sync()
}

func replacePrivateFile(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".candidate-*.conf")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}
	// Windows 不支持覆盖式 rename；Portainer 的生产网关运行在 Linux 容器主机，
	// 本分支仅为了本地单测兼容在此回退，真实网关演练必须验证 Linux 原子 rename 路径。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}
