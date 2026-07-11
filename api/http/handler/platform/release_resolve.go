package platform

import (
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/portainer/portainer/api/dataservices/platformreleaselock"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) releaseResolve(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "releaseId")
	if handlerErr != nil {
		return handlerErr
	}
	release, handlerErr := handler.requireReleasePermission(r, portainer.PlatformReleaseID(id), platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handlerErr := handler.requireReleaseRuntimePermission(r, release); handlerErr != nil {
		return handlerErr
	}

	var payload resolveReleasePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()
	var resolved *portainer.PlatformRelease
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		release, err := tx.PlatformRelease().Read(portainer.PlatformReleaseID(id))
		if err != nil {
			return err
		}
		if err := validateReleaseTransition(release.Status, portainer.PlatformReleaseStatusResolved); err != nil {
			return validationFailedError("Release can only be resolved from interrupted or recovery-failed")
		}
		if payload.Action == portainer.PlatformReleaseResolutionAcceptCurrent && release.RuntimeSnapshot.CurrentRuntimeRef.ResourceID == "" {
			return validationFailedError("accept-current requires a current runtime reference")
		}

		deployment, err := tx.PlatformServiceDeployment().Read(release.ServiceDeploymentID)
		if err != nil {
			return err
		}
		before := releaseAuditSummary(*release)

		release.Status = portainer.PlatformReleaseStatusResolved
		release.ManualActionRequired = false
		release.ResolutionAction = payload.Action
		release.ResolvedByUserID = userID
		release.ResolvedAt = now
		release.ResolutionComment = payload.Comment
		release.FinishedAt = now
		release.LeaseOwner = ""
		release.LeaseExpiresAt = 0
		release.Steps = append(release.Steps, portainer.PlatformReleaseStep{
			Name:       "resolve",
			Status:     portainer.PlatformReleaseStepStatusSucceeded,
			Reason:     string(payload.Action),
			Message:    payload.Comment,
			RuntimeRef: release.RuntimeSnapshot.CurrentRuntimeRef,
			StartedAt:  now,
			FinishedAt: now,
		})

		if err := tx.PlatformRelease().Update(release.ID, release); err != nil {
			return err
		}

		if payload.Action == portainer.PlatformReleaseResolutionAcceptCurrent {
			applyAcceptCurrentResolution(deployment, *release, now)
			if err := tx.PlatformServiceDeployment().Update(deployment.ID, deployment); err != nil {
				return err
			}
		}

		lockID := platformreleaselock.LockIDForServiceDeployment(release.ServiceDeploymentID)
		if err := tx.PlatformReleaseLock().Delete(lockID); err != nil && !tx.IsErrObjectNotFound(err) {
			return err
		}

		if err := handler.createReleaseAuditLog(tx, r, portainer.PlatformAuditActionReleaseResolved, portainer.PlatformAuditResultSuccess, *release, before, releaseAuditSummary(*release), ""); err != nil {
			return err
		}

		resolved = release
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, resolved)
}

func applyAcceptCurrentResolution(deployment *portainer.PlatformServiceDeployment, release portainer.PlatformRelease, now int64) {
	if deployment == nil {
		return
	}

	deployment.CurrentServingReleaseID = release.ID
	deployment.CurrentArtifactID = release.ArtifactID
	deployment.CurrentRuntimeRef = release.RuntimeSnapshot.CurrentRuntimeRef
	deployment.CurrentImage = release.Image
	deployment.LastDeployedSpecRevision = release.ConfigSnapshot.SpecRevision
	if deployment.LastDeployedSpecRevision == 0 {
		deployment.LastDeployedSpecRevision = release.ExpectedSpecRevision
	}
	deployment.LastDeployedAt = now
	deployment.DriftStatus = portainer.PlatformDeploymentDriftNone
	deployment.UpdatedAt = now
	deployment.ResourceVersion++
}
