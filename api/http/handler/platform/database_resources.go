package platform

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	platformservice "github.com/portainer/portainer/api/platform"
	httperror "github.com/portainer/portainer/pkg/libhttp/error"
	"github.com/portainer/portainer/pkg/libhttp/request"
	"github.com/portainer/portainer/pkg/libhttp/response"
)

type createDatabaseResourcePayload struct {
	EnvironmentID            portainer.PlatformEnvironmentID `json:"EnvironmentId"`
	EndpointID               portainer.EndpointID            `json:"EndpointId"`
	Name                     string                          `json:"Name"`
	Type                     portainer.PlatformDatabaseType  `json:"Type"`
	Host                     string                          `json:"Host"`
	Port                     int                             `json:"Port"`
	Database                 string                          `json:"Database"`
	Username                 string                          `json:"Username"`
	Password                 *string                         `json:"Password"`
	ConnectionTimeoutSeconds int                             `json:"ConnectionTimeoutSeconds"`
}

type updateDatabaseResourcePayload struct {
	ResourceVersion          int                            `json:"ResourceVersion"`
	Name                     string                         `json:"Name"`
	Type                     portainer.PlatformDatabaseType `json:"Type"`
	Host                     string                         `json:"Host"`
	Port                     int                            `json:"Port"`
	Database                 string                         `json:"Database"`
	Username                 string                         `json:"Username"`
	Password                 *string                        `json:"Password"`
	ConnectionTimeoutSeconds int                            `json:"ConnectionTimeoutSeconds"`
}

type createDatabaseBindingPayload struct {
	DatabaseResourceID portainer.PlatformDatabaseResourceID `json:"DatabaseResourceId"`
}

type updateDatabaseBindingPayload struct {
	ResourceVersion    int                                  `json:"ResourceVersion"`
	DatabaseResourceID portainer.PlatformDatabaseResourceID `json:"DatabaseResourceId"`
}

type databaseResourceTestResponse struct {
	Status string `json:"Status"`
	Reason string `json:"Reason,omitempty"`
}

// databaseWorkbenchContext 只提供未来前端定位工作台所需的环境级连接元数据；密码和
// DatabaseConnection 个人记录不会在项目 API 中出现，防止深链变成跨用户凭据读取入口。
type databaseWorkbenchContext struct {
	DatabaseResourceID portainer.PlatformDatabaseResourceID `json:"DatabaseResourceId"`
	EndpointID         portainer.EndpointID                 `json:"EndpointId"`
	Type               portainer.PlatformDatabaseType       `json:"Type"`
	Host               string                               `json:"Host"`
	Port               int                                  `json:"Port"`
	Database           string                               `json:"Database,omitempty"`
	Username           string                               `json:"Username,omitempty"`
}

func (payload *createDatabaseResourcePayload) Validate(*http.Request) error {
	normalizeDatabaseResourcePayload(payload)
	if payload.EnvironmentID <= 0 || payload.EndpointID <= 0 || payload.Name == "" || payload.Host == "" || payload.Password == nil || *payload.Password == "" {
		return errors.New("environment, endpoint, name, host and password are required")
	}
	return validateDatabaseResourcePayload(payload.Type, payload.Port, payload.ConnectionTimeoutSeconds)
}

func (payload *updateDatabaseResourcePayload) Validate(*http.Request) error {
	normalizeDatabaseResourceUpdatePayload(payload)
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.Name == "" || payload.Host == "" || (payload.Password != nil && *payload.Password == "") {
		return errors.New("name, host and a non-empty replacement password are required")
	}
	return validateDatabaseResourcePayload(payload.Type, payload.Port, payload.ConnectionTimeoutSeconds)
}

func (payload *createDatabaseBindingPayload) Validate(*http.Request) error {
	if payload.DatabaseResourceID <= 0 {
		return errors.New("database resource is required")
	}
	return nil
}

