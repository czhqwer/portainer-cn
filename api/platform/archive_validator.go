package platform

import (
	"archive/tar"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
)

const (
	archiveValidationMaxEntries int64 = 50_000
	archiveValidationMaxSize    int64 = 20 << 30
	archiveMetadataMaxSize      int64 = 4 << 20
)

type ArchiveFormat string

const (
	ArchiveFormatDocker ArchiveFormat = "docker-tar"
	ArchiveFormatOCI    ArchiveFormat = "oci-archive"
)

type ArchiveMetadata struct {
	Format       ArchiveFormat
	SourceRef    string
	Architecture string
	Created      string
}

// ValidateImageArchive 在调用 Docker 之前遍历完整 TAR。它拒绝会在解压时逃逸或耗尽资源的
// 条目，并确认 manifest、config 和所有 layer 已在同一 archive 中声明，避免把不可信归档直接交给 Docker。
func ValidateImageArchive(reader io.Reader, format ArchiveFormat) (ArchiveMetadata, error) {
	entries, contents, err := readSafeArchive(reader)
	if err != nil {
		return ArchiveMetadata{}, err
	}
	if format == ArchiveFormatDocker {
		return validateDockerArchive(entries, contents)
	}
	if format == ArchiveFormatOCI {
		return validateOCIArchive(entries, contents)
	}
	return ArchiveMetadata{}, errors.New("archive format is unsupported")
}

func readSafeArchive(reader io.Reader) (map[string]bool, map[string][]byte, error) {
	tarReader := tar.NewReader(reader)
	entries := map[string]bool{}
	contents := map[string][]byte{}
	var count, total int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, errors.New("archive cannot be read")
		}
		count++
		total += header.Size
		if count > archiveValidationMaxEntries || total > archiveValidationMaxSize || !safeArchiveHeader(header) || entries[header.Name] {
			return nil, nil, errors.New("archive is unsafe")
		}
		entries[header.Name] = true
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if header.Size <= archiveMetadataMaxSize {
				data, err := io.ReadAll(io.LimitReader(tarReader, archiveMetadataMaxSize+1))
				if err != nil || int64(len(data)) != header.Size {
					return nil, nil, errors.New("archive cannot be read")
				}
				contents[header.Name] = data
			} else if _, err := io.Copy(io.Discard, tarReader); err != nil {
				return nil, nil, errors.New("archive cannot be read")
			}
		}
	}
	return entries, contents, nil
}

func safeArchiveHeader(header *tar.Header) bool {
	if header == nil || header.Name == "" || header.Size < 0 || strings.HasPrefix(header.Name, "/") || strings.Contains(header.Name, "\\") || strings.HasPrefix(header.Name, "../") {
		return false
	}
	name := strings.TrimSuffix(header.Name, "/")
	if name == "" || path.Clean(name) != name || (header.Typeflag != tar.TypeDir && name != header.Name) {
		return false
	}
	return header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA || header.Typeflag == tar.TypeDir
}

type dockerManifestEntry struct {
	Config   string
	RepoTags []string
	Layers   []string
}

type imageConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Created      string `json:"created"`
}

func validateDockerArchive(entries map[string]bool, contents map[string][]byte) (ArchiveMetadata, error) {
	data, ok := contents["manifest.json"]
	if !ok {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	var manifest []dockerManifestEntry
	if json.Unmarshal(data, &manifest) != nil || len(manifest) != 1 || manifest[0].Config == "" || len(manifest[0].RepoTags) != 1 || manifest[0].RepoTags[0] == "" || len(manifest[0].Layers) == 0 || !entries[manifest[0].Config] || contents[manifest[0].Config] == nil {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	for _, layer := range manifest[0].Layers {
		if layer == "" || !entries[layer] {
			return ArchiveMetadata{}, errors.New("archive manifest is invalid")
		}
	}
	var config imageConfig
	if json.Unmarshal(contents[manifest[0].Config], &config) != nil || config.OS != "linux" || config.Architecture != "amd64" {
		return ArchiveMetadata{}, errors.New("archive architecture is unsupported")
	}
	return ArchiveMetadata{Format: ArchiveFormatDocker, SourceRef: manifest[0].RepoTags[0], Architecture: config.Architecture, Created: config.Created}, nil
}

func validateOCIArchive(entries map[string]bool, contents map[string][]byte) (ArchiveMetadata, error) {
	layout, ok := contents["oci-layout"]
	if !ok {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	var layoutDocument struct {
		ImageLayoutVersion string `json:"imageLayoutVersion"`
	}
	if json.Unmarshal(layout, &layoutDocument) != nil || layoutDocument.ImageLayoutVersion == "" {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	index, ok := contents["index.json"]
	if !ok {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	var document struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
			Annotations map[string]string `json:"annotations"`
		} `json:"manifests"`
	}
	if json.Unmarshal(index, &document) != nil || len(document.Manifests) != 1 || document.Manifests[0].Platform.OS != "linux" || document.Manifests[0].Platform.Architecture != "amd64" || !validSHA256Digest(document.Manifests[0].Digest) {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	manifestPath := "blobs/sha256/" + strings.TrimPrefix(document.Manifests[0].Digest, "sha256:")
	if !entries[manifestPath] || contents[manifestPath] == nil {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if json.Unmarshal(contents[manifestPath], &manifest) != nil || !validSHA256Digest(manifest.Config.Digest) || len(manifest.Layers) == 0 {
		return ArchiveMetadata{}, errors.New("archive manifest is invalid")
	}
	for _, digest := range append([]string{manifest.Config.Digest}, ociLayerDigests(manifest.Layers)...) {
		if !validSHA256Digest(digest) || !entries["blobs/sha256/"+strings.TrimPrefix(digest, "sha256:")] {
			return ArchiveMetadata{}, errors.New("archive manifest is invalid")
		}
	}
	return ArchiveMetadata{Format: ArchiveFormatOCI, SourceRef: document.Manifests[0].Annotations["org.opencontainers.image.ref.name"], Architecture: "amd64"}, nil
}

func ociLayerDigests(layers []struct {
	Digest string `json:"digest"`
}) []string {
	result := make([]string, len(layers))
	for i := range layers {
		result[i] = layers[i].Digest
	}
	return result
}
func validSHA256Digest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
