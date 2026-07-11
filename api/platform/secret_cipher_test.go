package platform

import (
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/datastore"
	"github.com/stretchr/testify/require"
)

func TestSecretCipherAndDockerEnvInjectEncryptedReleaseSnapshot(t *testing.T) {
	_, store := datastore.MustNewTestStore(t, true, true)
	cipher, err := NewSecretCipher(store.Connection())
	require.NoError(t, err)

	const value = "value-for-encryption-test"
	cipherText, hash, err := cipher.Encrypt(value)
	require.NoError(t, err)
	require.NotEqual(t, value, cipherText)
	decrypted, err := cipher.Decrypt(cipherText)
	require.NoError(t, err)
	require.Equal(t, value, decrypted)

	driver := NewDockerRuntimeDriver(store, nil)
	env, err := driver.dockerEnv(ReleaseExecutionRequest{
		Release: portainer.PlatformRelease{
			ConfigSnapshot: portainer.PlatformServiceConfigSnapshot{
				EffectiveConfigSnapshot: portainer.PlatformEffectiveConfigSnapshot{
					Hash: "config-hash",
					Entries: []portainer.PlatformConfigEntrySnapshot{
						{Key: "APP_ENV", ValueType: portainer.PlatformConfigValuePlain, Value: "test"},
					},
				},
				SecretSnapshots: []portainer.PlatformSecretSnapshot{{
					Name:              "API_TOKEN",
					CipherText:        cipherText,
					EncryptionVersion: PlatformSecretEncryptionVersion,
					Hash:              hash,
					HasValue:          true,
				}},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"APP_ENV=test", "API_TOKEN=" + value}, env)
}

func TestNewSecretCipherRejectsUnencryptedStore(t *testing.T) {
	_, store := datastore.MustNewTestStore(t, true, false)

	_, err := NewSecretCipher(store.Connection())
	require.Error(t, err)
}
