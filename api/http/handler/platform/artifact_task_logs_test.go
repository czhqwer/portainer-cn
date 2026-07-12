package platform

import (
	"fmt"
	"strings"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/stretchr/testify/require"
)

func TestArtifactTaskLogsRedactSensitiveValuesAndKeepNewestLines(t *testing.T) {
	artifact := portainer.PlatformArtifact{}

	for index := 0; index <= artifactTaskLogLimit; index++ {
		appendArtifactTaskLog(&artifact, fmt.Sprintf("Step %d/201", index))
	}
	appendArtifactTaskLog(&artifact, "pulling https://user:password@example.test/layer?X-Amz-Signature=signed-value token=secret-value Authorization: Bearer bearer-value from C:\\Users\\czh\\artifact.jar")

	logs := strings.Join(artifact.TaskLogs, "\n")
	require.Len(t, artifact.TaskLogs, artifactTaskLogLimit)
	require.NotContains(t, logs, "password")
	require.NotContains(t, logs, "signed-value")
	require.NotContains(t, logs, "secret-value")
	require.NotContains(t, logs, "bearer-value")
	require.NotContains(t, logs, "C:\\Users\\czh")
	require.Contains(t, logs, fmt.Sprintf("Step %d/201", artifactTaskLogLimit))
	require.NotContains(t, logs, "Step 0/201")
}

func TestArtifactTaskLogsResetForNewOperation(t *testing.T) {
	artifact := portainer.PlatformArtifact{TaskLogs: []string{"old build output"}}
	resetArtifactTaskLogs(&artifact)

	require.Empty(t, artifact.TaskLogs)
}
