package platform

import (
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestPlatformRegistryTagUsesUniqueControlledFormat(t *testing.T) {
	tag, err := PlatformRegistryTag("https://registry.example.com:5000/", "customer-a", "orders-api", "1.0.0", portainer.PlatformArtifactID(42))
	require.NoError(t, err)
	require.Equal(t, "registry.example.com:5000/customer-a/orders-api:1.0.0-42", tag)

	tag, err = PlatformRegistryTag("registry.example.com", "customer-a", "orders-api", "V1.0.0", portainer.PlatformArtifactID(43))
	require.NoError(t, err)
	require.Equal(t, "registry.example.com/customer-a/orders-api:v1.0.0-43", tag)

	_, err = PlatformRegistryTag("registry.example.com/team", "customer-a", "orders-api", "1.0.0", 42)
	require.Error(t, err)
	_, err = PlatformRegistryTag("registry.example.com", "Customer A", "orders-api", "1.0.0", 42)
	require.Error(t, err)
}
