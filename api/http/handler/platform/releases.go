package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"

	"github.com/segmentio/encoding/json"
)

const (
	releaseGate0BRequiredReason            = "GATE_0B_REQUIRED"
	releaseIdempotencyMismatchReason       = "IDEMPOTENCY_PAYLOAD_MISMATCH"
	releaseLockedReason                    = "RELEASE_LOCKED"
	idempotencyKeyHeader                   = "Idempotency-Key"
	idempotencyTTLSeconds            int64 = 24 * 60 * 60
)

type releaseCreateResponse struct {
	ReleaseID portainer.PlatformReleaseID     `json:"ReleaseId"`
	Status    portainer.PlatformReleaseStatus `json:"Status"`
	PollURL   string                          `json:"PollUrl"`
	Blocked   bool                            `json:"Blocked,omitempty"`
	Reason    string                          `json:"Reason,omitempty"`
}

type releaseValidateResponse struct {
	Valid                bool                                  `json:"Valid"`
	Executable           bool                                  `json:"Executable"`
	Code                 string                                `json:"Code,omitempty"`
	Reason               string                                `json:"Reason,omitempty"`
	Message              string                                `json:"Message,omitempty"`
	ServiceDeploymentID  portainer.PlatformServiceDeploymentID `json:"ServiceDeploymentId"`
	ArtifactID           portainer.PlatformArtifactID          `json:"ArtifactId"`
	ExpectedSpecRevision int                                   `json:"ExpectedSpecRevision"`
}

type platformCodedError struct {
	status  int
	code    string
	message string
	reason  string
	data    any
}

func (err *platformCodedError) Error() string {
	return err.message
}

func (handler *Handler) releaseList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	filters, handlerErr := releaseListFilters(r)
	if handlerErr != nil {
		return handlerErr
	}

	releases, err := handler.DataStore.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		if filters.projectID != 0 && release.ProjectID != filters.projectID {
			return false
		}
		if filters.environmentID != 0 && release.EnvironmentID != filters.environmentID {
			return false
		}
		if filters.applicationID != 0 && release.ApplicationID != filters.applicationID {
			return false
		}
		if filters.serviceDefinitionID != 0 && release.ServiceDefinitionID != filters.serviceDefinitionID {
			return false
		}
		if filters.serviceDeploymentID != 0 && release.ServiceDeploymentID != filters.serviceDeploymentID {
			return false
		}
		if filters.status != "" && release.Status != filters.status {
			return false
		}

		return true
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, releases)
}

func (handler *Handler) releaseInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}

	release, err := handler.DataStore.PlatformRelease().Read(portainer.PlatformReleaseID(id))
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, release)
}

func (handler *Handler) releaseValidate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload createReleasePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	err := handler.DataStore.ViewTx(func(tx dataservices.DataStoreTx) error {
		_, _, err := validateReleaseReferences(tx, payload)
		return err
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, releaseValidateResponse{
		Valid:                true,
		Executable:           false,
		Code:                 errPlatformUnsupportedOperation,
		Reason:               releaseGate0BRequiredReason,
		Message:              "Gate 0B Docker/Agent Spike is required before release execution.",
		ServiceDeploymentID:  payload.ServiceDeploymentID,
		ArtifactID:           payload.ArtifactID,
		ExpectedSpecRevision: payload.ExpectedSpecRevision,
	})
}

