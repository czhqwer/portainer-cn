package platform

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStaticDistBuildContextDefinesPredictableSPAMPAConfigs(t *testing.T) {
	file := writeDistZip(t, map[string]string{
		"index.html":      "<html></html>",
		"assets/app.js":   "console.log('ok')",
		"errors/404.html": "not found",
	})

	spa, err := NewStaticDistBuildContext(file, DistBuildOptions{Mode: DistModeSPA, CachePolicy: DistCachePolicyImmutable, NotFoundPage: "errors/404.html"})
	require.NoError(t, err)
	spaFiles := readBuildContext(t, spa)
	require.Contains(t, string(spaFiles["Dockerfile"]), "FROM nginx:1.27-alpine")
	require.Contains(t, string(spaFiles["default.conf"]), "try_files $uri $uri/ /index.html;")
	require.Contains(t, string(spaFiles["default.conf"]), "max-age=31536000, immutable")
	require.Contains(t, string(spaFiles["default.conf"]), "error_page 404 /errors/404.html;")
	require.Equal(t, "static-nginx-v1:spa:immutable:custom404", StaticBuildTemplateName(DistBuildOptions{Mode: DistModeSPA, CachePolicy: DistCachePolicyImmutable, NotFoundPage: "errors/404.html"}))

	mpa, err := NewStaticDistBuildContext(file, DistBuildOptions{Mode: DistModeMPA, CachePolicy: DistCachePolicyNoCache})
	require.NoError(t, err)
	mpaFiles := readBuildContext(t, mpa)
	require.Contains(t, string(mpaFiles["default.conf"]), "try_files $uri $uri/ =404;")
	require.NotContains(t, string(mpaFiles["default.conf"]), "/index.html;")
	require.Contains(t, string(mpaFiles["default.conf"]), "Cache-Control \"no-store\"")
}

func TestStaticDistBuildContextRejectsUnsafeAndIncompleteZIP(t *testing.T) {
	missingIndex := writeDistZip(t, map[string]string{"about.html": "about"})
	_, err := NewStaticDistBuildContext(missingIndex, DistBuildOptions{Mode: DistModeSPA})
	require.ErrorIs(t, err, errDistEntryMissing)

	zipSlip := writeDistZip(t, map[string]string{"../escape": "x"})
	_, err = NewStaticDistBuildContext(zipSlip, DistBuildOptions{Mode: DistModeMPA})
	require.ErrorIs(t, err, errDistArchiveUnsafe)

	link := filepath.Join(t.TempDir(), "link.zip")
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	header := &zip.FileHeader{Name: "index.html"}
	header.SetMode(os.ModeSymlink | 0o777)
	entry, err := writer.CreateHeader(header)
	require.NoError(t, err)
	_, err = entry.Write([]byte("target"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(link, archive.Bytes(), 0o600))
	_, err = NewStaticDistBuildContext(link, DistBuildOptions{Mode: DistModeSPA})
	require.ErrorIs(t, err, errDistArchiveUnsafe)

	invalid404 := writeDistZip(t, map[string]string{"index.html": "ok"})
	_, err = NewStaticDistBuildContext(invalid404, DistBuildOptions{Mode: DistModeSPA, NotFoundPage: "missing.html"})
	require.ErrorIs(t, err, errDistEntryMissing)
	require.Equal(t, "ARCHIVE_UNSAFE", DistBuildFailureReason(errDistArchiveUnsafe))
	require.Equal(t, "DIST_ENTRY_MISSING", DistBuildFailureReason(errDistEntryMissing))
}

func TestStaticDistBuildContextFileCleansFailedContext(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "contexts")
	file := writeDistZip(t, map[string]string{"missing.html": "missing"})
	contextFile, err := NewStaticDistBuildContextFile(file, directory, DistBuildOptions{Mode: DistModeSPA})
	require.Nil(t, contextFile)
	require.ErrorIs(t, err, errDistEntryMissing)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func writeDistZip(t *testing.T, files map[string]string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "dist.zip")
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(file, archive.Bytes(), 0o600))
	return file
}

func readBuildContext(t *testing.T, context []byte) map[string][]byte {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(context))
	files := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		files[header.Name] = content
	}
	return files
}
