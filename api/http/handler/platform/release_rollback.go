package platform

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type rollbackReleasePayload struct {
	ConfirmProduction bool `json:"ConfirmProduction"`
}

func (payload *rollbackReleasePayload) Validate(_ *http.Request) error {
	return nil
}

// rollbackDiffResponse deliberately only returns resource identities, change flags and secret
// presence. Historical configuration snapshots can contain values and ciphertext, neither of
// which should cross the release API boundary just to render a confirmation dialog.
type rollbackDiffResponse struct {
	SourceReleaseID         portainer.PlatformReleaseID `json:"SourceReleaseId"`
	CurrentReleaseID        portainer.PlatformReleaseID `json:"CurrentReleaseId,omitempty"`
	SourceImage             string                      `json:"SourceImage"`
	CurrentImage            string                      `json:"CurrentImage,omitempty"`
	ImageChanged            bool                        `json:"ImageChanged"`
	ConfigChanged           bool                        `json:"ConfigChanged"`
	PortsChanged            bool                        `json:"PortsChanged"`
	EnvironmentChanged      bool                        `json:"EnvironmentChanged"`
	ChangedConfigKeys       []string                    `json:"ChangedConfigKeys,omitempty"`
	ChangedEnvironmentNames []string                    `json:"ChangedEnvironmentNames,omitempty"`
	SensitiveVariables      []rollbackSensitiveVariable `json:"SensitiveVariables,omitempty"`
	Production              bool                        `json:"Production"`
}

type rollbackSensitiveVariable struct {
	Name            string `json:"Name"`
	SourceHasValue  bool   `json:"SourceHasValue"`
	CurrentHasValue bool   `json:"CurrentHasValue"`
	Changed         bool   `json:"Changed"`
}

func (handler *Handler) releaseRollbackDiff(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}

	source, handlerErr := handler.requireReleasePermission(r, portainer.PlatformReleaseID(id), platformPermissionArtifactRelease)
	if handlerErr != nil {
		return handlerErr
	}
	if handlerErr := handler.requireReleaseRuntimePermission(r, source); handlerErr != nil {
		return handlerErr
	}

	deployment, err := handler.DataStore.PlatformServiceDeployment().Read(source.ServiceDeploymentID)
	if err != nil {
		return handler.convertError(err)
	}
	environment, err := handler.DataStore.PlatformEnvironment().Read(source.EnvironmentID)
	if err != nil {
		return handler.convertError(err)
	}

	var current *portainer.PlatformRelease
	if deployment.CurrentServingReleaseID != 0 {
		current, err = handler.DataStore.PlatformRelease().Read(deployment.CurrentServingReleaseID)
		if err != nil && !handler.DataStore.IsErrObjectNotFound(err) {
			return handler.convertError(err)
		}
	}

	return response.JSON(w, buildRollbackDiff(*source, current, environment.IsProduction))
}

