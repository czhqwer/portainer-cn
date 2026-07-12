package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConsumeDockerBuildOutputUsesSafeReasonCodes(t *testing.T) {
	err := consumeDockerBuildOutput(strings.NewReader(`{"error":"pull access denied for private.example.test"}`), nil)
	require.Equal(t, "BASE_IMAGE_UNAVAILABLE", ControlledBuildFailureReason(err))

	err = consumeDockerBuildOutput(strings.NewReader(`{"error":"a build command failed"}`), nil)
	require.Equal(t, "CONTROLLED_BUILD_FAILED", ControlledBuildFailureReason(err))
	require.Equal(t, "BUILD_TIMEOUT", ControlledBuildFailureReason(context.DeadlineExceeded))
}

func TestConsumeDockerBuildOutputForwardsDockerStream(t *testing.T) {
	var lines []string
	err := consumeDockerBuildOutput(strings.NewReader(`{"stream":"Step 1/2 : FROM eclipse-temurin:8-jre\n"}`), func(line string) {
		lines = append(lines, strings.TrimSpace(line))
	})

	require.NoError(t, err)
	require.Equal(t, []string{"Step 1/2 : FROM eclipse-temurin:8-jre"}, lines)
}
