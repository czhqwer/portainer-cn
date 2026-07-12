package platform

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJava8BuildContextUsesFixedTemplateAndRejectsShellArguments(t *testing.T) {
	context, err := NewJava8BuildContext(bytes.NewReader([]byte("jar")), JavaBuildOptions{JVMArgs: []string{"-Xmx512m"}, AppArgs: []string{"--server.port=8080"}, Port: 8080})
	require.NoError(t, err)
	reader := tar.NewReader(bytes.NewReader(context))
	files := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		files[header.Name] = data
	}
	require.Contains(t, string(files["Dockerfile"]), "FROM eclipse-temurin:8-jre")
	require.Contains(t, string(files["Dockerfile"]), "ENTRYPOINT [\"java\"")
	require.NotContains(t, string(files["Dockerfile"]), "sh -c")
	_, err = NewJava8BuildContext(bytes.NewReader([]byte("jar")), JavaBuildOptions{AppArgs: []string{";rm"}, Port: 8080})
	require.Error(t, err)
}