func (handler *Handler) releaseRollback(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}

	var payload rollbackReleasePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	source, handlerErr := handler.requireReleasePermission(r, portainer.PlatformReleaseID(id), platformPermissionArtifactRelease)
	if handlerErr != nil {
		return handlerErr
	}
	if handlerErr := handler.requireReleaseRuntimePermission(r, source); handlerErr != nil {
		return handlerErr
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if idempotencyKey == "" {
		return validationFailed(errors.New("Idempotency-Key header is required"))
	}
	if handler.ReleaseExecutor == nil {
		return writePlatformError(
			w,
			http.StatusServiceUnavailable,
			errPlatformUnsupportedOperation,
			"Docker release executor is not configured.",
			platformservice.ReleaseFailureReasonExecutorUnavailable,
			nil,
		)
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	now := time.Now().Unix()
	var release *portainer.PlatformRelease
	var refs *releaseReferenceSet
	shouldExecute := false

	// 历史回滚必须以历史成功 Release 的不可变快照创建一条新的事实记录；这里不复用
	// 当前部署的配置，也不更新来源 Release，避免回滚操作改写已发生的发布历史。
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		sourceRelease, err := tx.PlatformRelease().Read(portainer.PlatformReleaseID(id))
		if err != nil {
			return err
		}
		payloadHash := rollbackPayloadHash(*sourceRelease, payload)
		idempotencyKeyHash := releaseIdempotencyHash(userID, sourceRelease.ServiceDeploymentID, idempotencyKey)

		existing, err := findIdempotentRelease(tx, sourceRelease.ServiceDeploymentID, userID, idempotencyKeyHash, now)
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
		rollbackRefs, err := handler.validateRollbackReferences(tx, sourceRelease, payload)
		if err != nil {
			return err
		}

		if err := ensureRollbackReleaseLock(tx, sourceRelease.ServiceDeploymentID); err != nil {
			return err
		}

		release = newRollbackRelease(sourceRelease, rollbackRefs.deployment, rollbackRefs.artifact, userID, idempotencyKeyHash, payloadHash, now)
		if err := tx.PlatformRelease().Create(release); err != nil {
			return err
		}
		lock := &portainer.PlatformReleaseLock{
			ServiceDeploymentID: sourceRelease.ServiceDeploymentID,
			ReleaseID:           release.ID,
			IdempotencyKeyHash:  idempotencyKeyHash,
			PayloadHash:         payloadHash,
			LeaseOwner:          "platform-release-api",
			LeaseExpiresAt:      now + 15*60,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := tx.PlatformReleaseLock().Create(lock); err != nil {
			return err
		}
		if err := handler.createReleaseAuditLog(tx, r, portainer.PlatformAuditActionRollbackCreated, portainer.PlatformAuditResultSuccess, *release, releaseAuditSummary(*sourceRelease), releaseAuditSummary(*release), ""); err != nil {
			return err
		}

		refs = rollbackRefs
		shouldExecute = true
		return nil
	})
	if err != nil {
		var codedErr *platformCodedError
		if errors.As(err, &codedErr) {
			return writePlatformError(w, codedErr.status, codedErr.code, codedErr.message, codedErr.reason, codedErr.data)
		}
		return handler.convertError(err)
	}

	if shouldExecute {
		result, err := handler.ReleaseExecutor.Execute(r.Context(), platformservice.ReleaseExecutionRequest{
			Project:           *refs.project,
			Environment:       *refs.environment,
			Application:       *refs.application,
			ServiceDefinition: *refs.service,
			Deployment:        *refs.deployment,
			Artifact:          *refs.artifact,
			Release:           *release,
		})
		if err != nil {
			return handler.convertError(err)
		}
		if err := handler.persistReleaseExecutionResult(r, result); err != nil {
			return handler.convertError(err)
		}
		release = &result.Release
	}

	return response.JSONWithStatus(w, releaseResponse(release), http.StatusAccepted)
}

