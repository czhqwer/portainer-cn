package platform

import (
	"strings"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/datastore"
	"github.com/segmentio/encoding/json"
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

func TestDockerEnvInjectsDatabaseBindingOnlyAtRuntime(t *testing.T) {
	_, store := datastore.MustNewTestStore(t, true, true)
	cipher, err := NewSecretCipher(store.Connection())
	require.NoError(t, err)
	password := "database-password"
	cipherText, hash, err := cipher.Encrypt(password)
	require.NoError(t, err)
	resource := portainer.NewPlatformDatabaseResource()
	resource.ProjectID = 1
	resource.EnvironmentID = 1
	resource.EndpointID = 1
	resource.Name = "orders-db"
	resource.Type = portainer.PlatformDatabaseTypePostgres
	resource.Host = "orders-db.internal"
	resource.Port = 5432
	resource.Database = "orders"
	resource.Username = "orders_app"
	resource.PasswordCipherText = cipherText
	resource.CredentialEncryptionVersion = portainer.PlatformDatabaseCredentialEncryptionVersion
	resource.CredentialHash = hash
	resource.HasPassword = true
	require.NoError(t, store.PlatformDatabaseResource().Create(&resource))

	release := portainer.PlatformRelease{ConfigSnapshot: portainer.PlatformServiceConfigSnapshot{
		EffectiveConfigSnapshot: portainer.PlatformEffectiveConfigSnapshot{Hash: "config-hash"},
		DatabaseBindings: []portainer.PlatformDatabaseBindingSnapshot{{
			BindingID: 1, BindingRevision: 1, DatabaseResourceID: resource.ID, ResourceRevision: resource.Revision, Type: resource.Type, VariableHash: DatabaseBindingVariableHash(resource),
		}},
	}}
	serialized, err := json.Marshal(release)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), password)
	require.NotContains(t, string(serialized), cipherText)

	driver := NewDockerRuntimeDriver(store, nil)
	env, err := driver.dockerEnv(ReleaseExecutionRequest{Release: release})
	require.NoError(t, err)
	require.Equal(t, []string{
		"DATABASE_HOST=orders-db.internal",
		"DATABASE_PORT=5432",
		"DATABASE_USER=orders_app",
		"DATABASE_PASSWORD=" + password,
		"DATABASE_NAME=orders",
		"DATABASE_URL=postgres://orders_app:database-password@orders-db.internal:5432/orders",
	}, env)

	resource.Host = "changed.internal"
	require.NoError(t, store.PlatformDatabaseResource().Update(resource.ID, &resource))
	_, err = driver.dockerEnv(ReleaseExecutionRequest{Release: release})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "Database binding snapshot is unavailable"))
}
