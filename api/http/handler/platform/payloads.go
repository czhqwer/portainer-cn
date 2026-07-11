package platform

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	portainer "github.com/portainer/portainer/api"
)

type createProjectPayload struct {
	Name           string                                             `json:"Name"`
	Slug           string                                             `json:"Slug"`
	Description    string                                             `json:"Description,omitempty"`
	MemberPolicies map[portainer.UserID]portainer.PlatformProjectRole `json:"MemberPolicies,omitempty"`
	TeamPolicies   map[portainer.TeamID]portainer.PlatformProjectRole `json:"TeamPolicies,omitempty"`
}

func (payload *createProjectPayload) Validate(_ *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Slug = strings.TrimSpace(payload.Slug)
	payload.Description = strings.TrimSpace(payload.Description)

	return validateNameSlug(payload.Name, payload.Slug)
}

type updateProjectPayload struct {
	ResourceVersion int                                                `json:"ResourceVersion"`
	Name            *string                                            `json:"Name,omitempty"`
	Description     *string                                            `json:"Description,omitempty"`
	MemberPolicies  map[portainer.UserID]portainer.PlatformProjectRole `json:"MemberPolicies,omitempty"`
	TeamPolicies    map[portainer.TeamID]portainer.PlatformProjectRole `json:"TeamPolicies,omitempty"`
}

func (payload *updateProjectPayload) Validate(_ *http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		payload.Name = &name
		if name == "" {
			return errors.New("Name is required")
		}
	}
	if payload.Description != nil {
		description := strings.TrimSpace(*payload.Description)
		payload.Description = &description
	}

	return nil
}

type createEnvironmentPayload struct {
	Name              string                               `json:"Name"`
	Slug              string                               `json:"Slug"`
	Type              portainer.PlatformEnvironmentType    `json:"Type,omitempty"`
	IsProduction      bool                                 `json:"IsProduction,omitempty"`
	TargetMode        portainer.PlatformTargetMode         `json:"TargetMode,omitempty"`
	Targets           []portainer.PlatformDeploymentTarget `json:"Targets,omitempty"`
	DefaultRegistryID portainer.RegistryID                 `json:"DefaultRegistryId,omitempty"`
	HealthCheckHost   string                               `json:"HealthCheckHost,omitempty"`
	ReleasePolicy     portainer.PlatformReleasePolicy      `json:"ReleasePolicy,omitempty"`
}

func (payload *createEnvironmentPayload) Validate(_ *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Slug = strings.TrimSpace(payload.Slug)
	payload.HealthCheckHost = strings.TrimSpace(payload.HealthCheckHost)

	return validateNameSlug(payload.Name, payload.Slug)
}

type updateEnvironmentPayload struct {
	ResourceVersion   int                                   `json:"ResourceVersion"`
	Name              *string                               `json:"Name,omitempty"`
	Type              *portainer.PlatformEnvironmentType    `json:"Type,omitempty"`
	IsProduction      *bool                                 `json:"IsProduction,omitempty"`
	TargetMode        *portainer.PlatformTargetMode         `json:"TargetMode,omitempty"`
	Targets           *[]portainer.PlatformDeploymentTarget `json:"Targets,omitempty"`
	DefaultRegistryID *portainer.RegistryID                 `json:"DefaultRegistryId,omitempty"`
	HealthCheckHost   *string                               `json:"HealthCheckHost,omitempty"`
	ReleasePolicy     *portainer.PlatformReleasePolicy      `json:"ReleasePolicy,omitempty"`
}

func (payload *updateEnvironmentPayload) Validate(_ *http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		payload.Name = &name
		if name == "" {
			return errors.New("Name is required")
		}
	}
	if payload.HealthCheckHost != nil {
		healthCheckHost := strings.TrimSpace(*payload.HealthCheckHost)
		payload.HealthCheckHost = &healthCheckHost
	}

	return nil
}

type createApplicationPayload struct {
	Name         string             `json:"Name"`
	Slug         string             `json:"Slug"`
	Description  string             `json:"Description,omitempty"`
	OwnerUserIDs []portainer.UserID `json:"OwnerUserIds,omitempty"`
}