// validateRollbackReferences only accepts a complete successful historical snapshot. Image
// existence is finally confirmed by the Docker pull in the executor, while incomplete or
// unreadable snapshots are rejected before any runtime operation can touch the live service.
func (handler *Handler) validateRollbackReferences(tx dataservices.DataStoreTx, source *portainer.PlatformRelease, payload rollbackReleasePayload) (*releaseReferenceSet, error) {
	if source == nil || source.Status != portainer.PlatformReleaseStatusSucceeded {
		return nil, validationFailedError("Only a successful historical release can be rolled back")
	}
	if source.Image == "" && source.ArtifactSnapshot.ImageRef == "" {
		return nil, validationFailedError("Historical release image snapshot is unavailable")
	}
	if source.ConfigSnapshot.SpecRevision <= 0 {
		return nil, validationFailedError("Historical release configuration snapshot is unavailable")
	}
	if err := portainer.ValidatePlatformDeploymentDesiredSpecV01(source.ConfigSnapshot.DesiredSpecSnapshot); err != nil {
		return nil, validationFailedError("Historical release configuration snapshot is invalid")
	}
	if err := handler.validateRollbackSecretSnapshots(source.ConfigSnapshot.SecretSnapshots); err != nil {
		return nil, err
	}

	project, err := readActiveProject(tx, source.ProjectID)
	if err != nil {
		return nil, err
	}
	environment, err := readActiveEnvironment(tx, source.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if environment.IsProduction && !payload.ConfirmProduction {
		return nil, validationFailedError("Production rollback requires explicit confirmation")
	}
	application, err := readActiveApplication(tx, source.ApplicationID)
	if err != nil {
		return nil, err
	}
	service, err := readActiveServiceDefinition(tx, source.ServiceDefinitionID)
	if err != nil {
		return nil, err
	}
	deployment, err := readActiveServiceDeployment(tx, source.ServiceDeploymentID)
	if err != nil {
		return nil, err
	}
	if deployment.CurrentServingReleaseID == source.ID {
		return nil, validationFailedError("Historical release is already serving")
	}
	if environment.ProjectID != source.ProjectID || application.ProjectID != source.ProjectID || service.ProjectID != source.ProjectID || deployment.ProjectID != source.ProjectID ||
		service.ApplicationID != source.ApplicationID || deployment.ApplicationID != source.ApplicationID || deployment.ServiceDefinitionID != source.ServiceDefinitionID || deployment.EnvironmentID != source.EnvironmentID {
		return nil, validationFailedError("Historical release resources are no longer compatible")
	}

	rollbackDeployment := *deployment
	rollbackDeployment.DesiredSpec = source.ConfigSnapshot.DesiredSpecSnapshot
	rollbackDeployment.SpecRevision = source.ConfigSnapshot.SpecRevision
	artifact := artifactFromRollbackSource(source)

	return &releaseReferenceSet{
		project:     project,
		environment: environment,
		application: application,
		service:     service,
		deployment:  &rollbackDeployment,
		artifact:    artifact,
	}, nil
}

func (handler *Handler) validateRollbackSecretSnapshots(snapshots []portainer.PlatformSecretSnapshot) error {
	var cipher *platformservice.SecretCipher
	for _, snapshot := range snapshots {
		if !snapshot.HasValue {
			continue
		}
		if snapshot.Name == "" || snapshot.CipherText == "" || snapshot.Hash == "" || snapshot.EncryptionVersion != platformservice.PlatformSecretEncryptionVersion {
			return validationFailedError("Historical release sensitive configuration snapshot is unavailable")
		}
		if cipher == nil {
			var err error
			cipher, err = platformservice.NewSecretCipher(handler.DataStore.Connection())
			if err != nil {
				return validationFailedError("Historical release sensitive configuration snapshot is unavailable")
			}
		}
		if _, err := cipher.Decrypt(snapshot.CipherText); err != nil {
			return validationFailedError("Historical release sensitive configuration snapshot is unavailable")
		}
	}
	return nil
}

func artifactFromRollbackSource(source *portainer.PlatformRelease) *portainer.PlatformArtifact {
	image := source.Image
	if image == "" {
		image = source.ArtifactSnapshot.ImageRef
	}
	return &portainer.PlatformArtifact{
		ID:                  source.ArtifactID,
		ProjectID:           source.ProjectID,
		ApplicationID:       source.ApplicationID,
		ServiceDefinitionID: source.ServiceDefinitionID,
		Name:                source.ArtifactSnapshot.Name,
		Version:             source.ArtifactSnapshot.Version,
		Type:                portainer.PlatformArtifactTypeImage,
		SourceType:          portainer.PlatformArtifactSourceImageReference,
		ImageRef:            image,
		ImageDigest:         firstNonEmptyString(source.ImageDigest, source.ArtifactSnapshot.ImageDigest),
		Traceability:        source.Traceability,
		RegistryID:          source.ArtifactSnapshot.RegistryID,
		SHA256:              source.ArtifactSnapshot.SHA256,
		Size:                source.ArtifactSnapshot.Size,
		Retained:            source.ArtifactSnapshot.Retained,
	}
}

func newRollbackRelease(source *portainer.PlatformRelease, deployment *portainer.PlatformServiceDeployment, artifact *portainer.PlatformArtifact, userID portainer.UserID, idempotencyKeyHash string, payloadHash string, now int64) *portainer.PlatformRelease {
	return &portainer.PlatformRelease{
		ProjectID:               source.ProjectID,
		EnvironmentID:           source.EnvironmentID,
		ApplicationID:           source.ApplicationID,
		ServiceDefinitionID:     source.ServiceDefinitionID,
		ServiceDeploymentID:     source.ServiceDeploymentID,
		ArtifactID:              source.ArtifactID,
		Version:                 source.Version,
		TriggerType:             portainer.PlatformReleaseTriggerRollback,
		Strategy:                source.Strategy,
		Status:                  portainer.PlatformReleaseStatusQueued,
		OperatorUserID:          userID,
		IdempotencyKeyHash:      idempotencyKeyHash,
		PayloadHash:             payloadHash,
		ExpectedSpecRevision:    source.ConfigSnapshot.SpecRevision,
		Image:                   artifact.ImageRef,
		ImageDigest:             artifact.ImageDigest,
		Traceability:            source.Traceability,
		ArtifactSnapshot:        source.ArtifactSnapshot,
		ConfigSnapshot:          source.ConfigSnapshot,
		TargetSnapshot:          targetSnapshotFromDeployment(deployment),
		HealthCheckResult:       portainer.PlatformHealthCheckResult{Status: portainer.PlatformHealthCheckStatusSkipped},
		PreviousReleaseID:       deployment.CurrentServingReleaseID,
		RollbackSourceReleaseID: source.ID,
		ResolutionAction:        portainer.PlatformReleaseResolutionNone,
		CreatedAt:               now,
		QueueExpiresAt:          now + 600,
	}
}

func rollbackPayloadHash(source portainer.PlatformRelease, payload rollbackReleasePayload) string {
	return hashString(fmt.Sprintf("rollback:%d:%s:%s:%t", source.ID, source.Image, source.ConfigSnapshot.ConfigHash, payload.ConfirmProduction))
}

func ensureRollbackReleaseLock(tx dataservices.DataStoreTx, deploymentID portainer.PlatformServiceDeploymentID) error {
	lockID := platformreleaselock.LockIDForServiceDeployment(deploymentID)
	lock, err := tx.PlatformReleaseLock().Read(lockID)
	if err != nil && !tx.IsErrObjectNotFound(err) {
		return err
	}
	if lock == nil {
		return nil
	}
	lockedRelease, err := tx.PlatformRelease().Read(lock.ReleaseID)
	if err != nil && !tx.IsErrObjectNotFound(err) {
		return err
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
	if err := tx.PlatformReleaseLock().Delete(lockID); err != nil {
		return err
	}
	return nil
}

func buildRollbackDiff(source portainer.PlatformRelease, current *portainer.PlatformRelease, production bool) rollbackDiffResponse {
	result := rollbackDiffResponse{
		SourceReleaseID: source.ID,
		SourceImage:     releaseImage(source),
		Production:      production,
	}
	if current == nil {
		result.ImageChanged = true
		result.ConfigChanged = true
		result.PortsChanged = len(source.ConfigSnapshot.DesiredSpecSnapshot.Ports) > 0
		result.EnvironmentChanged = len(source.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides) > 0
		result.ChangedConfigKeys = changedConfigKeys(source.ConfigSnapshot.EffectiveConfigSnapshot, portainer.PlatformEffectiveConfigSnapshot{})
		result.ChangedEnvironmentNames = changedEnvironmentNames(source.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides, nil)
		result.SensitiveVariables = changedSensitiveVariables(source, nil)
		return result
	}

	result.CurrentReleaseID = current.ID
	result.CurrentImage = releaseImage(*current)
	result.ImageChanged = result.SourceImage != result.CurrentImage || source.ImageDigest != current.ImageDigest
	result.ConfigChanged = !reflect.DeepEqual(source.ConfigSnapshot.EffectiveConfigSnapshot, current.ConfigSnapshot.EffectiveConfigSnapshot)
	result.PortsChanged = !reflect.DeepEqual(source.ConfigSnapshot.DesiredSpecSnapshot.Ports, current.ConfigSnapshot.DesiredSpecSnapshot.Ports)
	result.EnvironmentChanged = !reflect.DeepEqual(source.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides, current.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides)
	result.ChangedConfigKeys = changedConfigKeys(source.ConfigSnapshot.EffectiveConfigSnapshot, current.ConfigSnapshot.EffectiveConfigSnapshot)
	result.ChangedEnvironmentNames = changedEnvironmentNames(source.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides, current.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides)
	result.SensitiveVariables = changedSensitiveVariables(source, current)
	return result
}

func releaseImage(release portainer.PlatformRelease) string {
	return firstNonEmptyString(release.Image, release.ArtifactSnapshot.ImageRef)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func changedConfigKeys(source, current portainer.PlatformEffectiveConfigSnapshot) []string {
	sourceEntries := configEntryFingerprints(source.Entries)
	currentEntries := configEntryFingerprints(current.Entries)
	return changedNames(sourceEntries, currentEntries)
}

func configEntryFingerprints(entries []portainer.PlatformConfigEntrySnapshot) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.Sensitive {
			continue
		}
		result[entry.Key] = hashString(fmt.Sprintf("%q:%q:%t:%t:%q:%q", entry.ValueType, entry.Value, entry.Required, entry.HasValue, entry.Source, entry.Hash))
	}
	return result
}

func changedEnvironmentNames(source, current []portainer.PlatformEnvVar) []string {
	sourceEntries := environmentFingerprints(source)
	currentEntries := environmentFingerprints(current)
	return changedNames(sourceEntries, currentEntries)
}

func environmentFingerprints(entries []portainer.PlatformEnvVar) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsSecret {
			continue
		}
		result[entry.Name] = hashString(fmt.Sprintf("%q:%q:%t:%q", entry.Value, entry.Source, entry.HasValue, entry.Hash))
	}
	return result
}

