package platform

import (
	"archive/tar"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateImageArchiveDockerRejectsUnsafeEntriesAndValidatesManifest(t *testing.T) {
	valid := dockerArchiveFixture(t, "linux", "amd64", false)
	metadata, err := ValidateImageArchive(bytes.NewReader(valid), ArchiveFormatDocker)
	require.NoError(t, err)
	require.Equal(t, "example/orders:1.0.0", metadata.SourceRef)

	unsafe := tarFixture(t, []tarFixtureEntry{{name: "../escape", body: []byte("x")}})
	_, err = ValidateImageArchive(bytes.NewReader(unsafe), ArchiveFormatDocker)
	require.Error(t, err)

	link := tarFixture(t, []tarFixtureEntry{{name: "manifest.json", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"}})
	_, err = ValidateImageArchive(bytes.NewReader(link), ArchiveFormatDocker)
	require.Error(t, err)

	missingLayer := dockerArchiveFixture(t, "linux", "amd64", true)
	_, err = ValidateImageArchive(bytes.NewReader(missingLayer), ArchiveFormatDocker)
	require.Error(t, err)
}

func TestValidateImageArchiveRejectsUnsupportedArchitectureAndInvalidOCI(t *testing.T) {
	unsupported := dockerArchiveFixture(t, "linux", "arm64", false)
	_, err := ValidateImageArchive(bytes.NewReader(unsupported), ArchiveFormatDocker)
	require.Error(t, err)

	invalidOCI := tarFixture(t, []tarFixtureEntry{{name: "oci-layout", body: []byte(`{"imageLayoutVersion":"1.0.0"}`)}, {name: "index.json", body: []byte(`{"manifests":[]}`)}})
	_, err = ValidateImageArchive(bytes.NewReader(invalidOCI), ArchiveFormatOCI)
	require.Error(t, err)
}

type tarFixtureEntry struct {
	name     string
	body     []byte
	typeflag byte
	linkname string
}

func tarFixture(t *testing.T, entries []tarFixtureEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		flag := entry.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: entry.name, Size: int64(len(entry.body)), Typeflag: flag, Linkname: entry.linkname}))
		if flag == tar.TypeReg {
			_, err := writer.Write(entry.body)
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func dockerArchiveFixture(t *testing.T, os, architecture string, missingLayer bool) []byte {
	t.Helper()
	entries := []tarFixtureEntry{{name: "manifest.json", body: []byte(`[{"Config":"config.json","RepoTags":["example/orders:1.0.0"],"Layers":["layer.tar"]}]`)}, {name: "config.json", body: []byte(`{"os":"` + os + `","architecture":"` + architecture + `","created":"2026-07-12T00:00:00Z"}`)}}
	if !missingLayer {
		entries = append(entries, tarFixtureEntry{name: "layer.tar", body: []byte("layer")})
	}
	return tarFixture(t, entries)
}