func (handler *Handler) releaseCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload createReleasePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if idempotencyKey == "" {
		return validationFailed(errors.New("Idempotency-Key header is required"))
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	payloadHash, err := releasePayloadHash(payload)
	if err != nil {
		return handler.convertError(err)
	}
	idempotencyKeyHash := releaseIdempotencyHash(userID, payload.ServiceDeploymentID, idempotencyKey)
	now := time.Now().Unix()
	var release *portainer.PlatformRelease

	// Gate 0B 前仍写入失败 Release 事实，原因是幂等键需要有稳定返回值；
	// 但不会创建 ReleaseLock、不会入队 worker，也不会触发任何 Docker 操作。
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existing, err := findIdempotentRelease(tx, payload.ServiceDeploymentID, userID, idempotencyKeyHash, now)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.PayloadHash != payloadHash {
				return &platformCodedError{
					status:  http.StatusConflict,
					code:    errPlatformIdempotencyPayloadMismatch,
					message: "Idempotency-Key was already used with a different payload.",
					reason:  releaseIdempotencyMismatchReason,
				}
			}
			release = existing
			return nil
		}

		lockID := platformreleaselock.LockIDForServiceDeployment(payload.ServiceDeploymentID)
		lock, err := tx.PlatformReleaseLock().Read(lockID)
		if err != nil && !tx.IsErrObjectNotFound(err) {
			return err
		}
		if lock != nil {
			lockedRelease, readErr := tx.PlatformRelease().Read(lock.ReleaseID)
			if readErr != nil && !tx.IsErrObjectNotFound(readErr) {
				return readErr
			}
			if lock.IdempotencyKeyHash == idempotencyKeyHash {
				if lock.PayloadHash != payloadHash {
					return &platformCodedError{
						status:  http.StatusConflict,
						code:    errPlatformIdempotencyPayloadMismatch,
						message: "Idempotency-Key was already used with a different payload.",
						reason:  releaseIdempotencyMismatchReason,
					}
				}
				release = lockedRelease
				return nil
			}
			if lockedRelease != nil && !releaseStatusReleasesLock(lockedRelease.Status) {
				return &platformCodedError{
					status:  http.StatusConflict,
					code:    errPlatformReleaseConflict,
					message: "ServiceDeployment already has an active release.",
					reason:  releaseLockedReason,
					data:    releaseResponse(lockedRelease),
				}
			}
			if lockedRelease == nil || releaseStatusReleasesLock(lockedRelease.Status) {
				if err := tx.PlatformReleaseLock().Delete(lockID); err != nil {
					return err
				}
			}
		}

		deployment, artifact, err := validateReleaseReferences(tx, payload)
		if err != nil {
			return err
		}

		release = newGate0BBlockedRelease(payload, deployment, artifact, userID, idempotencyKeyHash, payloadHash, now)

		return tx.PlatformRelease().Create(release)
	})
	if err != nil {
		var codedErr *platformCodedError
		if errors.As(err, &codedErr) {
			return writePlatformError(w, codedErr.status, codedErr.code, codedErr.message, codedErr.reason, codedErr.data)
		}

		return handler.convertError(err)
	}

	createResponse := releaseResponse(release)
	if release.FailureReason == releaseGate0BRequiredReason {
		createResponse.Blocked = true
		createResponse.Reason = releaseGate0BRequiredReason
		return writePlatformError(
			w,
			http.StatusBadRequest,
			errPlatformUnsupportedOperation,
			"Gate 0B Docker/Agent Spike is required before release execution.",
			releaseGate0BRequiredReason,
			createResponse,
		)
	}

	return response.JSONWithStatus(w, createResponse, http.StatusAccepted)
}