func (payload *updateDatabaseBindingPayload) Validate(*http.Request) error {
	if err := validateResourceVersion(payload.ResourceVersion); err != nil {
		return err
	}
	if payload.DatabaseResourceID <= 0 {
		return errors.New("database resource is required")
	}
	return nil
}

func (handler *Handler) databaseResourceList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionView); handlerErr != nil {
		return handlerErr
	}
	resources, err := handler.DataStore.PlatformDatabaseResource().ReadAll(func(resource portainer.PlatformDatabaseResource) bool {
		return resource.ProjectID == portainer.PlatformProjectID(projectID) && (includeArchived(r) || isActive(resource.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}
	visible := make([]portainer.PlatformDatabaseResource, 0, len(resources))
	for _, resource := range resources {
		if handler.requireEndpointAccess(r, resource.EndpointID) == nil {
			visible = append(visible, redactDatabaseResource(resource))
		}
	}
	return response.JSON(w, visible)
}

func (handler *Handler) databaseResourceCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	projectID, handlerErr := handler.routeID(r, "projectId")
	if handlerErr != nil {
		return handlerErr
	}
	if _, handlerErr = handler.requireProjectPermission(r, portainer.PlatformProjectID(projectID), platformPermissionManage); handlerErr != nil {
		return handlerErr
	}
	var payload createDatabaseResourcePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	environment, handlerErr := handler.requireEnvironmentPermission(r, payload.EnvironmentID, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if environment.ProjectID != portainer.PlatformProjectID(projectID) || !environmentHasEndpoint(*environment, payload.EndpointID) {
		return platformAccessDenied()
	}
	if handlerErr = handler.requireEndpointAccess(r, payload.EndpointID); handlerErr != nil {
		return handlerErr
	}

	resource := portainer.NewPlatformDatabaseResource()
	resource.ProjectID = portainer.PlatformProjectID(projectID)
	resource.EnvironmentID = environment.ID
	resource.EndpointID = payload.EndpointID
	applyDatabaseResourcePayload(&resource, payload.Name, payload.Type, payload.Host, payload.Port, payload.Database, payload.Username, payload.ConnectionTimeoutSeconds)
	resource.PlatformLifecycle = newLifecycle(time.Now().Unix())
	if err := handler.encryptDatabaseResourcePassword(&resource, *payload.Password, nil); err != nil {
		return validationFailedError("Encrypted platform secret storage is unavailable")
	}

	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		existing, err := tx.PlatformDatabaseResource().ReadAll(func(item portainer.PlatformDatabaseResource) bool {
			return item.EnvironmentID == resource.EnvironmentID && isActive(item.PlatformLifecycle) && strings.EqualFold(item.Name, resource.Name)
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return duplicateError("database resource name already exists in environment")
		}
		if err := tx.PlatformDatabaseResource().Create(&resource); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseResourceCreated, Result: portainer.PlatformAuditResultSuccess, ProjectID: resource.ProjectID, EnvironmentID: resource.EnvironmentID, AfterSummary: databaseResourceAuditSummary(resource), SensitiveFields: []string{"DATABASE_PASSWORD"}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSONWithStatus(w, redactDatabaseResource(resource), http.StatusCreated)
}

func (handler *Handler) databaseResourceInspect(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	resource, handlerErr := handler.databaseResourceFromRequest(r, platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}
	return response.JSON(w, redactDatabaseResource(*resource))
}

func (handler *Handler) databaseResourceUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	resource, handlerErr := handler.databaseResourceFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	var payload updateDatabaseResourcePayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	var updated *portainer.PlatformDatabaseResource
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformDatabaseResource().Read(resource.ID)
		if err != nil {
			return err
		}
		if !isActive(current.PlatformLifecycle) {
			return notFoundError("Database resource is archived")
		}
		if err := requireResourceVersion(current.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}
		before := *current
		applyDatabaseResourcePayload(current, payload.Name, payload.Type, payload.Host, payload.Port, payload.Database, payload.Username, payload.ConnectionTimeoutSeconds)
		if payload.Password != nil {
			if err := handler.encryptDatabaseResourcePassword(current, *payload.Password, nil); err != nil {
				return validationFailedError("Encrypted platform secret storage is unavailable")
			}
		}
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformDatabaseResource().Update(current.ID, current); err != nil {
			return err
		}
		if err := handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseResourceUpdated, Result: portainer.PlatformAuditResultSuccess, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID, BeforeSummary: databaseResourceAuditSummary(before), AfterSummary: databaseResourceAuditSummary(*current), SensitiveFields: []string{"DATABASE_PASSWORD"}}); err != nil {
			return err
		}
		updated = current
		return nil
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, redactDatabaseResource(*updated))
}