func (payload *createApplicationPayload) Validate(_ *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Slug = strings.TrimSpace(payload.Slug)
	payload.Description = strings.TrimSpace(payload.Description)

	return validateNameSlug(payload.Name, payload.Slug)
}

type updateApplicationPayload struct {
	ResourceVersion int                 `json:"ResourceVersion"`
	Name            *string             `json:"Name,omitempty"`
	Description     *string             `json:"Description,omitempty"`
	OwnerUserIDs    *[]portainer.UserID `json:"OwnerUserIds,omitempty"`
}

func (payload *updateApplicationPayload) Validate(_ *http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		payload.Name = &name
		if name == "" {
			return errors.New("Name is required")
		}
	}
	if payload.Description != nil {
		description := strings.TrimSpace(*payload.Description)
		payload.Description = &description
	}

	return nil
}

type createServiceDefinitionPayload struct {
	Name        string                        `json:"Name"`
	Slug        string                        `json:"Slug"`
	Type        portainer.PlatformServiceType `json:"Type,omitempty"`
	Description string                        `json:"Description,omitempty"`
}

func (payload *createServiceDefinitionPayload) Validate(_ *http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Slug = strings.TrimSpace(payload.Slug)
	payload.Description = strings.TrimSpace(payload.Description)

	return validateNameSlug(payload.Name, payload.Slug)
}

type updateServiceDefinitionPayload struct {
	ResourceVersion int                            `json:"ResourceVersion"`
	Name            *string                        `json:"Name,omitempty"`
	Type            *portainer.PlatformServiceType `json:"Type,omitempty"`
	Description     *string                        `json:"Description,omitempty"`
}

func (payload *updateServiceDefinitionPayload) Validate(_ *http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		payload.Name = &name
		if name == "" {
			return errors.New("Name is required")
		}
	}
	if payload.Description != nil {
		description := strings.TrimSpace(*payload.Description)
		payload.Description = &description
	}

	return nil
}

type createServiceDeploymentPayload struct {
	EnvironmentID portainer.PlatformEnvironmentID          `json:"EnvironmentId"`
	DesiredSpec   *portainer.PlatformDeploymentDesiredSpec `json:"DesiredSpec,omitempty"`
}

func (payload *createServiceDeploymentPayload) Validate(_ *http.Request) error {
	if payload.EnvironmentID == 0 {
		return errors.New("EnvironmentId is required")
	}
	if payload.DesiredSpec != nil {
		portainer.NormalizePlatformDeploymentDesiredSpec(payload.DesiredSpec)
		if err := portainer.ValidatePlatformDeploymentDesiredSpecV01(*payload.DesiredSpec); err != nil {
			return err
		}
	}

	return nil
}

type updateServiceDeploymentPayload struct {
	ResourceVersion int                                      `json:"ResourceVersion"`
	DesiredSpec     *portainer.PlatformDeploymentDesiredSpec `json:"DesiredSpec,omitempty"`
}

func (payload *updateServiceDeploymentPayload) Validate(_ *http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.DesiredSpec == nil {
		return errors.New("DesiredSpec is required")
	}
	if payload.DesiredSpec != nil {
		portainer.NormalizePlatformDeploymentDesiredSpec(payload.DesiredSpec)
		if err := portainer.ValidatePlatformDeploymentDesiredSpecV01(*payload.DesiredSpec); err != nil {
			return err
		}
	}

	return nil
}

func validateResourceVersion(resourceVersion int) error {
	if resourceVersion <= 0 {
		return errors.New("ResourceVersion is required")
	}

	return nil
}

func validateNameSlug(name, slug string) error {
	if name == "" {
		return errors.New("Name is required")
	}
	if len(name) > 64 {
		return errors.New("Name must be 64 characters or fewer")
	}
	if slug == "" {
		return errors.New("Slug is required")
	}
	if len(slug) > 64 {
		return errors.New("Slug must be 64 characters or fewer")
	}
	if slug[0] == '-' || slug[len(slug)-1] == '-' {
		return errors.New("Slug must not start or end with '-'")
	}

	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}

		return fmt.Errorf("Slug contains unsupported character %q", r)
	}

	return nil
}
