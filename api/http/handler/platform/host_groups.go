package platform

import (
	"errors"
	"net/http"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type createHostGroupPayload struct {
	EnvironmentID portainer.PlatformEnvironmentID      `json:"EnvironmentId"`
	Name          string                               `json:"Name"`
	Targets       []portainer.PlatformDeploymentTarget `json:"Targets"`
}

type updateHostGroupPayload struct {
	ResourceVersion int                                  `json:"ResourceVersion"`
	Name            string                               `json:"Name"`
	Targets         []portainer.PlatformDeploymentTarget `json:"Targets"`
}

func (payload *createHostGroupPayload) Validate(*http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.EnvironmentID <= 0 || payload.Name == "" || len(payload.Targets) == 0 {
		return errors.New("environment, name and targets are required")
	}
	return nil
}

func (payload *updateHostGroupPayload) Validate(*http.Request) error {
	payload.Name = strings.TrimSpace(payload.Name)
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.Name == "" || len(payload.Targets) == 0 {
		return errors.New("name and targets are required")
	}
	return nil
}

func (handler *Handler) hostGroupList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	groups, err := handler.DataStore.PlatformHostGroup().ReadAll(func(group portainer.PlatformHostGroup) bool {
		return group.ProjectID == portainer.PlatformProjectID(projectID) && (includeArchived(r) || isActive(group.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, groups)
}

func (handler *Handler) hostGroupCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}
	var payload createHostGroupPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	environment, handlerErr := handler.requireEnvironmentPermission(r, payload.EnvironmentID, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if environment.ProjectID != portainer.PlatformProjectID(projectID) {
		return platformAccessDenied()
	}
	if handlerErr = handler.requireEndpointAccessForTargets(r, payload.Targets); handlerErr != nil {
		return handlerErr
	}
	group := portainer.NewPlatformHostGroup()
	group.ProjectID, group.EnvironmentID, group.Name, group.Targets = portainer.PlatformProjectID(projectID), environment.ID, payload.Name, payload.Targets
	group.PlatformLifecycle = newLifecycle(time.Now().Unix())
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existing, err := tx.PlatformHostGroup().ReadAll(func(item portainer.PlatformHostGroup) bool {
			return item.EnvironmentID == group.EnvironmentID && isActive(item.PlatformLifecycle) && strings.EqualFold(item.Name, group.Name)
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return duplicateError("host group name already exists in environment")
		}
		if err := tx.PlatformHostGroup().Create(&group); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionHostGroupCreated, Result: portainer.PlatformAuditResultSuccess, ProjectID: group.ProjectID, EnvironmentID: group.EnvironmentID, AfterSummary: map[string]any{"hostGroupId": group.ID, "targetCount": len(group.Targets)}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSONWithStatus(w, group, http.StatusCreated)
}

func (handler *Handler) hostGroupInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	group, handlerErr := handler.hostGroupFromRequest(r, platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}
	return response.JSON(w, group)
}

func (handler *Handler) hostGroupUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	group, handlerErr := handler.hostGroupFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	var payload updateHostGroupPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if handlerErr = handler.requireEndpointAccessForTargets(r, payload.Targets); handlerErr != nil {
		return handlerErr
	}
	var updated *portainer.PlatformHostGroup
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformHostGroup().Read(group.ID)
		if err != nil {
			return err
		}
		if !isActive(current.PlatformLifecycle) {
			return notFoundError("Host group is archived")
		}
		if err := requireResourceVersion(current.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}
		before := *current
		current.Name, current.Targets = payload.Name, payload.Targets
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformHostGroup().Update(current.ID, current); err != nil {
			return err
		}
		if err := handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionHostGroupUpdated, Result: portainer.PlatformAuditResultSuccess, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID, BeforeSummary: map[string]any{"hostGroupId": before.ID, "targetCount": len(before.Targets)}, AfterSummary: map[string]any{"hostGroupId": current.ID, "targetCount": len(current.Targets)}}); err != nil {
			return err
		}
		updated = current
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, updated)
}

func (handler *Handler) hostGroupArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	group, handlerErr := handler.hostGroupFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformHostGroup().Read(group.ID)
		if err != nil {
			return err
		}
		if !isActive(current.PlatformLifecycle) {
			return notFoundError("Host group is archived")
		}
		environment, err := tx.PlatformEnvironment().Read(current.EnvironmentID)
		if err != nil {
			return err
		}
		if environment.HostGroupID == current.ID {
			return conflictError("Host group is referenced by environment")
		}
		archiveLifecycle(&current.PlatformLifecycle, time.Now().Unix(), userID)
		if err := tx.PlatformHostGroup().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionHostGroupArchived, Result: portainer.PlatformAuditResultSuccess, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID, AfterSummary: map[string]any{"hostGroupId": current.ID, "archived": true}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.Empty(w)
}

func (handler *Handler) hostGroupFromRequest(r *http.Request, permission platformPermission) (*portainer.PlatformHostGroup, *httperror.HandlerError) {
	id, handlerErr := handler.routeID(r, "hostGroupId")
	if handlerErr != nil {
		return nil, handlerErr
	}
	group, err := handler.DataStore.PlatformHostGroup().Read(portainer.PlatformHostGroupID(id))
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr = handler.requireProjectPermission(r, group.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}
	return group, nil
}
