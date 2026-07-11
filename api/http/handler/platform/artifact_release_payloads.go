package platform

import (
	"errors"
	"net/http"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

type createImageReferenceArtifactPayload struct {
	ProjectID           portainer.PlatformProjectID           `json:"ProjectId"`
	ApplicationID       portainer.PlatformApplicationID       `json:"ApplicationId,omitempty"`
	ServiceDefinitionID portainer.PlatformServiceDefinitionID `json:"ServiceDefinitionId,omitempty"`
	Name                string                                `json:"Name"`
	Version             string                                `json:"Version"`
	ImageRef            string                                `json:"ImageRef"`
	ImageDigest         string                                `json:"ImageDigest,omitempty"`
	Traceability        portainer.PlatformTraceability        `json:"Traceability,omitempty"`
	RegistryID          portainer.RegistryID                  `json:"RegistryId,omitempty"`
	Retained            bool                                  `json:"Retained,omitempty"`
	Cleanable           bool                                  `json:"Cleanable,omitempty"`
}

func (payload *createImageReferenceArtifactPayload) Validate(_ *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Version = strings.TrimSpace(payload.Version)
	payload.ImageRef = strings.TrimSpace(payload.ImageRef)
	payload.ImageDigest = strings.TrimSpace(payload.ImageDigest)

	if payload.ProjectID == 0 {
		return errors.New("ProjectId is required")
	}
	if payload.Name == "" {
		return errors.New("Name is required")
	}
	if payload.Version == "" {
		return errors.New("Version is required")
	}
	if payload.ImageRef == "" {
		return errors.New("ImageRef is required")
	}

	return nil
}

type createReleasePayload struct {
	ProjectID            portainer.PlatformProjectID           `json:"ProjectId"`
	EnvironmentID        portainer.PlatformEnvironmentID       `json:"EnvironmentId"`
	ApplicationID        portainer.PlatformApplicationID       `json:"ApplicationId"`
	ServiceDefinitionID  portainer.PlatformServiceDefinitionID `json:"ServiceDefinitionId"`
	ServiceDeploymentID  portainer.PlatformServiceDeploymentID `json:"ServiceDeploymentId"`
	ArtifactID           portainer.PlatformArtifactID          `json:"ArtifactId"`
	Version              string                                `json:"Version"`
	ExpectedSpecRevision int                                   `json:"ExpectedSpecRevision"`
	Strategy             portainer.PlatformReleaseStrategy     `json:"Strategy"`
	TriggerType          portainer.PlatformReleaseTriggerType  `json:"TriggerType,omitempty"`
}

func (payload *createReleasePayload) Validate(_ *http.Request) error {
	payload.Version = strings.TrimSpace(payload.Version)

	if payload.ProjectID == 0 {
		return errors.New("ProjectId is required")
	}
	if payload.EnvironmentID == 0 {
		return errors.New("EnvironmentId is required")
	}
	if payload.ApplicationID == 0 {
		return errors.New("ApplicationId is required")
	}
	if payload.ServiceDefinitionID == 0 {
		return errors.New("ServiceDefinitionId is required")
	}
	if payload.ServiceDeploymentID == 0 {
		return errors.New("ServiceDeploymentId is required")
	}
	if payload.ArtifactID == 0 {
		return errors.New("ArtifactId is required")
	}
	if payload.Version == "" {
		return errors.New("Version is required")
	}
	if payload.ExpectedSpecRevision <= 0 {
		return errors.New("ExpectedSpecRevision is required")
	}
	if payload.Strategy.Type == "" {
		payload.Strategy = portainer.NewPlatformReleaseStrategy()
	}
	if payload.Strategy.Type != portainer.PlatformReleaseStrategyReplace {
		return errors.New("only replace strategy is supported in V0.1")
	}
	if payload.TriggerType == "" {
		payload.TriggerType = portainer.PlatformReleaseTriggerDeploy
	}

	return nil
}