func validateReleaseReferences(tx dataservices.DataStoreTx, payload createReleasePayload) (*portainer.PlatformServiceDeployment, *portainer.PlatformArtifact, error) {
	if _, err := readActiveProject(tx, payload.ProjectID); err != nil {
		return nil, nil, err
	}
	environment, err := readActiveEnvironment(tx, payload.EnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	application, err := readActiveApplication(tx, payload.ApplicationID)
	if err != nil {
		return nil, nil, err
	}
	service, err := readActiveServiceDefinition(tx, payload.ServiceDefinitionID)
	if err != nil {
		return nil, nil, err
	}
	deployment, err := readActiveServiceDeployment(tx, payload.ServiceDeploymentID)
	if err != nil {
		return nil, nil, err
	}
	artifact, err := tx.PlatformArtifact().Read(payload.ArtifactID)
	if err != nil {
		return nil, nil, err
	}
	if !isActive(artifact.PlatformLifecycle) {
		return nil, nil, notFoundError("Artifact is archived")
	}

	if environment.ProjectID != payload.ProjectID ||
		application.ProjectID != payload.ProjectID ||
		service.ProjectID != payload.ProjectID ||
		deployment.ProjectID != payload.ProjectID ||
		artifact.ProjectID != payload.ProjectID {
		return nil, nil, validationFailedError("Release resources belong to different projects")
	}
	if service.ApplicationID != payload.ApplicationID ||
		deployment.ApplicationID != payload.ApplicationID {
		return nil, nil, validationFailedError("Release resources belong to different applications")
	}
	if deployment.ServiceDefinitionID != payload.ServiceDefinitionID {
		return nil, nil, validationFailedError("ServiceDeployment does not belong to ServiceDefinition")
	}
	if deployment.EnvironmentID != payload.EnvironmentID {
		return nil, nil, validationFailedError("ServiceDeployment does not belong to Environment")
	}
	if artifact.ApplicationID != 0 && artifact.ApplicationID != payload.ApplicationID {
		return nil, nil, validationFailedError("Artifact does not belong to Application")
	}
	if artifact.ServiceDefinitionID != 0 && artifact.ServiceDefinitionID != payload.ServiceDefinitionID {
		return nil, nil, validationFailedError("Artifact does not belong to ServiceDefinition")
	}
	if artifact.Type != portainer.PlatformArtifactTypeImage || artifact.SourceType != portainer.PlatformArtifactSourceImageReference || artifact.ImageRef == "" {
		return nil, nil, validationFailedError("Only image-reference artifacts are supported in V0.1")
	}
	if deployment.SpecRevision != payload.ExpectedSpecRevision {
		return nil, nil, validationFailedError("ExpectedSpecRevision does not match current deployment SpecRevision")
	}
	if err := portainer.ValidatePlatformDeploymentDesiredSpecV01(deployment.DesiredSpec); err != nil {
		return nil, nil, err
	}

	return deployment, artifact, nil
}

func findIdempotentRelease(tx dataservices.DataStoreTx, serviceDeploymentID portainer.PlatformServiceDeploymentID, userID portainer.UserID, idempotencyKeyHash string, now int64) (*portainer.PlatformRelease, error) {
	releases, err := tx.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		return release.ServiceDeploymentID == serviceDeploymentID &&
			release.OperatorUserID == userID &&
			release.IdempotencyKeyHash == idempotencyKeyHash &&
			now-release.CreatedAt <= idempotencyTTLSeconds
	})
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, nil
	}

	return &releases[0], nil
}

func newGate0BBlockedRelease(payload createReleasePayload, deployment *portainer.PlatformServiceDeployment, artifact *portainer.PlatformArtifact, userID portainer.UserID, idempotencyKeyHash string, payloadHash string, now int64) *portainer.PlatformRelease {
	return &portainer.PlatformRelease{
		ProjectID:            payload.ProjectID,
		EnvironmentID:        payload.EnvironmentID,
		ApplicationID:        payload.ApplicationID,
		ServiceDefinitionID:  payload.ServiceDefinitionID,
		ServiceDeploymentID:  payload.ServiceDeploymentID,
		ArtifactID:           payload.ArtifactID,
		Version:              payload.Version,
		TriggerType:          payload.TriggerType,
		Strategy:             payload.Strategy,
		Status:               portainer.PlatformReleaseStatusFailed,
		OperatorUserID:       userID,
		IdempotencyKeyHash:   idempotencyKeyHash,
		PayloadHash:          payloadHash,
		ExpectedSpecRevision: payload.ExpectedSpecRevision,
		Image:                artifact.ImageRef,
		ImageDigest:          artifact.ImageDigest,
		Traceability:         artifact.Traceability,
		ArtifactSnapshot:     artifactSnapshotFromArtifact(artifact),
		ConfigSnapshot: portainer.PlatformServiceConfigSnapshot{
			SpecRevision:        deployment.SpecRevision,
			DesiredSpecSnapshot: deployment.DesiredSpec,
		},
		TargetSnapshot:    targetSnapshotFromDeployment(deployment),
		HealthCheckResult: portainer.PlatformHealthCheckResult{Status: portainer.PlatformHealthCheckStatusSkipped, ErrorMessage: releaseGate0BRequiredReason},
		Steps:             gate0BBlockedSteps(now),
		FailureReason:     releaseGate0BRequiredReason,
		ResolutionAction:  portainer.PlatformReleaseResolutionNone,
		StartedAt:         now,
		FinishedAt:        now,
		CreatedAt:         now,
		QueueExpiresAt:    now + 600,
	}
}

