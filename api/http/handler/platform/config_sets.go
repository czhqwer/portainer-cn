package platform

import (
	"net/http"
	"strconv"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type configSetListFilters struct {
	projectID portainer.PlatformProjectID
	scopeType portainer.PlatformConfigScopeType
	scopeID   int
}

type effectiveConfigResponse struct {
	EffectiveConfig portainer.PlatformEffectiveConfigSnapshot `json:"EffectiveConfig"`
	DriftStatus     portainer.PlatformDeploymentDriftStatus   `json:"DriftStatus"`
}

func (handler *Handler) configSetList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	filters, handlerErr := configSetFilters(r)
	if handlerErr != nil {
		return handlerErr
	}
	visibleProjectIDs, handlerErr := handler.visibleProjectIDs(r)
	if handlerErr != nil {
		return handlerErr
	}
	if filters.projectID != 0 {
		if _, handlerErr := handler.requireProjectPermission(r, filters.projectID, platformPermissionView); handlerErr != nil {
			return handlerErr
		}
	}

	withArchived := includeArchived(r)
	configSets, err := handler.DataStore.PlatformConfigSet().ReadAll(func(configSet portainer.PlatformConfigSet) bool {
		if visibleProjectIDs != nil && !visibleProjectIDs[configSet.ProjectID] {
			return false
		}
		if !withArchived && !isActive(configSet.PlatformLifecycle) {
			return false
		}
		if filters.projectID != 0 && configSet.ProjectID != filters.projectID {
			return false
		}
		if filters.scopeType != "" && configSet.ScopeType != filters.scopeType {
			return false
		}
		return filters.scopeID == 0 || configSet.ScopeID == filters.scopeID
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, redactConfigSets(configSets))
}

func (handler *Handler) configSetInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "configSetId")
	if handlerErr != nil {
		return handlerErr
	}

	configSet, handlerErr := handler.requireConfigSetPermission(r, portainer.PlatformConfigSetID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}
	if !includeArchived(r) && !isActive(configSet.PlatformLifecycle) {
		return notFoundError("Config set is archived")
	}

	return response.JSON(w, redactConfigSet(*configSet))
}

func (handler *Handler) configSetCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	var payload createConfigSetPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if _, handlerErr := handler.requireProjectPermission(r, payload.ProjectID, platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	now := time.Now().Unix()
	configSet := portainer.NewPlatformConfigSet()
	configSet.ProjectID = payload.ProjectID
	configSet.ScopeType = payload.ScopeType
	configSet.ScopeID = payload.ScopeID
	configSet.Name = payload.Name
	configSet.Entries = payload.Entries
	configSet.PlatformLifecycle = newLifecycle(now)
	if err := handler.encryptSensitiveConfigEntries(configSet.Entries, nil); err != nil {
		return validationFailed(err)
	}
	portainer.NormalizePlatformConfigSet(&configSet)

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if err := validateConfigSetScope(tx, &configSet); err != nil {
			return err
		}
		if err := ensureConfigSetNameAvailable(tx, configSet, 0); err != nil {
			return err
		}
		if err := tx.PlatformConfigSet().Create(&configSet); err != nil {
			return err
		}
		return refreshProjectConfigDrift(tx, configSet.ProjectID)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, redactConfigSet(configSet), http.StatusCreated)
}

func (handler *Handler) configSetUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "configSetId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireConfigSetPermission(r, portainer.PlatformConfigSetID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	var payload updateConfigSetPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	var configSet *portainer.PlatformConfigSet
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		configSet, err = readActiveConfigSet(tx, portainer.PlatformConfigSetID(id))
		if err != nil {
			return err
		}
		if err := requireResourceVersion(configSet.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}
		existingEntries := append([]portainer.PlatformConfigEntry(nil), configSet.Entries...)
		if payload.Name != nil {
			configSet.Name = *payload.Name
		}
		if payload.Entries != nil {
			configSet.Entries = *payload.Entries
		}
		portainer.NormalizePlatformConfigSet(configSet)
		if err := handler.encryptSensitiveConfigEntries(configSet.Entries, existingEntries); err != nil {
			return validationFailedError(err.Error())
		}
		if err := portainer.ValidatePlatformConfigSet(*configSet); err != nil {
			return validationFailedError(err.Error())
		}
		if err := ensureConfigSetNameAvailable(tx, *configSet, configSet.ID); err != nil {
			return err
		}
		touchLifecycle(&configSet.PlatformLifecycle, now)
		if err := tx.PlatformConfigSet().Update(configSet.ID, configSet); err != nil {
			return err
		}
		return refreshProjectConfigDrift(tx, configSet.ProjectID)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, redactConfigSet(*configSet))
}

