package platform

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/http/security"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
)

const (
	errPlatformInvalidRequest             = "PLATFORM_INVALID_REQUEST"
	errPlatformNotFound                   = "PLATFORM_NOT_FOUND"
	errPlatformValidationFailed           = "PLATFORM_VALIDATION_FAILED"
	errPlatformResourceVersionConflict    = "PLATFORM_RESOURCE_VERSION_CONFLICT"
	errPlatformReleaseConflict            = "PLATFORM_RELEASE_CONFLICT"
	errPlatformIdempotencyPayloadMismatch = "PLATFORM_IDEMPOTENCY_PAYLOAD_MISMATCH"
	errPlatformUnsupportedOperation       = "PLATFORM_UNSUPPORTED_OPERATION"
	errPlatformInternalError              = "PLATFORM_INTERNAL_ERROR"
)

func (handler *Handler) routeID(r *http.Request, name string) (int, *httperror.HandlerError) {
	id, err := request.RetrieveNumericRouteVariableValue(r, name)
	if err != nil {
		return 0, httperror.BadRequest(errPlatformInvalidRequest, err)
	}

	return id, nil
}

func (handler *Handler) convertError(err error) *httperror.HandlerError {
	if err == nil {
		return nil
	}

	var handlerErr *httperror.HandlerError
	if errors.As(err, &handlerErr) {
		return handlerErr
	}

	if handler.DataStore != nil && handler.DataStore.IsErrObjectNotFound(err) {
		return httperror.NotFound(errPlatformNotFound, err)
	}

	return httperror.InternalServerError(errPlatformInternalError, err)
}

func validationFailed(err error) *httperror.HandlerError {
	return httperror.BadRequest(errPlatformValidationFailed, err)
}

func notFoundError(message string) *httperror.HandlerError {
	return httperror.NotFound(errPlatformNotFound, errors.New(message))
}

func conflictError(message string) *httperror.HandlerError {
	return httperror.Conflict(errPlatformResourceVersionConflict, errors.New(message))
}

func duplicateError(message string) *httperror.HandlerError {
	return httperror.Conflict(errPlatformValidationFailed, errors.New(message))
}

func includeArchived(r *http.Request) bool {
	value := strings.ToLower(r.URL.Query().Get("includeArchived"))

	return value == "true" || value == "1"
}

func newLifecycle(now int64) portainer.PlatformLifecycle {
	lifecycle := portainer.NewPlatformLifecycle()
	lifecycle.CreatedAt = now
	lifecycle.UpdatedAt = now

	return lifecycle
}

func touchLifecycle(lifecycle *portainer.PlatformLifecycle, now int64) {
	lifecycle.ResourceVersion++
	lifecycle.UpdatedAt = now
}

func archiveLifecycle(lifecycle *portainer.PlatformLifecycle, now int64, userID portainer.UserID) {
	lifecycle.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	lifecycle.ArchivedAt = now
	lifecycle.ArchivedByUserID = userID
	touchLifecycle(lifecycle, now)
}

func isActive(lifecycle portainer.PlatformLifecycle) bool {
	return lifecycle.LifecycleStatus != portainer.PlatformLifecycleStatusArchived
}

func requireResourceVersion(current, expected int) error {
	if current != expected {
		return conflictError(fmt.Sprintf("ResourceVersion mismatch: current=%d expected=%d", current, expected))
	}

	return nil
}

func currentUserID(r *http.Request) (portainer.UserID, error) {
	securityContext, err := security.RetrieveRestrictedRequestContext(r)
	if err != nil {
		return 0, err
	}

	return securityContext.UserID, nil
}

func normalizeEnvironment(environment *portainer.PlatformEnvironment) {
	if environment.Type == "" {
		environment.Type = portainer.PlatformEnvironmentTypeCustom
	}
	if environment.Type == portainer.PlatformEnvironmentTypeProd {
		environment.IsProduction = true
	}
	if environment.TargetMode == "" {
		environment.TargetMode = portainer.PlatformTargetModeSingle
	}
	if environment.ReleasePolicy.Type == "" {
		environment.ReleasePolicy = portainer.NewPlatformReleasePolicy()
	}

	for i := range environment.Targets {
		if environment.Targets[i].Role == "" {
			environment.Targets[i].Role = portainer.PlatformDeploymentTargetRoleWorkload
		}
	}
}

func normalizeServiceDefinition(service *portainer.PlatformServiceDefinition) {
	if service.Type == "" {
		service.Type = portainer.PlatformServiceTypeBackend
	}
}

func readActiveProject(tx dataservices.DataStoreTx, id portainer.PlatformProjectID) (*portainer.PlatformProject, error) {
	project, err := tx.PlatformProject().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(project.PlatformLifecycle) {
		return nil, notFoundError("Project is archived")
	}

	return project, nil
}

func readActiveEnvironment(tx dataservices.DataStoreTx, id portainer.PlatformEnvironmentID) (*portainer.PlatformEnvironment, error) {
	environment, err := tx.PlatformEnvironment().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(environment.PlatformLifecycle) {
		return nil, notFoundError("Environment is archived")
	}

	return environment, nil
}

func readActiveApplication(tx dataservices.DataStoreTx, id portainer.PlatformApplicationID) (*portainer.PlatformApplication, error) {
	application, err := tx.PlatformApplication().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(application.PlatformLifecycle) {
		return nil, notFoundError("Application is archived")
	}

	return application, nil
}

func readActiveServiceDefinition(tx dataservices.DataStoreTx, id portainer.PlatformServiceDefinitionID) (*portainer.PlatformServiceDefinition, error) {
	service, err := tx.PlatformServiceDefinition().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(service.PlatformLifecycle) {
		return nil, notFoundError("Service definition is archived")
	}

	return service, nil
}

func readActiveServiceDeployment(tx dataservices.DataStoreTx, id portainer.PlatformServiceDeploymentID) (*portainer.PlatformServiceDeployment, error) {
	deployment, err := tx.PlatformServiceDeployment().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(deployment.PlatformLifecycle) {
		return nil, notFoundError("Service deployment is archived")
	}

	return deployment, nil
}