func (handler *Handler) databaseResourceArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	resource, handlerErr := handler.databaseResourceFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformDatabaseResource().Read(resource.ID)
		if err != nil {
			return err
		}
		if !isActive(current.PlatformLifecycle) {
			return notFoundError("Database resource is archived")
		}
		bindings, err := tx.PlatformServiceDatabaseBinding().ReadAll(func(binding portainer.PlatformServiceDatabaseBinding) bool {
			return binding.DatabaseResourceID == current.ID && isActive(binding.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(bindings) > 0 {
			return conflictError("Database resource is referenced by an active service binding")
		}
		if err := ensureDatabaseResourceHasNoReleaseSnapshot(tx, current.ID); err != nil {
			return err
		}
		archiveLifecycle(&current.PlatformLifecycle, time.Now().Unix(), userID)
		if err := tx.PlatformDatabaseResource().Update(current.ID, current); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseResourceArchived, Result: portainer.PlatformAuditResultSuccess, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID, AfterSummary: map[string]any{"databaseResourceId": current.ID, "archived": true}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.Empty(w)
}

func (handler *Handler) databaseResourceTest(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	resource, handlerErr := handler.databaseResourceFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	if handler.DatabaseResourceProbe == nil {
		return writePlatformError(w, http.StatusServiceUnavailable, errPlatformUnsupportedOperation, "Database probe is unavailable", "DATABASE_PROBE_UNAVAILABLE", nil)
	}
	password, err := handler.decryptDatabaseResourcePassword(*resource)
	if err != nil {
		return writePlatformError(w, http.StatusBadRequest, errPlatformValidationFailed, "Database credential is unavailable", "DATABASE_CREDENTIAL_UNAVAILABLE", nil)
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(resource.ConnectionTimeoutSeconds)*time.Second)
	defer cancel()
	probeErr := handler.DatabaseResourceProbe.Probe(ctx, *resource, password)
	password = ""
	result := databaseResourceTestResponse{Status: "succeeded"}
	if probeErr != nil {
		result.Status = "failed"
		result.Reason = "DATABASE_UNREACHABLE"
		if ctx.Err() != nil {
			result.Reason = "DATABASE_TIMEOUT"
		}
	}
	// 外部连接已经结束后才开启写事务，只记录稳定结果，避免把网络等待时间持有在 BoltDB 事务内。
	if err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseResourceUpdated, Result: mapDatabaseProbeAuditResult(result.Status), ProjectID: resource.ProjectID, EnvironmentID: resource.EnvironmentID, AfterSummary: map[string]any{"databaseResourceId": resource.ID, "status": result.Status}, FailureReason: result.Reason})
	}); err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, result)
}

