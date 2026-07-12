package datastore

import (
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
)

type StoreTx struct {
	store *Store
	tx    portainer.Transaction
}

func (tx *StoreTx) IsErrObjectNotFound(err error) bool {
	return tx.store.IsErrObjectNotFound(err)
}

func (tx *StoreTx) AllowList() dataservices.AllowListService {
	return tx.store.AllowListService.Tx(tx.tx)
}

func (tx *StoreTx) CustomTemplate() dataservices.CustomTemplateService {
	return tx.store.CustomTemplateService.Tx(tx.tx)
}

func (tx *StoreTx) DatabaseConnection() dataservices.DatabaseConnectionService {
	return tx.store.DatabaseConnectionService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformProject() dataservices.PlatformProjectService {
	return tx.store.PlatformProjectService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformEnvironment() dataservices.PlatformEnvironmentService {
	return tx.store.PlatformEnvironmentService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformApplication() dataservices.PlatformApplicationService {
	return tx.store.PlatformApplicationService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformServiceDefinition() dataservices.PlatformServiceDefinitionService {
	return tx.store.PlatformServiceDefinitionService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformServiceDeployment() dataservices.PlatformServiceDeploymentService {
	return tx.store.PlatformServiceDeploymentService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformConfigSet() dataservices.PlatformConfigSetService {
	return tx.store.PlatformConfigSetService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformArtifact() dataservices.PlatformArtifactService {
	return tx.store.PlatformArtifactService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformArtifactStorage() dataservices.PlatformArtifactStorageService {
	return tx.store.PlatformArtifactStorageService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformHostGroup() dataservices.PlatformHostGroupService {
	return tx.store.PlatformHostGroupService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformGateway() dataservices.PlatformGatewayService {
	return tx.store.PlatformGatewayService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformGatewayRoute() dataservices.PlatformGatewayRouteService {
	return tx.store.PlatformGatewayRouteService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformGatewayCertificate() dataservices.PlatformGatewayCertificateService {
	return tx.store.PlatformGatewayCertificateService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformGatewayConfigVersion() dataservices.PlatformGatewayConfigVersionService {
	return tx.store.PlatformGatewayConfigVersionService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformRelease() dataservices.PlatformReleaseService {
	return tx.store.PlatformReleaseService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformReleaseLock() dataservices.PlatformReleaseLockService {
	return tx.store.PlatformReleaseLockService.Tx(tx.tx)
}

func (tx *StoreTx) PlatformAuditLog() dataservices.PlatformAuditLogService {
	return tx.store.PlatformAuditLogService.Tx(tx.tx)
}

func (tx *StoreTx) PendingActions() dataservices.PendingActionsService {
	return tx.store.PendingActionsService.Tx(tx.tx)
}

func (tx *StoreTx) EdgeGroup() dataservices.EdgeGroupService {
	return tx.store.EdgeGroupService.Tx(tx.tx)
}

func (tx *StoreTx) EdgeJob() dataservices.EdgeJobService {
	return tx.store.EdgeJobService.Tx(tx.tx)
}

func (tx *StoreTx) EdgeStack() dataservices.EdgeStackService {
	return tx.store.EdgeStackService.Tx(tx.tx)
}

func (tx *StoreTx) EdgeStackStatus() dataservices.EdgeStackStatusService {
	return tx.store.EdgeStackStatusService.Tx(tx.tx)
}

func (tx *StoreTx) Endpoint() dataservices.EndpointService {
	return tx.store.EndpointService.Tx(tx.tx)
}

func (tx *StoreTx) EndpointGroup() dataservices.EndpointGroupService {
	return tx.store.EndpointGroupService.Tx(tx.tx)
}

func (tx *StoreTx) EndpointRelation() dataservices.EndpointRelationService {
	return tx.store.EndpointRelationService.Tx(tx.tx)
}

func (tx *StoreTx) HelmUserRepository() dataservices.HelmUserRepositoryService { return nil }

func (tx *StoreTx) Registry() dataservices.RegistryService {
	return tx.store.RegistryService.Tx(tx.tx)
}

func (tx *StoreTx) ResourceControl() dataservices.ResourceControlService {
	return tx.store.ResourceControlService.Tx(tx.tx)
}

func (tx *StoreTx) Role() dataservices.RoleService {
	return tx.store.RoleService.Tx(tx.tx)
}

func (tx *StoreTx) APIKeyRepository() dataservices.APIKeyRepository { return nil }

func (tx *StoreTx) Settings() dataservices.SettingsService {
	return tx.store.SettingsService.Tx(tx.tx)
}

func (tx *StoreTx) Snapshot() dataservices.SnapshotService {
	return tx.store.SnapshotService.Tx(tx.tx)
}

func (tx *StoreTx) Source() dataservices.SourceService {
	return tx.store.SourceService.Tx(tx.tx)
}

func (tx *StoreTx) SSLSettings() dataservices.SSLSettingsService {
	return tx.store.SSLSettingsService.Tx(tx.tx)
}

func (tx *StoreTx) Stack() dataservices.StackService {
	return tx.store.StackService.Tx(tx.tx)
}

func (tx *StoreTx) Tag() dataservices.TagService {
	return tx.store.TagService.Tx(tx.tx)
}

func (tx *StoreTx) TeamMembership() dataservices.TeamMembershipService {
	return tx.store.TeamMembershipService.Tx(tx.tx)
}

func (tx *StoreTx) Team() dataservices.TeamService {
	return tx.store.TeamService.Tx(tx.tx)
}

func (tx *StoreTx) TunnelServer() dataservices.TunnelServerService { return nil }

func (tx *StoreTx) User() dataservices.UserService {
	return tx.store.UserService.Tx(tx.tx)
}

func (tx *StoreTx) UserActivityLog() dataservices.UserActivityLogService {
	return tx.store.UserActivityLogService.Tx(tx.tx)
}

func (tx *StoreTx) UserAuthenticationLog() dataservices.UserAuthenticationLogService {
	return tx.store.UserAuthenticationLogService.Tx(tx.tx)
}

func (tx *StoreTx) Version() dataservices.VersionService { return nil }
func (tx *StoreTx) Webhook() dataservices.WebhookService { return nil }

func (tx *StoreTx) Workflow() dataservices.WorkflowService {
	return tx.store.WorkflowService.Tx(tx.tx)
}
