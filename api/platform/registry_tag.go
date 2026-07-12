package platform

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

var (
	registryHostPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]{1,5})?$`)
	registryTagPart     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

// PlatformRegistryTag 固化候选镜像的唯一推送坐标。Artifact ID 进入 tag，避免重新构建或同版本制品覆盖其他 Artifact 的 tag。
func PlatformRegistryTag(registryURL, projectSlug, serviceSlug, version string, artifactID portainer.PlatformArtifactID) (string, error) {
	projectSlug = strings.ToLower(strings.TrimSpace(projectSlug))
	serviceSlug = strings.ToLower(strings.TrimSpace(serviceSlug))
	version = strings.ToLower(strings.TrimSpace(version))
	host, err := registryHost(registryURL)
	if err != nil || !registryTagPart.MatchString(projectSlug) || !registryTagPart.MatchString(serviceSlug) || !registryTagPart.MatchString(version) || artifactID <= 0 {
		return "", fmt.Errorf("registry tag is invalid")
	}
	return fmt.Sprintf("%s/%s/%s:%s-%d", host, projectSlug, serviceSlug, version, artifactID), nil
}

func registryHost(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", fmt.Errorf("registry URL is invalid")
		}
		value = parsed.Host
	}
	value = strings.TrimSuffix(value, "/")
	if !registryHostPattern.MatchString(value) {
		return "", fmt.Errorf("registry URL is invalid")
	}
	return value, nil
}
