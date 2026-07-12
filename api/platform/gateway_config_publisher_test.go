package platform

import (
	"context"
	"errors"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestGatewayConfigPublisherPreflightFailureKeepsActiveConfig(t *testing.T) {
	store := publisherStoreWithActiveConfig(t)
	runtime := &fakeGatewayRuntime{testErr: errors.New("test failed")}
	_, originalHash, err := store.ActiveConfig(1)
	require.NoError(t, err)
	runtime.originalHash = originalHash
	publisher, err := NewGatewayConfigPublisher(store, runtime)
	require.NoError(t, err)

	_, err = publisher.Publish(t.Context(), publisherGateway(), []byte("server { listen 8080; }"))
	require.EqualError(t, err, GatewayConfigFailurePreflight)
	_, activeHash, err := store.ActiveConfig(1)
	require.NoError(t, err)
	require.Equal(t, runtime.originalHash, activeHash)
	require.Zero(t, runtime.reloadCalls)
}

func TestGatewayConfigPublisherReloadFailureRestoresPreviousConfig(t *testing.T) {
	store := publisherStoreWithActiveConfig(t)
	runtime := &fakeGatewayRuntime{reloadErrors: []error{errors.New("reload failed"), nil}}
	_, originalHash, err := store.ActiveConfig(1)
	require.NoError(t, err)
	runtime.originalHash = originalHash
	publisher, err := NewGatewayConfigPublisher(store, runtime)
	require.NoError(t, err)

	result, err := publisher.Publish(t.Context(), publisherGateway(), []byte("server { listen 8080; }"))
	require.EqualError(t, err, GatewayConfigFailureReload)
	require.True(t, result.Recovered)
	_, activeHash, err := store.ActiveConfig(1)
	require.NoError(t, err)
	require.Equal(t, originalHash, activeHash)
	require.Equal(t, 2, runtime.reloadCalls)
}

func TestGatewayConfigPublisherReportsRecoveryFailure(t *testing.T) {
	store := publisherStoreWithActiveConfig(t)
	runtime := &fakeGatewayRuntime{reloadErrors: []error{errors.New("reload failed"), errors.New("recovery failed")}}
	publisher, err := NewGatewayConfigPublisher(store, runtime)
	require.NoError(t, err)

	_, err = publisher.Publish(t.Context(), publisherGateway(), []byte("server { listen 8080; }"))
	require.EqualError(t, err, GatewayConfigFailureRecovery)
}

func publisherStoreWithActiveConfig(t *testing.T) *GatewayConfigStore {
	t.Helper()
	store, err := NewGatewayConfigStore(t.TempDir())
	require.NoError(t, err)
	hash, err := store.WriteCandidate(1, []byte("server { listen 80; }"))
	require.NoError(t, err)
	_, err = store.Activate(1, hash)
	require.NoError(t, err)
	return store
}

func publisherGateway() portainer.PlatformGateway {
	return portainer.PlatformGateway{ID: 1, EndpointID: 1, ManagedContainerID: "gateway"}
}

type fakeGatewayRuntime struct {
	testErr      error
	reloadErrors []error
	reloadCalls  int
	originalHash string
}

func (runtime *fakeGatewayRuntime) Ensure(context.Context, portainer.PlatformGateway, string) (string, error) {
	return "gateway", nil
}

func (runtime *fakeGatewayRuntime) Test(context.Context, portainer.PlatformGateway, string) error {
	return runtime.testErr
}

func (runtime *fakeGatewayRuntime) Reload(context.Context, portainer.PlatformGateway) error {
	defer func() { runtime.reloadCalls++ }()
	if len(runtime.reloadErrors) == 0 {
		return nil
	}
	err := runtime.reloadErrors[0]
	runtime.reloadErrors = runtime.reloadErrors[1:]
	return err
}