func artifactSnapshotFromArtifact(artifact *portainer.PlatformArtifact) portainer.PlatformArtifactSnapshot {
	return portainer.PlatformArtifactSnapshot{
		ArtifactID:   artifact.ID,
		Name:         artifact.Name,
		Version:      artifact.Version,
		Type:         artifact.Type,
		SourceType:   artifact.SourceType,
		ImageRef:     artifact.ImageRef,
		ImageDigest:  artifact.ImageDigest,
		Traceability: artifact.Traceability,
		RegistryID:   artifact.RegistryID,
		SHA256:       artifact.SHA256,
		Size:         artifact.Size,
		Retained:     artifact.Retained,
	}
}

func targetSnapshotFromDeployment(deployment *portainer.PlatformServiceDeployment) portainer.PlatformTargetSnapshot {
	return portainer.PlatformTargetSnapshot{
		RuntimeDriver: deployment.DesiredSpec.Runtime.RuntimeDriver,
		ExecutorMode:  portainer.PlatformExecutorModeSingle,
	}
}

func gate0BBlockedSteps(now int64) []portainer.PlatformReleaseStep {
	return []portainer.PlatformReleaseStep{
		{
			Name:       "gate-0b-check",
			Status:     portainer.PlatformReleaseStepStatusFailed,
			Reason:     releaseGate0BRequiredReason,
			Message:    "Docker release executor is blocked until Gate 0B passes.",
			StartedAt:  now,
			FinishedAt: now,
		},
	}
}

func releasePayloadHash(payload createReleasePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return hashString(string(data)), nil
}

func releaseIdempotencyHash(userID portainer.UserID, serviceDeploymentID portainer.PlatformServiceDeploymentID, key string) string {
	return hashString(fmt.Sprintf("%d:%d:%s", userID, serviceDeploymentID, key))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:])
}

func releaseResponse(release *portainer.PlatformRelease) releaseCreateResponse {
	if release == nil {
		return releaseCreateResponse{}
	}

	return releaseCreateResponse{
		ReleaseID: release.ID,
		Status:    release.Status,
		PollURL:   fmt.Sprintf("/api/platform/releases/%d", release.ID),
	}
}

type releaseFilters struct {
	projectID           portainer.PlatformProjectID
	environmentID       portainer.PlatformEnvironmentID
	applicationID       portainer.PlatformApplicationID
	serviceDefinitionID portainer.PlatformServiceDefinitionID
	serviceDeploymentID portainer.PlatformServiceDeploymentID
	status              portainer.PlatformReleaseStatus
}

func releaseListFilters(r *http.Request) (releaseFilters, *httperror.HandlerError) {
	projectID, err := optionalQueryID(r, "projectId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	environmentID, err := optionalQueryID(r, "environmentId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	applicationID, err := optionalQueryID(r, "applicationId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDefinitionID, err := optionalQueryID(r, "serviceDefinitionId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}
	serviceDeploymentID, err := optionalQueryID(r, "serviceDeploymentId")
	if err != nil {
		return releaseFilters{}, httperror.BadRequest(errPlatformInvalidRequest, err)
	}

	return releaseFilters{
		projectID:           portainer.PlatformProjectID(projectID),
		environmentID:       portainer.PlatformEnvironmentID(environmentID),
		applicationID:       portainer.PlatformApplicationID(applicationID),
		serviceDefinitionID: portainer.PlatformServiceDefinitionID(serviceDefinitionID),
		serviceDeploymentID: portainer.PlatformServiceDeploymentID(serviceDeploymentID),
		status:              portainer.PlatformReleaseStatus(r.URL.Query().Get("status")),
	}, nil
}