func (handler *Handler) configSetArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "configSetId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireConfigSetPermission(r, portainer.PlatformConfigSetID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}
	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		configSet, err := readActiveConfigSet(tx, portainer.PlatformConfigSetID(id))
		if err != nil {
			return err
		}
		archiveLifecycle(&configSet.PlatformLifecycle, time.Now().Unix(), userID)
		if err := tx.PlatformConfigSet().Update(configSet.ID, configSet); err != nil {
			return err
		}
		return refreshProjectConfigDrift(tx, configSet.ProjectID)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}

func (handler *Handler) serviceDeploymentEffectiveConfig(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireServiceDeploymentPermission(r, portainer.PlatformServiceDeploymentID(id), platformPermissionView); handlerErr != nil {
		return handlerErr
	}

	var result effectiveConfigResponse
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		deployment, err := readActiveServiceDeployment(tx, portainer.PlatformServiceDeploymentID(id))
		if err != nil {
			return err
		}
		effectiveConfig, err := effectiveConfigForDeployment(tx, *deployment)
		if err != nil {
			return validationFailedError(err.Error())
		}
		if err := refreshDeploymentConfigDrift(tx, deployment, effectiveConfig); err != nil {
			return err
		}
		result = effectiveConfigResponse{EffectiveConfig: effectiveConfig, DriftStatus: deployment.DriftStatus}
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, result)
}

// effectiveConfigForDeployment loads only active project-local sets before invoking the
// pure resolver. 这样 handler 负责对象可见性和 datastore 边界，合并顺序保持可单测并可供发布链路复用。
func effectiveConfigForDeployment(tx dataservices.DataStoreTx, deployment portainer.PlatformServiceDeployment) (portainer.PlatformEffectiveConfigSnapshot, error) {
	configSets, err := configSetsForDeployment(tx, deployment)
	if err != nil {
		return portainer.PlatformEffectiveConfigSnapshot{}, err
	}

	return platformservice.BuildEffectiveConfigSnapshot(deployment, configSets)
}

func secretSnapshotsForDeployment(tx dataservices.DataStoreTx, deployment portainer.PlatformServiceDeployment) ([]portainer.PlatformSecretSnapshot, error) {
	configSets, err := configSetsForDeployment(tx, deployment)
	if err != nil {
		return nil, err
	}
	return platformservice.BuildSecretSnapshots(deployment, configSets)
}

func configSetsForDeployment(tx dataservices.DataStoreTx, deployment portainer.PlatformServiceDeployment) ([]portainer.PlatformConfigSet, error) {
	return tx.PlatformConfigSet().ReadAll(func(configSet portainer.PlatformConfigSet) bool {
		return configSet.ProjectID == deployment.ProjectID && isActive(configSet.PlatformLifecycle)
	})
}

func validateConfigSetScope(tx dataservices.DataStoreTx, configSet *portainer.PlatformConfigSet) error {
	if _, err := readActiveProject(tx, configSet.ProjectID); err != nil {
		return err
	}

	switch configSet.ScopeType {
	case portainer.PlatformConfigScopeProject:
		if configSet.ScopeID != int(configSet.ProjectID) {
			return validationFailedError("Project-scoped config set ScopeId must equal ProjectId")
		}
	case portainer.PlatformConfigScopeEnvironment:
		environment, err := readActiveEnvironment(tx, portainer.PlatformEnvironmentID(configSet.ScopeID))
		if err != nil {
			return err
		}
		if environment.ProjectID != configSet.ProjectID {
			return validationFailedError("Environment does not belong to Project")
		}
	case portainer.PlatformConfigScopeServiceDeployment:
		deployment, err := readActiveServiceDeployment(tx, portainer.PlatformServiceDeploymentID(configSet.ScopeID))
		if err != nil {
			return err
		}
		if deployment.ProjectID != configSet.ProjectID {
			return validationFailedError("Service deployment does not belong to Project")
		}
	default:
		return validationFailedError("Config set ScopeType is invalid")
	}

	return nil
}

