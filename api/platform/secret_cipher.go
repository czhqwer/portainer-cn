package platform

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/pkg/libcrypto"
)

const PlatformSecretEncryptionVersion = "boltdb-v1"

type platformSecretKeyProvider interface {
	PlatformSecretEncryptionKey() []byte
}

// SecretCipher reuses Portainer's active encrypted BoltDB key for field-level secrets.
// 数据库未启用加密时拒绝创建该对象，确保敏感值不会因为部署环境漏配 secret key 而退化为明文保存。
type SecretCipher struct {
	key []byte
}

func NewSecretCipher(connection portainer.Connection) (*SecretCipher, error) {
	if connection == nil || !connection.IsEncryptedStore() {
		return nil, errors.New("platform secret storage requires an encrypted database")
	}

	provider, ok := connection.(platformSecretKeyProvider)
	if !ok {
		return nil, errors.New("platform secret storage key is unavailable")
	}
	key := provider.PlatformSecretEncryptionKey()
	if len(key) == 0 {
		return nil, errors.New("platform secret storage key is unavailable")
	}

	return &SecretCipher{key: key}, nil
}

func (cipher *SecretCipher) Encrypt(value string) (cipherText string, hash string, err error) {
	if cipher == nil || len(cipher.key) == 0 {
		return "", "", errors.New("platform secret cipher is unavailable")
	}

	encrypted, err := libcrypto.Encrypt([]byte(value), cipher.key)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(value))
	return base64.RawStdEncoding.EncodeToString(encrypted), hex.EncodeToString(sum[:]), nil
}

func (cipher *SecretCipher) Decrypt(cipherText string) (string, error) {
	if cipher == nil || len(cipher.key) == 0 {
		return "", errors.New("platform secret cipher is unavailable")
	}

	encrypted, err := base64.RawStdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", errors.New("platform secret ciphertext is invalid")
	}
	plain, err := libcrypto.Decrypt(encrypted, cipher.key)
	if err != nil {
		return "", errors.New("platform secret ciphertext cannot be decrypted")
	}
	return string(plain), nil
}