func changedNames(source, current map[string]string) []string {
	keys := make(map[string]struct{}, len(source)+len(current))
	for key := range source {
		keys[key] = struct{}{}
	}
	for key := range current {
		keys[key] = struct{}{}
	}

	result := make([]string, 0, len(keys))
	for key := range keys {
		if source[key] != current[key] {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}

type rollbackSensitiveState struct {
	hasValue    bool
	fingerprint string
}

func changedSensitiveVariables(source portainer.PlatformRelease, current *portainer.PlatformRelease) []rollbackSensitiveVariable {
	sourceStates := sensitiveVariableStates(source)
	currentStates := map[string]rollbackSensitiveState{}
	if current != nil {
		currentStates = sensitiveVariableStates(*current)
	}
	keys := make(map[string]struct{}, len(sourceStates)+len(currentStates))
	for key := range sourceStates {
		keys[key] = struct{}{}
	}
	for key := range currentStates {
		keys[key] = struct{}{}
	}

	result := make([]rollbackSensitiveVariable, 0, len(keys))
	for key := range keys {
		sourceState := sourceStates[key]
		currentState := currentStates[key]
		if sourceState.fingerprint == currentState.fingerprint {
			continue
		}
		result = append(result, rollbackSensitiveVariable{
			Name:            key,
			SourceHasValue:  sourceState.hasValue,
			CurrentHasValue: currentState.hasValue,
			Changed:         true,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func sensitiveVariableStates(release portainer.PlatformRelease) map[string]rollbackSensitiveState {
	result := map[string]rollbackSensitiveState{}
	for _, entry := range release.ConfigSnapshot.EffectiveConfigSnapshot.Entries {
		if entry.Sensitive {
			result[entry.Key] = rollbackSensitiveState{hasValue: entry.HasValue, fingerprint: hashString(fmt.Sprintf("config:%q:%t:%q", entry.ValueType, entry.HasValue, entry.Hash))}
		}
	}
	for _, snapshot := range release.ConfigSnapshot.SecretSnapshots {
		if snapshot.Name != "" {
			result[snapshot.Name] = rollbackSensitiveState{hasValue: snapshot.HasValue, fingerprint: hashString(fmt.Sprintf("secret:%t:%q:%q", snapshot.HasValue, snapshot.EncryptionVersion, snapshot.Hash))}
		}
	}
	for _, variable := range release.ConfigSnapshot.DesiredSpecSnapshot.EnvOverrides {
		if variable.IsSecret && variable.Name != "" {
			result[variable.Name] = rollbackSensitiveState{hasValue: variable.HasValue, fingerprint: hashString(fmt.Sprintf("environment:%t:%q:%q", variable.HasValue, variable.Source, variable.Hash))}
		}
	}
	return result
}
