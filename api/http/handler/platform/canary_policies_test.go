package platform

import "testing"

import "github.com/stretchr/testify/require"

func TestAllowedCanaryTransitionOnlyAllowsFixedAdjacentSteps(t *testing.T) {
	require.True(t, allowedCanaryTransition(0, 5))
	require.True(t, allowedCanaryTransition(50, 0))
	require.False(t, allowedCanaryTransition(0, 25))
	require.False(t, allowedCanaryTransition(25, 100))
}