func (handler *Handler) serviceDatabaseBindingList(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	deployment, handlerErr := handler.serviceDeploymentFromBindingRequest(r, platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}
	bindings, err := handler.DataStore.PlatformServiceDatabaseBinding().ReadAll(func(binding portainer.PlatformServiceDatabaseBinding) bool {
		return binding.ServiceDeploymentID == deployment.ID && (includeArchived(r) || isActive(binding.PlatformLifecycle))
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSON(w, bindings)
}

func (handler *Handler) serviceDatabaseBindingCreate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	deployment, handlerErr := handler.serviceDeploymentFromBindingRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	var payload createDatabaseBindingPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	var binding portainer.PlatformServiceDatabaseBinding
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		resource, err := readActivePlatformDatabaseResource(tx, payload.DatabaseResourceID)
		if err != nil {
			return err
		}
		if !databaseResourceMatchesDeployment(*resource, *deployment) {
			return platformAccessDenied()
		}
		existing, err := tx.PlatformServiceDatabaseBinding().ReadAll(func(item portainer.PlatformServiceDatabaseBinding) bool {
			return item.ServiceDeploymentID == deployment.ID && isActive(item.PlatformLifecycle)
		})
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return conflictError("Service deployment already has an active database binding")
		}
		binding = portainer.NewPlatformServiceDatabaseBinding()
		binding.ProjectID, binding.EnvironmentID, binding.ServiceDeploymentID, binding.DatabaseResourceID = deployment.ProjectID, deployment.EnvironmentID, deployment.ID, resource.ID
		binding.PlatformLifecycle = newLifecycle(time.Now().Unix())
		if err := tx.PlatformServiceDatabaseBinding().Create(&binding); err != nil {
			return err
		}
		markDatabaseBindingDrift(deployment)
		if err := tx.PlatformServiceDeployment().Update(deployment.ID, deployment); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseBindingCreated, Result: portainer.PlatformAuditResultSuccess, ProjectID: binding.ProjectID, EnvironmentID: binding.EnvironmentID, ServiceDeploymentID: binding.ServiceDeploymentID, AfterSummary: map[string]any{"databaseBindingId": binding.ID, "databaseResourceId": binding.DatabaseResourceID}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.JSONWithStatus(w, binding, http.StatusCreated)
}

func (handler *Handler) serviceDatabaseBindingUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	binding, handlerErr := handler.databaseBindingFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	var payload updateDatabaseBindingPayload
	if err := request.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return validationFailed(err)
	}
	var updated *portainer.PlatformServiceDatabaseBinding
	err := handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformServiceDatabaseBinding().Read(binding.ID)
		if err != nil {
			return err
		}
		if !isActive(current.PlatformLifecycle) {
			return notFoundError("Database binding is archived")
		}
		if err := requireResourceVersion(current.ResourceVersion, payload.ResourceVersion); err != nil {
			return err
		}
		deployment, err := readActiveServiceDeployment(tx, current.ServiceDeploymentID)
		if err != nil {
			return err
		}
		resource, err := readActivePlatformDatabaseResource(tx, payload.DatabaseResourceID)
		if err != nil {
			return err
		}
		if !databaseResourceMatchesDeployment(*resource, *deployment) {
			return platformAccessDenied()
		}
		before := *current
		current.DatabaseResourceID = resource.ID
		touchLifecycle(&current.PlatformLifecycle, time.Now().Unix())
		if err := tx.PlatformServiceDatabaseBinding().Update(current.ID, current); err != nil {
			return err
		}
		markDatabaseBindingDrift(deployment)
		if err := tx.PlatformServiceDeployment().Update(deployment.ID, deployment); err != nil {
			return err
		}
		if err := handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseBindingUpdated, Result: portainer.PlatformAuditResultSuccess, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID, ServiceDeploymentID: current.ServiceDeploymentID, BeforeSummary: map[string]any{"databaseBindingId": before.ID, "databaseResourceId": before.DatabaseResourceID}, AfterSummary: map[string]any{"databaseBindingId": current.ID, "databaseResourceId": current.DatabaseResourceID}}); err != nil {
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

func (handler *Handler) serviceDatabaseBindingArchive(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	binding, handlerErr := handler.databaseBindingFromRequest(r, platformPermissionManage)
	if handlerErr != nil {
		return handlerErr
	}
	userID, err := currentUserID(r)
	if err != nil {
		return handler.convertError(err)
	}
	err = handler.DataStore.UpdateTx(func(tx dataservices.DataStoreTx) error {
		current, err := tx.PlatformServiceDatabaseBinding().Read(binding.ID)
		if err != nil {
			return err
		}
		if !isActive(current.PlatformLifecycle) {
			return notFoundError("Database binding is archived")
		}
		deployment, err := readActiveServiceDeployment(tx, current.ServiceDeploymentID)
		if err != nil {
			return err
		}
		archiveLifecycle(&current.PlatformLifecycle, time.Now().Unix(), userID)
		if err := tx.PlatformServiceDatabaseBinding().Update(current.ID, current); err != nil {
			return err
		}
		markDatabaseBindingDrift(deployment)
		if err := tx.PlatformServiceDeployment().Update(deployment.ID, deployment); err != nil {
			return err
		}
		return handler.createPlatformAuditLog(tx, r, &portainer.PlatformAuditLog{Action: portainer.PlatformAuditActionDatabaseBindingArchived, Result: portainer.PlatformAuditResultSuccess, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID, ServiceDeploymentID: current.ServiceDeploymentID, AfterSummary: map[string]any{"databaseBindingId": current.ID, "archived": true}})
	})
	if err != nil {
		return handler.convertError(err)
	}
	return response.Empty(w)
}

func (handler *Handler) serviceDatabaseWorkbenchContext(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	deployment, handlerErr := handler.serviceDeploymentFromBindingRequest(r, platformPermissionView)
	if handlerErr != nil {
		return handlerErr
	}
	bindings, err := handler.DataStore.PlatformServiceDatabaseBinding().ReadAll(func(binding portainer.PlatformServiceDatabaseBinding) bool {
		return binding.ServiceDeploymentID == deployment.ID && isActive(binding.PlatformLifecycle)
	})
	if err != nil {
		return handler.convertError(err)
	}
	if len(bindings) != 1 {
		return notFoundError("Active database binding is unavailable")
	}
	resource, err := handler.DataStore.PlatformDatabaseResource().Read(bindings[0].DatabaseResourceID)
	if err != nil || !isActive(resource.PlatformLifecycle) {
		return notFoundError("Database resource is unavailable")
	}
	if handlerErr = handler.requireEndpointAccess(r, resource.EndpointID); handlerErr != nil {
		return handlerErr
	}
	return response.JSON(w, databaseWorkbenchContext{DatabaseResourceID: resource.ID, EndpointID: resource.EndpointID, Type: resource.Type, Host: resource.Host, Port: resource.Port, Database: resource.Database, Username: resource.Username})
}

func (handler *Handler) databaseResourceFromRequest(r *http.Request, permission platformPermission) (*portainer.PlatformDatabaseResource, *httperror.HandlerError) {
	id, handlerErr := handler.routeID(r, "databaseResourceId")
	if handlerErr != nil {
		return nil, handlerErr
	}
	resource, err := handler.DataStore.PlatformDatabaseResource().Read(portainer.PlatformDatabaseResourceID(id))
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr = handler.requireProjectPermission(r, resource.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}
	if handlerErr = handler.requireEndpointAccess(r, resource.EndpointID); handlerErr != nil {
		return nil, handlerErr
	}
	return resource, nil
}

func (handler *Handler) databaseBindingFromRequest(r *http.Request, permission platformPermission) (*portainer.PlatformServiceDatabaseBinding, *httperror.HandlerError) {
	id, handlerErr := handler.routeID(r, "databaseBindingId")
	if handlerErr != nil {
		return nil, handlerErr
	}
	binding, err := handler.DataStore.PlatformServiceDatabaseBinding().Read(portainer.PlatformServiceDatabaseBindingID(id))
	if err != nil {
		return nil, handler.convertError(err)
	}
	if _, handlerErr = handler.requireProjectPermission(r, binding.ProjectID, permission); handlerErr != nil {
		return nil, handlerErr
	}
	resource, err := handler.DataStore.PlatformDatabaseResource().Read(binding.DatabaseResourceID)
	if err != nil || (handler.requireEndpointAccess(r, resource.EndpointID) != nil) {
		return nil, platformAccessDenied()
	}
	return binding, nil
}

func (handler *Handler) serviceDeploymentFromBindingRequest(r *http.Request, permission platformPermission) (*portainer.PlatformServiceDeployment, *httperror.HandlerError) {
	id, handlerErr := handler.routeID(r, "deploymentId")
	if handlerErr != nil {
		return nil, handlerErr
	}
	deployment, handlerErr := handler.requireServiceDeploymentPermission(r, portainer.PlatformServiceDeploymentID(id), permission)
	if handlerErr != nil {
		return nil, handlerErr
	}
	environment, err := handler.DataStore.PlatformEnvironment().Read(deployment.EnvironmentID)
	if err != nil {
		return nil, handler.convertError(err)
	}
	if handlerErr = handler.requireEnvironmentEndpointAccess(r, environment); handlerErr != nil {
		return nil, handlerErr
	}
	return deployment, nil
}

func (handler *Handler) encryptDatabaseResourcePassword(resource *portainer.PlatformDatabaseResource, password string, _ *portainer.PlatformDatabaseResource) error {
	cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
	if err != nil {
		return err
	}
	cipherText, hash, err := cipher.Encrypt(password)
	if err != nil {
		return err
	}
	resource.PasswordCipherText = cipherText
	resource.CredentialEncryptionVersion = portainer.PlatformDatabaseCredentialEncryptionVersion
	resource.CredentialHash = hash
	resource.HasPassword = true
	return nil
}

func (handler *Handler) decryptDatabaseResourcePassword(resource portainer.PlatformDatabaseResource) (string, error) {
	if !resource.HasPassword || resource.PasswordCipherText == "" || resource.CredentialEncryptionVersion != portainer.PlatformDatabaseCredentialEncryptionVersion {
		return "", errors.New("database credential is unavailable")
	}
	cipher, err := platformservice.NewSecretCipher(handler.DataStore.Connection())
	if err != nil {
		return "", err
	}
	return cipher.Decrypt(resource.PasswordCipherText)
}

func normalizeDatabaseResourcePayload(payload *createDatabaseResourcePayload) {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Host = strings.TrimSpace(payload.Host)
	payload.Database = strings.TrimSpace(payload.Database)
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Port == 0 {
		payload.Port = defaultDatabaseResourcePort(payload.Type)
	}
	if payload.ConnectionTimeoutSeconds == 0 {
		payload.ConnectionTimeoutSeconds = 5
	}
}

func normalizeDatabaseResourceUpdatePayload(payload *updateDatabaseResourcePayload) {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Host = strings.TrimSpace(payload.Host)
	payload.Database = strings.TrimSpace(payload.Database)
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Port == 0 {
		payload.Port = defaultDatabaseResourcePort(payload.Type)
	}
	if payload.ConnectionTimeoutSeconds == 0 {
		payload.ConnectionTimeoutSeconds = 5
	}
}

func validateDatabaseResourcePayload(resourceType portainer.PlatformDatabaseType, port, timeout int) error {
	if resourceType != portainer.PlatformDatabaseTypeMySQL && resourceType != portainer.PlatformDatabaseTypeMariaDB && resourceType != portainer.PlatformDatabaseTypePostgres && resourceType != portainer.PlatformDatabaseTypeRedis {
		return errors.New("unsupported database type")
	}
	if port < 1 || port > 65535 || timeout < 1 || timeout > 30 {
		return errors.New("database connection settings are invalid")
	}
	return nil
}

func defaultDatabaseResourcePort(resourceType portainer.PlatformDatabaseType) int {
	switch resourceType {
	case portainer.PlatformDatabaseTypeMySQL, portainer.PlatformDatabaseTypeMariaDB:
		return 3306
	case portainer.PlatformDatabaseTypePostgres:
		return 5432
	case portainer.PlatformDatabaseTypeRedis:
		return 6379
	default:
		return 0
	}
}

func applyDatabaseResourcePayload(resource *portainer.PlatformDatabaseResource, name string, resourceType portainer.PlatformDatabaseType, host string, port int, database, username string, timeout int) {
	resource.Name, resource.Type, resource.Host, resource.Port = name, resourceType, host, port
	resource.Database, resource.Username, resource.ConnectionTimeoutSeconds = database, username, timeout
}

func environmentHasEndpoint(environment portainer.PlatformEnvironment, endpointID portainer.EndpointID) bool {
	for _, target := range environment.Targets {
		if target.EndpointID == endpointID && target.Enabled && target.Role == portainer.PlatformDeploymentTargetRoleWorkload {
			return true
		}
	}
	return false
}

func readActivePlatformDatabaseResource(tx dataservices.DataStoreTx, id portainer.PlatformDatabaseResourceID) (*portainer.PlatformDatabaseResource, error) {
	resource, err := tx.PlatformDatabaseResource().Read(id)
	if err != nil {
		return nil, err
	}
	if !isActive(resource.PlatformLifecycle) {
		return nil, notFoundError("Database resource is archived")
	}
	return resource, nil
}

func databaseResourceMatchesDeployment(resource portainer.PlatformDatabaseResource, deployment portainer.PlatformServiceDeployment) bool {
	return resource.ProjectID == deployment.ProjectID && resource.EnvironmentID == deployment.EnvironmentID
}

func markDatabaseBindingDrift(deployment *portainer.PlatformServiceDeployment) {
	if deployment != nil && deployment.LastDeployedSpecRevision > 0 {
		deployment.DriftStatus = portainer.PlatformDeploymentDriftConfigChanged
	}
}

func redactDatabaseResource(resource portainer.PlatformDatabaseResource) portainer.PlatformDatabaseResource {
	resource.PasswordCipherText = ""
	resource.CredentialEncryptionVersion = ""
	resource.CredentialHash = ""
	return resource
}

func databaseResourceAuditSummary(resource portainer.PlatformDatabaseResource) map[string]any {
	return map[string]any{"databaseResourceId": resource.ID, "name": resource.Name, "type": resource.Type, "endpointId": resource.EndpointID, "hasPassword": resource.HasPassword, "revision": resource.Revision}
}

func mapDatabaseProbeAuditResult(status string) portainer.PlatformAuditResult {
	if status == "succeeded" {
		return portainer.PlatformAuditResultSuccess
	}
	return portainer.PlatformAuditResultFailed
}

func ensureDatabaseResourceHasNoReleaseSnapshot(tx dataservices.DataStoreTx, resourceID portainer.PlatformDatabaseResourceID) error {
	releases, err := tx.PlatformRelease().ReadAll(func(release portainer.PlatformRelease) bool {
		for _, snapshot := range release.ConfigSnapshot.DatabaseBindings {
			if snapshot.DatabaseResourceID == resourceID {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	if len(releases) > 0 {
		return conflictError("Database resource is referenced by a historical release snapshot")
	}
	return nil
}

func validateDatabaseBindingSnapshots(tx dataservices.DataStoreTx, snapshots []portainer.PlatformDatabaseBindingSnapshot) error {
	for _, snapshot := range snapshots {
		resource, err := readActivePlatformDatabaseResource(tx, snapshot.DatabaseResourceID)
		if err != nil || resource.Revision != snapshot.ResourceRevision || resource.Type != snapshot.Type || !resource.HasPassword || snapshot.VariableHash != platformservice.DatabaseBindingVariableHash(*resource) {
			return validationFailedError("Historical release database binding snapshot is unavailable")
		}
	}
	return nil
}

// databaseBindingSnapshotsForDeployment 把数据库绑定冻结为无密文的 Release 事实。资源
// 版本和变量哈希会在 Docker 注入前再次比对，防止资源更新后把新连接静默带入已排队发布。
func databaseBindingSnapshotsForDeployment(tx dataservices.DataStoreTx, deployment portainer.PlatformServiceDeployment, effectiveConfig portainer.PlatformEffectiveConfigSnapshot) ([]portainer.PlatformDatabaseBindingSnapshot, error) {
	bindings, err := tx.PlatformServiceDatabaseBinding().ReadAll(func(binding portainer.PlatformServiceDatabaseBinding) bool {
		return binding.ServiceDeploymentID == deployment.ID && isActive(binding.PlatformLifecycle)
	})
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	if err := validateDatabaseConfigConflicts(effectiveConfig); err != nil {
		return nil, err
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	snapshots := make([]portainer.PlatformDatabaseBindingSnapshot, 0, len(bindings))
	for _, binding := range bindings {
		resource, err := readActivePlatformDatabaseResource(tx, binding.DatabaseResourceID)
		if err != nil {
			return nil, err
		}
		if !databaseResourceMatchesDeployment(*resource, deployment) || !resource.HasPassword {
			return nil, validationFailedError("Database binding is unavailable for this deployment")
		}
		snapshots = append(snapshots, portainer.PlatformDatabaseBindingSnapshot{BindingID: binding.ID, BindingRevision: binding.Revision, DatabaseResourceID: resource.ID, ResourceRevision: resource.Revision, Type: resource.Type, VariableHash: platformservice.DatabaseBindingVariableHash(*resource)})
	}
	return snapshots, nil
}

func validateDatabaseConfigConflicts(effectiveConfig portainer.PlatformEffectiveConfigSnapshot) error {
	if len(effectiveConfig.Entries) == 0 {
		return nil
	}
	reserved := map[string]struct{}{
		"DATABASE_HOST": {}, "DATABASE_PORT": {}, "DATABASE_USER": {}, "DATABASE_PASSWORD": {}, "DATABASE_NAME": {}, "DATABASE_URL": {},
	}
	for _, entry := range effectiveConfig.Entries {
		if _, found := reserved[entry.Key]; found {
			return validationFailedError("Database binding variables cannot be overridden by ConfigSet or EnvOverrides")
		}
	}
	return nil
}

// preflightServiceDatabaseBindings 在 Release 持久化前完成短时数据库认证探测。它不持有
// BoltDB 事务；之后事务中的版本快照和运行期二次比对共同处理并发修改，避免网络等待阻塞控制面。
func (handler *Handler) preflightServiceDatabaseBindings(ctx context.Context, deploymentID portainer.PlatformServiceDeploymentID) string {
	bindings, err := handler.DataStore.PlatformServiceDatabaseBinding().ReadAll(func(binding portainer.PlatformServiceDatabaseBinding) bool {
		return binding.ServiceDeploymentID == deploymentID && isActive(binding.PlatformLifecycle)
	})
	if err != nil {
		return "DATABASE_BINDING_UNAVAILABLE"
	}
	if len(bindings) == 0 {
		return ""
	}
	if handler.DatabaseResourceProbe == nil {
		return "DATABASE_PROBE_UNAVAILABLE"
	}
	for _, binding := range bindings {
		resource, err := handler.DataStore.PlatformDatabaseResource().Read(binding.DatabaseResourceID)
		if err != nil || !isActive(resource.PlatformLifecycle) || !resource.HasPassword {
			return "DATABASE_BINDING_UNAVAILABLE"
		}
		password, err := handler.decryptDatabaseResourcePassword(*resource)
		if err != nil {
			return "DATABASE_CREDENTIAL_UNAVAILABLE"
		}
		probeCtx, cancel := context.WithTimeout(ctx, time.Duration(resource.ConnectionTimeoutSeconds)*time.Second)
		err = handler.DatabaseResourceProbe.Probe(probeCtx, *resource, password)
		password = ""
		timedOut := probeCtx.Err() != nil
		cancel()
		if err != nil {
			if timedOut {
				return "DATABASE_TIMEOUT"
			}
			return "DATABASE_UNREACHABLE"
		}
	}
	return ""
}
