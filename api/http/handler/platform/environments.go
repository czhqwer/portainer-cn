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

func (handler *Handler) environmentList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}

	if _, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(id), platformPermissionView); handlerErr != nil {
		return handlerErr
	}

	withArchived := includeArchived(r)
	environments, err := handler.DataStore.PlatformEnvironment().ReadAll(func(environment portainer.PlatformEnvironment) bool {
		return environment.ProjectID == portainer.PlatformProjectID(id) && (withArchived || isActive(environment.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, environments)
}

func (handler *Handler) environmentInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "environmentId")
	if handlerErr != nil {
		return handlerErr
	}

	environment, handlerErr := handler.requireEnvironmentPermission(r, portainer.PlatformEnvironmentID(id), platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}

	return response.JSON(w, environment)
}

func (handler *Handler) environmentCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	var payload createEnvironmentPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}

	now := time.Now().Unix()
	environment := &portainer.PlatformEnvironment{
		ProjectID:         portainer.PlatformProjectID(projectID),
		Name:              payload.Name,
		Slug:              payload.Slug,
		Type:              payload.Type,
		IsProduction:      payload.IsProduction,
		TargetMode:        payload.TargetMode,
		Targets:           payload.Targets,
		DefaultRegistryID: payload.DefaultRegistryID,
		HealthCheckHost:   payload.HealthCheckHost,
		ReleasePolicy:     payload.ReleasePolicy,
		BatchPolicy:       payload.BatchPolicy,
		PlatformLifecycle: newLifecycle(now),
	}
	normalizeEnvironment(environment)
	if len(environment.Targets) > 0 {
		if handlerErr := handler.requireEndpointAccessForTargets(r, environment.Targets); handlerErr != nil {
			return handlerErr
		}
	}

	// 环境只绑定 Portainer Endpoint 等目标描述，不创建或修改任何运行时资源；
	// Docker/Agent 侧真实验证留给 Gate 0B Spike 后的执行器批次。
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		if _, err := readActiveProject(tx, environment.ProjectID); err != nil {
			return err
		}

		existingEnvironments, err := tx.PlatformEnvironment().ReadAll(func(existing portainer.PlatformEnvironment) bool {
			return existing.ProjectID == environment.ProjectID &&
				isActive(existing.PlatformLifecycle) &&
				existing.Slug == environment.Slug
		})
		if err != nil {
			return err
		}
		if len(existingEnvironments) > 0 {
			return duplicateError("Environment slug already exists in project")
		}

		return tx.PlatformEnvironment().Create(environment)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSONWithStatus(w, environment, http.StatusCreated)
}

func (handler *Handler) environmentUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "environmentId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireEnvironmentPermission(r, portainer.PlatformEnvironmentID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	var payload updateEnvironmentPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	if payload.Targets != nil && len(*payload.Targets) > 0 {
		if handlerErr := handler.requireEndpointAccessForTargets(r, *payload.Targets); handlerErr != nil {
			return handlerErr
		}
	}

	now := time.Now().Unix()
	var environment *portainer.PlatformEnvironment

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		var err error
		environment, err = readActiveEnvironment(tx, portainer.PlatformEnvironmentID(id))
		if err != nil {
			return err
		}
		if err := requireResourceVersion(environment.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}

		if payload.Name != nil {
			environment.Name = *payload.Name
		}
		if payload.Type != nil {
			environment.Type = *payload.Type
		}
		if payload.IsProduction != nil {
			environment.IsProduction = *payload.IsProduction
		}
		if payload.TargetMode != nil {
			environment.TargetMode = *payload.TargetMode
		}
		if payload.Targets != nil {
			environment.Targets = *payload.Targets
		}
		if payload.DefaultRegistryID != nil {
			environment.DefaultRegistryID = *payload.DefaultRegistryID
		}
		if payload.HealthCheckHost != nil {
			environment.HealthCheckHost = *payload.HealthCheckHost
		}
		if payload.ReleasePolicy != nil {
			environment.ReleasePolicy = *payload.ReleasePolicy
		}
		if payload.HostGroupID != nil {
			if *payload.HostGroupID == 0 {
				environment.HostGroupID = 0
			} else {
				group, err := tx.PlatformHostGroup().Read(*payload.HostGroupID)
				if err != nil {
					return err
				}
				if !isActive(group.PlatformLifecycle) || group.ProjectID != environment.ProjectID || group.EnvironmentID != environment.ID {
					return validationFailedError("HostGroupId does not belong to environment")
				}
				environment.HostGroupID = group.ID
			}
		}
		if payload.BatchPolicy != nil {
			environment.BatchPolicy = *payload.BatchPolicy
		}
		normalizeEnvironment(environment)
		if environment.TargetMode == portainer.PlatformTargetModeMulti {
			if err := validateMultiEnvironmentTargets(*environment); err != nil {
				return validationFailedError(err.Error())
			}
		}
		touchLifecycle(&environment.PlatformLifecycle, now)

		return tx.PlatformEnvironment().Update(environment.ID, environment)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.JSON(w, environment)
}

// validateMultiEnvironmentTargets 将多主机边界放在服务端，避免客户端通过缺失 gateway、重复 target
// 或容器内部地址制造无法恢复的发布拓扑。HostAddress 是网关转发和发布预检的唯一入口。
func validateMultiEnvironmentTargets(environment portainer.PlatformEnvironment) error {
	workloads, gateways := 0, 0
	seen := map[portainer.EndpointID]struct{}{}
	for _, target := range environment.Targets {
		if !target.Enabled {
			continue
		}
		if target.EndpointID <= 0 {
			return errors.New("multi target endpoint is required")
		}
		if _, exists := seen[target.EndpointID]; exists {
			return errors.New("multi target endpoint is duplicated")
		}
		seen[target.EndpointID] = struct{}{}
		switch target.Role {
		case portainer.PlatformDeploymentTargetRoleWorkload:
			if strings.TrimSpace(target.HostAddress) == "" || strings.ContainsAny(target.HostAddress, "\r\n\x00/\\@") {
				return errors.New("multi workload host address is invalid")
			}
			workloads++
		case portainer.PlatformDeploymentTargetRoleGateway:
			gateways++
		default:
			return errors.New("multi target role is invalid")
		}
	}
	if workloads == 0 || gateways != 1 {
		return errors.New("multi environment requires workload targets and one gateway target")
	}
	return portainer.ValidatePlatformBatchPolicy(environment.BatchPolicy)
}

func (handler *Handler) environmentArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	id, handlerErr := handler.routeID(r, "environmentId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr := handler.requireEnvironmentPermission(r, portainer.PlatformEnvironmentID(id), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}

	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}

	now := time.Now().Unix()

	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		environment, err := readActiveEnvironment(tx, portainer.PlatformEnvironmentID(id))
		if err != nil {
			return err
		}

		deployments, err := tx.PlatformServiceDeployment().ReadAll(func(deployment portainer.PlatformServiceDeployment) bool {
			return deployment.EnvironmentID == environment.ID && isActive(deployment.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(deployments) > 0 {
			return duplicateError("Environment has active service deployments")
		}

		archiveLifecycle(&environment.PlatformLifecycle, now, userID)

		return tx.PlatformEnvironment().Update(environment.ID, environment)
	})
	if err != nil {
		return handler.convertError(err)
	}

	return response.Empty(w)
}