func ensureConfigSetNameAvailable(tx dataservices.DataStoreTx, configSet portainer.PlatformConfigSet, ignoreID portainer.PlatformConfigSetID) error {
	existing, err := tx.PlatformConfigSet().ReadAll(func(candidate portainer.PlatformConfigSet) bool {
		return candidate.ID != ignoreID &&
			candidate.ProjectID == configSet.ProjectID &&
			candidate.ScopeType == configSet.ScopeType &&
			candidate.ScopeID == configSet.ScopeID &&
			candidate.Name == configSet.Name &&
			isActive(candidate.PlatformLifecycle)
	})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return duplicateError("Config set name already exists for scope")
	}
	return nil
}

func refreshProjectConfigDrift(tx dataservices.DataStoreTx, projectID portainer.PlatformProjectID) error {
	deployments, err := tx.PlatformServiceDeployment().ReadAll(func(deployment portainer.PlatformServiceDeployment) bool {
		return deployment.ProjectID == projectID && isActive(deployment.PlatformLifecycle) && deployment.CurrentServingReleaseID != 0
	})
	if err != nil {
		return err
	}
	for i := range deployments {
		effectiveConfig, err := effectiveConfigForDeployment(tx, deployments[i])
		if err != nil {
			deployments[i].DriftStatus = portainer.PlatformDeploymentDriftConfigChanged
			if updateErr := tx.PlatformServiceDeployment().Update(deployments[i].ID, &deployments[i]); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err := refreshDeploymentConfigDrift(tx, &deployments[i], effectiveConfig); err != nil {
			return err
		}
	}
	return nil
}

func refreshDeploymentConfigDrift(tx dataservices.DataStoreTx, deployment *portainer.PlatformServiceDeployment, effectiveConfig portainer.PlatformEffectiveConfigSnapshot) error {
	if deployment.CurrentServingReleaseID == 0 || deployment.DriftStatus == portainer.PlatformDeploymentDriftRuntimeMissing {
		return nil
	}
	release, err := tx.PlatformRelease().Read(deployment.CurrentServingReleaseID)
	if err != nil {
		return err
	}
	previous := release.ConfigSnapshot.EffectiveConfigSnapshot
	if previous.Hash == "" && len(previous.Entries) == 0 && len(previous.ConfigSetRevisions) == 0 {
		return nil
	}

	drift := portainer.PlatformDeploymentDriftNone
	if previous.Hash != effectiveConfig.Hash {
		drift = portainer.PlatformDeploymentDriftConfigChanged
	}
	if deployment.DriftStatus == drift {
		return nil
	}
	deployment.DriftStatus = drift
	return tx.PlatformServiceDeployment().Update(deployment.ID, deployment)
}

func readActiveConfigSet(tx dataservices.DataStoreTx, id portainer.PlatformConfigSetID) (*portainer.PlatformConfigSet, error) {
	configSet, err := tx.PlatformConfigSet().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(configSet.PlatformLifecycle) {
		return nil, notFoundError("Config set is archived")
	}
	return configSet, nil
}

func configSetFilters(r *http.Request) (configSetListFilters, *httperror.HandlerError) {
	filters := configSetListFilters{scopeType: portainer.PlatformConfigScopeType(r.URL.Query().Get("scopeType"))}
	var err error
	if filters.projectID, err = configSetQueryProjectID(r); err != nil {
		return configSetListFilters{}, validationFailed(err)
	}
	if filters.scopeID, err = configSetQueryScopeID(r); err != nil {
		return configSetListFilters{}, validationFailed(err)
	}
	return filters, nil
}

func configSetQueryProjectID(r *http.Request) (portainer.PlatformProjectID, error) {
	value := r.URL.Query().Get("projectId")
	if value == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return portainer.PlatformProjectID(id), nil
}

func configSetQueryScopeID(r *http.Request) (int, error) {
	value := r.URL.Query().Get("scopeId")
	if value == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

func redactConfigSets(configSets []portainer.PlatformConfigSet) []portainer.PlatformConfigSet {
	result := make([]portainer.PlatformConfigSet, len(configSets))
	for i := range configSets {
		result[i] = redactConfigSet(configSets[i])
	}
	return result
}

func redactConfigSet(configSet portainer.PlatformConfigSet) portainer.PlatformConfigSet {
	result := configSet
	result.Entries = append([]portainer.PlatformConfigEntry(nil), configSet.Entries...)
	for i := range result.Entries {
		if result.Entries[i].Sensitive {
			result.Entries[i].Value = ""
		}
		result.Entries[i].CipherText = ""
	}
	return result
}
