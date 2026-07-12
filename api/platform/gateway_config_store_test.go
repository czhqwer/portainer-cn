package platform

import (
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestGatewayConfigStoreKeepsVersionsAndRestoresActiveConfig(t *testing.T) {
	store, err := NewGatewayConfigStore(t.TempDir())
	require.NoError(t, err)
	first := []byte("server { listen 80; }")
	firstHash, err := store.WriteCandidate(1, first)
	require.NoError(t, err)
	previous, err := store.Activate(1, firstHash)
	require.NoError(t, err)
	require.Empty(t, previous)

	second := []byte("server { listen 8080; }")
	secondHash, err := store.WriteCandidate(1, second)
	require.NoError(t, err)
	previous, err = store.Activate(1, secondHash)
	require.NoError(t, err)
	require.Equal(t, firstHash, previous)

	_, activeHash, err := store.ActiveConfig(1)
	require.NoError(t, err)
	require.Equal(t, secondHash, activeHash)
	_, err = store.Activate(1, firstHash)
	require.NoError(t, err)
	active, activeHash, err := store.ActiveConfig(1)
	require.NoError(t, err)
	require.Equal(t, first, active)
	require.Equal(t, firstHash, activeHash)
}

func TestGatewayConfigStoreRejectsInvalidInputs(t *testing.T) {
	store, err := NewGatewayConfigStore(t.TempDir())
	require.NoError(t, err)
	_, err = store.WriteCandidate(0, []byte("server {}"))
	require.Error(t, err)
	_, err = store.Activate(portainer.PlatformGatewayID(1), "../escape")
	require.Error(t, err)
}
