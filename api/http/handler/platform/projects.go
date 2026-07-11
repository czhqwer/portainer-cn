package platform

import (
	"net/http"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

func (handler *Handler) projectList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	withArchived := includeArchived(r)
	visibleProjectIDs, handlerErr := handler.visibleProjectIDs(r)
	if handlerErr != nil {
		return handlerErr
	}

	projects, err := handler.DataStore.PlatformProject().ReadAll(func(project portainer.PlatformProject) bool {
		return (visibleProjectIDs == nil || visibleProjectIDs[project.ID]) && (withArchived || isActive(project.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, handler.projectResponses(r, projects))
}

func (handler *Handler) projectInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}

	project, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}

	return response.JSON(w, handler.projectResponse(r, *project))
}

func (handler *Handler) projectCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	if handlerErr := handler.requireGlobalAdmin(r); handlerErr != nil {
		return handlerErr
	}
	var payload createProjectPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if err := validateProjectPolicies(payload.MemberPolicies, payload.TeamPolicies); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	project := &portainer.PlatformProject{
		Name:              payload.Name,
		Slug:              payload.Slug,
		Description:       payload.Description,
		MemberPolicies:    payload.MemberPolicies,
		TeamPolicies:      payload.TeamPolicies,
		PlatformLifecycle: newLifecycle(now),
	}

	// 创建项目时只写控制面元数据，并在同一事务内检查 active slug；
	// 真实部署资源必须等后续批次和 Gate 0B，不在这里产生任何 Docker 副作用。
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existingProjects, err := tx.PlatformProject().ReadAll(func(existing portainer.PlatformProject) bool {
			return isActive(existing.PlatformLifecycle) && existing.Slug == payload.Slug
		})
		if err != nil {
			return err
		}
		if len(existingProjects) > 0 {
			return duplicateError("Project slug already exists")
		}

		return tx.PlatformProject().Create(project)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, handler.projectResponse(r, *project), http.StatusCreated)
}

func (handler *Handler) projectUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	project, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(id), platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}

	var payload updateProjectPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		project, err = readActiveProject(tx, portainer.PlatformProjectID(id))
		if err != nil {
			return err
		}
		if err := requireResourceVersion(project.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}

		if payload.Name != nil {
			project.Name = *payload.Name
		}
		if payload.Description != nil {
			project.Description = *payload.Description
		}
		memberPolicies := project.MemberPolicies
		teamPolicies := project.TeamPolicies
		if payload.MemberPolicies != nil {
			memberPolicies = payload.MemberPolicies
		}
		if payload.TeamPolicies != nil {
			teamPolicies = payload.TeamPolicies
		}
		if err := validateProjectPolicies(memberPolicies, teamPolicies); err != nil {
			return validationFailed(err)
		}
		project.MemberPolicies = memberPolicies
		project.TeamPolicies = teamPolicies
		touchLifecycle(&project.PlatformLifecycle, now)

		return tx.PlatformProject().Update(project.ID, project)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, handler.projectResponse(r, *project))
}

func (handler *Handler) projectArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		project, err := readActiveProject(tx, portainer.PlatformProjectID(id))
		if err != nil {
			return err
		}

		environments, err := tx.PlatformEnvironment().ReadAll(func(environment portainer.PlatformEnvironment) bool {
			return environment.ProjectID == project.ID && isActive(environment.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(environments) > 0 {
			return duplicateError("Project has active environments")
		}

		applications, err := tx.PlatformApplication().ReadAll(func(application portainer.PlatformApplication) bool {
			return application.ProjectID == project.ID && isActive(application.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(applications) > 0 {
			return duplicateError("Project has active applications")
		}

		archiveLifecycle(&project.PlatformLifecycle, now, userID)

		return tx.PlatformProject().Update(project.ID, project)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}
