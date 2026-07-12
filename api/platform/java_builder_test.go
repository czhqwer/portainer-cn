package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumeDockerBuildOutputUsesSafeReasonCodes(t *testing.T) {
	err := consumeDockerBuildOutput(strings.NewReader(`{"error":"pull access denied for private.example.test"}`))
	require.Equal(t, "BASE_IMAGE_UNAVAILABLE", ControlledBuildFailureReason(err))

	err = consumeDockerBuildOutput(strings.NewReader(`{"error":"a build command failed"}`))
	require.Equal(t, "CONTROLLED_BUILD_FAILED", ControlledBuildFailureReason(err))
	require.Equal(t, "BUILD_TIMEOUT", ControlledBuildFailureReason(context.DeadlineExceeded))
}
