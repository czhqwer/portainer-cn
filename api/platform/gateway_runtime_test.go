package platform

import (
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestIsManagedGatewayContainer(t *testing.T) {
	require.True(t, isManagedGatewayContainer(map[string]string{
		platformLabelPrefix + ".managed": "true",
		platformGatewayLabel:             "7",
	}, portainer.PlatformGatewayID(7)))
	require.False(t, isManagedGatewayContainer(map[string]string{platformLabelPrefix + ".managed": "true"}, 7))
	require.False(t, isManagedGatewayContainer(map[string]string{platformGatewayLabel: "7"}, 7))
}
