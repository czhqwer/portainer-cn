package datastore

import (
	"path/filepath"
	"testing"

	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/dataservices"
	"github.com/stretchr/testify/require"
)

func TestPlatformDataServicesCRUDAndArchive(t *testing.T) {
	t.Parallel()
	_, store := MustNewTestStore(t, true, false)

	project := samplePlatformProject()
	require.NoError(t, store.PlatformProject().Create(project))
	require.NotZero(t, project.ID)
	gotProject, err := store.PlatformProject().Read(project.ID)
	require.NoError(t, err)
	require.Equal(t, project.Name, gotProject.Name)
	project.Description = "updated"
	project.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	project.ArchivedAt = 100
	require.NoError(t, store.PlatformProject().Update(project.ID, project))
	gotProject, err = store.PlatformProject().Read(project.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformLifecycleStatusArchived, gotProject.LifecycleStatus)

	environment := samplePlatformEnvironment(project.ID)
	require.NoError(t, store.PlatformEnvironment().Create(environment))
	gotEnvironment, err := store.PlatformEnvironment().Read(environment.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformTargetModeSingle, gotEnvironment.TargetMode)
	environment.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformEnvironment().Update(environment.ID, environment))

	application := samplePlatformApplication(project.ID)
	require.NoError(t, store.PlatformApplication().Create(application))
	gotApplication, err := store.PlatformApplication().Read(application.ID)
	require.NoError(t, err)
	require.Equal(t, application.Slug, gotApplication.Slug)
	application.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformApplication().Update(application.ID, application))

	definition := samplePlatformServiceDefinition(project.ID, application.ID)
	require.NoError(t, store.PlatformServiceDefinition().Create(definition))
	gotDefinition, err := store.PlatformServiceDefinition().Read(definition.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformServiceTypeBackend, gotDefinition.Type)
	definition.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformServiceDefinition().Update(definition.ID, definition))

	deployment := samplePlatformServiceDeployment(project.ID, environment.ID, application.ID, definition.ID)
	require.NoError(t, store.PlatformServiceDeployment().Create(deployment))
	gotDeployment, err := store.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, 1, gotDeployment.SpecRevision)
	deployment.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformServiceDeployment().Update(deployment.ID, deployment))

	configSet := samplePlatformConfigSet(project.ID)
	require.NoError(t, store.PlatformConfigSet().Create(configSet))
	gotConfigSet, err := store.PlatformConfigSet().Read(configSet.ID)
	require.NoError(t, err)
	require.Equal(t, 1, gotConfigSet.Revision)
	require.Equal(t, portainer.PlatformConfigEntrySourceProject, gotConfigSet.Entries[0].Source)
	configSet.Entries[0].Value = "test"
	require.NoError(t, store.PlatformConfigSet().Update(configSet.ID, configSet))
	gotConfigSet, err = store.PlatformConfigSet().Read(configSet.ID)
	require.NoError(t, err)
	require.Equal(t, 2, gotConfigSet.Revision)
	configSet.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformConfigSet().Update(configSet.ID, configSet))
	gotConfigSet, err = store.PlatformConfigSet().Read(configSet.ID)
	require.NoError(t, err)
	require.Equal(t, 2, gotConfigSet.Revision)

	artifact := samplePlatformArtifact(project.ID, application.ID, definition.ID)
	require.NoError(t, store.PlatformArtifact().Create(artifact))
	gotArtifact, err := store.PlatformArtifact().Read(artifact.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformArtifactSourceImageReference, gotArtifact.SourceType)
	artifact.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformArtifact().Update(artifact.ID, artifact))

	storage := samplePlatformArtifactStorage()
	require.NoError(t, store.PlatformArtifactStorage().Create(storage))
	gotStorage, err := store.PlatformArtifactStorage().Read(storage.ID)
	require.NoError(t, err)
	require.Equal(t, storage.Bucket, gotStorage.Bucket)
	storage.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformArtifactStorage().Update(storage.ID, storage))

	hostGroup := portainer.NewPlatformHostGroup()
	hostGroup.ProjectID = project.ID
	hostGroup.EnvironmentID = environment.ID
	hostGroup.Name = "production-a"
	hostGroup.Targets = []portainer.PlatformDeploymentTarget{{EndpointID: 1, NodeName: "node-a", HostAddress: "10.0.0.11", Role: portainer.PlatformDeploymentTargetRoleWorkload, Enabled: true}}
	require.NoError(t, store.PlatformHostGroup().Create(&hostGroup))
	gotHostGroup, err := store.PlatformHostGroup().Read(hostGroup.ID)
	require.NoError(t, err)
	require.Equal(t, hostGroup.Name, gotHostGroup.Name)
	hostGroup.LifecycleStatus = portainer.PlatformLifecycleStatusArchived
	require.NoError(t, store.PlatformHostGroup().Update(hostGroup.ID, &hostGroup))

	databaseResource := samplePlatformDatabaseResource(project.ID, environment.ID)
	require.NoError(t, store.PlatformDatabaseResource().Create(databaseResource))
	gotDatabaseResource, err := store.PlatformDatabaseResource().Read(databaseResource.ID)
	require.NoError(t, err)
	require.True(t, gotDatabaseResource.HasPassword)
	require.NotEmpty(t, gotDatabaseResource.PasswordCipherText)
	databaseResource.Host = "orders-db-v2.internal"
	require.NoError(t, store.PlatformDatabaseResource().Update(databaseResource.ID, databaseResource))
	gotDatabaseResource, err = store.PlatformDatabaseResource().Read(databaseResource.ID)
	require.NoError(t, err)
	require.Equal(t, 2, gotDatabaseResource.Revision)

	databaseBinding := portainer.NewPlatformServiceDatabaseBinding()
	databaseBinding.ProjectID = project.ID
	databaseBinding.EnvironmentID = environment.ID
	databaseBinding.ServiceDeploymentID = deployment.ID
	databaseBinding.DatabaseResourceID = databaseResource.ID
	require.NoError(t, store.PlatformServiceDatabaseBinding().Create(&databaseBinding))
	gotDatabaseBinding, err := store.PlatformServiceDatabaseBinding().Read(databaseBinding.ID)
	require.NoError(t, err)
	require.Equal(t, databaseResource.ID, gotDatabaseBinding.DatabaseResourceID)

	gateway := portainer.NewPlatformGateway()
	gateway.ProjectID = project.ID
	gateway.EnvironmentID = environment.ID
	gateway.EndpointID = 1
	gateway.Name = "demo-gateway"
	require.NoError(t, store.PlatformGateway().Create(&gateway))

	route := portainer.NewPlatformGatewayRoute()
	route.GatewayID = gateway.ID
	route.ProjectID = project.ID
	route.EnvironmentID = environment.ID
	route.ServiceDeploymentID = deployment.ID
	route.Domain = "api.example.test"
	route.Path = "/api"
	route.TargetPort = 8080
	require.NoError(t, store.PlatformGatewayRoute().Create(&route))

	certificate := portainer.NewPlatformGatewayCertificate()
	certificate.ProjectID = project.ID
	certificate.Name = "example-test"
	certificate.Domains = []string{"api.example.test"}
	require.NoError(t, store.PlatformGatewayCertificate().Create(&certificate))

	configVersion := portainer.PlatformGatewayConfigVersion{
		GatewayID:  gateway.ID,
		Revision:   1,
		Status:     portainer.PlatformGatewayConfigStatusCandidate,
		ConfigHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RouteIDs:   []portainer.PlatformGatewayRouteID{route.ID},
	}
	require.NoError(t, store.PlatformGatewayConfigVersion().Create(&configVersion))
	gotGateway, err := store.PlatformGateway().Read(gateway.ID)
	require.NoError(t, err)
	require.Equal(t, gateway.Name, gotGateway.Name)
	gotConfigVersion, err := store.PlatformGatewayConfigVersion().Read(configVersion.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformGatewayConfigStatusCandidate, gotConfigVersion.Status)

	release := samplePlatformRelease(project.ID, environment.ID, application.ID, definition.ID, deployment.ID, artifact.ID)
	require.NoError(t, store.PlatformRelease().Create(release))
	gotRelease, err := store.PlatformRelease().Read(release.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformReleaseStatusQueued, gotRelease.Status)
	release.Status = portainer.PlatformReleaseStatusFailed
	require.NoError(t, store.PlatformRelease().Update(release.ID, release))

	audit := samplePlatformAuditLog(project.ID, environment.ID, application.ID, definition.ID, deployment.ID, artifact.ID, release.ID)
	require.NoError(t, store.PlatformAuditLog().Create(audit))
	gotAudit, err := store.PlatformAuditLog().Read(audit.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformAuditActionReleaseCreated, gotAudit.Action)
}

func TestPlatformDataServicesTxAndReleaseLock(t *testing.T) {
	t.Parallel()
	_, store := MustNewTestStore(t, true, false)

	require.NoError(t, store.UpdateTx(func(tx dataservices.DataStoreTx) error {
		project := samplePlatformProject()
		if err := tx.PlatformProject().Create(project); err != nil {
			return err
		}

		environment := samplePlatformEnvironment(project.ID)
		if err := tx.PlatformEnvironment().Create(environment); err != nil {
			return err
		}

		deployment := samplePlatformServiceDeployment(project.ID, environment.ID, 0, 0)
		if err := tx.PlatformServiceDeployment().Create(deployment); err != nil {
			return err
		}
		if err := tx.PlatformConfigSet().Create(samplePlatformConfigSet(project.ID)); err != nil {
			return err
		}
		if err := tx.PlatformArtifactStorage().Create(samplePlatformArtifactStorage()); err != nil {
			return err
		}

		release := samplePlatformRelease(project.ID, environment.ID, 0, 0, deployment.ID, 0)
		if err := tx.PlatformRelease().Create(release); err != nil {
			return err
		}

		if err := tx.PlatformReleaseLock().Create(&portainer.PlatformReleaseLock{
			ServiceDeploymentID: deployment.ID,
			ReleaseID:           release.ID,
			IdempotencyKeyHash:  "idem-hash",
			PayloadHash:         "payload-hash",
		}); err != nil {
			return err
		}

		return tx.PlatformAuditLog().Create(samplePlatformAuditLog(project.ID, environment.ID, 0, 0, deployment.ID, 0, release.ID))
	}))

	var locks []portainer.PlatformReleaseLock
	require.NoError(t, store.ViewTx(func(tx dataservices.DataStoreTx) error {
		var err error
		locks, err = tx.PlatformReleaseLock().ReadAll(func(lock portainer.PlatformReleaseLock) bool {
			return lock.IdempotencyKeyHash == "idem-hash"
		})
		return err
	}))

	require.Len(t, locks, 1)
	require.Equal(t, "1", locks[0].ID)

	exists, err := store.PlatformReleaseLock().Exists(locks[0].ID)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestPlatformDataServicesExportImport(t *testing.T) {
	t.Parallel()
	_, store := MustNewTestStore(t, true, false)

	project := samplePlatformProject()
	require.NoError(t, store.PlatformProject().Create(project))
	environment := samplePlatformEnvironment(project.ID)
	require.NoError(t, store.PlatformEnvironment().Create(environment))
	application := samplePlatformApplication(project.ID)
	require.NoError(t, store.PlatformApplication().Create(application))
	definition := samplePlatformServiceDefinition(project.ID, application.ID)
	require.NoError(t, store.PlatformServiceDefinition().Create(definition))
	deployment := samplePlatformServiceDeployment(project.ID, environment.ID, application.ID, definition.ID)
	require.NoError(t, store.PlatformServiceDeployment().Create(deployment))
	configSet := samplePlatformConfigSet(project.ID)
	require.NoError(t, store.PlatformConfigSet().Create(configSet))
	artifact := samplePlatformArtifact(project.ID, application.ID, definition.ID)
	require.NoError(t, store.PlatformArtifact().Create(artifact))
	storage := samplePlatformArtifactStorage()
	require.NoError(t, store.PlatformArtifactStorage().Create(storage))
	databaseResource := samplePlatformDatabaseResource(project.ID, environment.ID)
	require.NoError(t, store.PlatformDatabaseResource().Create(databaseResource))
	databaseBinding := portainer.NewPlatformServiceDatabaseBinding()
	databaseBinding.ProjectID = project.ID
	databaseBinding.EnvironmentID = environment.ID
	databaseBinding.ServiceDeploymentID = deployment.ID
	databaseBinding.DatabaseResourceID = databaseResource.ID
	require.NoError(t, store.PlatformServiceDatabaseBinding().Create(&databaseBinding))
	gateway := portainer.NewPlatformGateway()
	gateway.ProjectID = project.ID
	gateway.EnvironmentID = environment.ID
	gateway.EndpointID = 1
	gateway.Name = "export-gateway"
	require.NoError(t, store.PlatformGateway().Create(&gateway))
	route := portainer.NewPlatformGatewayRoute()
	route.GatewayID = gateway.ID
	route.ProjectID = project.ID
	route.EnvironmentID = environment.ID
	route.ServiceDeploymentID = deployment.ID
	route.Domain = "export.example.test"
	route.TargetPort = 8080
	require.NoError(t, store.PlatformGatewayRoute().Create(&route))
	certificate := portainer.NewPlatformGatewayCertificate()
	certificate.ProjectID = project.ID
	certificate.Name = "export-certificate"
	certificate.Domains = []string{"export.example.test"}
	certificate.MaterialRef = "gateway-certificates/export-certificate"
	certificate.HasPrivateKey = true
	require.NoError(t, store.PlatformGatewayCertificate().Create(&certificate))
	configVersion := portainer.PlatformGatewayConfigVersion{
		GatewayID:  gateway.ID,
		Revision:   1,
		Status:     portainer.PlatformGatewayConfigStatusActive,
		ConfigHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RouteIDs:   []portainer.PlatformGatewayRouteID{route.ID},
	}
	require.NoError(t, store.PlatformGatewayConfigVersion().Create(&configVersion))
	release := samplePlatformRelease(project.ID, environment.ID, application.ID, definition.ID, deployment.ID, artifact.ID)
	require.NoError(t, store.PlatformRelease().Create(release))
	lock := &portainer.PlatformReleaseLock{
		ServiceDeploymentID: deployment.ID,
		ReleaseID:           release.ID,
		IdempotencyKeyHash:  "idem-hash",
		PayloadHash:         "payload-hash",
	}
	require.NoError(t, store.PlatformReleaseLock().Create(lock))
	audit := samplePlatformAuditLog(project.ID, environment.ID, application.ID, definition.ID, deployment.ID, artifact.ID, release.ID)
	require.NoError(t, store.PlatformAuditLog().Create(audit))

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	require.NoError(t, store.Export(backupFile))

	_, importedStore := MustNewTestStore(t, true, false)
	importedProject, err := importedStore.PlatformProject().Read(project.ID)
	require.Error(t, err)
	require.Nil(t, importedProject)

	require.NoError(t, importedStore.Import(backupFile))

	importedProject, err = importedStore.PlatformProject().Read(project.ID)
	require.NoError(t, err)
	require.Equal(t, project.Name, importedProject.Name)

	importedDeployment, err := importedStore.PlatformServiceDeployment().Read(deployment.ID)
	require.NoError(t, err)
	require.Equal(t, portainer.PlatformRuntimeDriverDockerContainer, importedDeployment.DesiredSpec.Runtime.RuntimeDriver)

	importedConfigSet, err := importedStore.PlatformConfigSet().Read(configSet.ID)
	require.NoError(t, err)
	require.Equal(t, configSet.Entries, importedConfigSet.Entries)

	importedStorage, err := importedStore.PlatformArtifactStorage().Read(storage.ID)
	require.NoError(t, err)
	require.Equal(t, storage.Bucket, importedStorage.Bucket)
	require.Empty(t, importedStorage.AccessKeyCipherText)
	require.Empty(t, importedStorage.SecretKeyCipherText)
	require.Empty(t, importedStorage.CredentialEncryptionVersion)
	require.Empty(t, importedStorage.CredentialHash)

	importedDatabaseResource, err := importedStore.PlatformDatabaseResource().Read(databaseResource.ID)
	require.NoError(t, err)
	require.Equal(t, databaseResource.Host, importedDatabaseResource.Host)
	require.False(t, importedDatabaseResource.HasPassword)
	require.Empty(t, importedDatabaseResource.PasswordCipherText)
	require.Empty(t, importedDatabaseResource.CredentialEncryptionVersion)
	require.Empty(t, importedDatabaseResource.CredentialHash)
	importedDatabaseBinding, err := importedStore.PlatformServiceDatabaseBinding().Read(databaseBinding.ID)
	require.NoError(t, err)
	require.Equal(t, databaseResource.ID, importedDatabaseBinding.DatabaseResourceID)

	importedGateway, err := importedStore.PlatformGateway().Read(gateway.ID)
	require.NoError(t, err)
	require.Equal(t, gateway.Name, importedGateway.Name)
	importedRoute, err := importedStore.PlatformGatewayRoute().Read(route.ID)
	require.NoError(t, err)
	require.Equal(t, route.Domain, importedRoute.Domain)
	importedCertificate, err := importedStore.PlatformGatewayCertificate().Read(certificate.ID)
	require.NoError(t, err)
	require.Empty(t, importedCertificate.MaterialRef)
	require.False(t, importedCertificate.HasPrivateKey)
	importedConfigVersion, err := importedStore.PlatformGatewayConfigVersion().Read(configVersion.ID)
	require.NoError(t, err)
	require.Equal(t, configVersion.ConfigHash, importedConfigVersion.ConfigHash)

	importedLock, err := importedStore.PlatformReleaseLock().Read(lock.ID)
	require.NoError(t, err)
	require.Equal(t, release.ID, importedLock.ReleaseID)

	importedAudit, err := importedStore.PlatformAuditLog().Read(audit.ID)
	require.NoError(t, err)
	require.Equal(t, release.ID, importedAudit.ReleaseID)
}

func TestPlatformObservabilityConfigExportStripsCredentials(t *testing.T) {
	t.Parallel()
	_, store := MustNewTestStore(t, true, false)

	config := portainer.NewPlatformObservabilityConfig()
	config.PrometheusURL = "https://prometheus.example.test"
	config.BearerTokenCipherText = "cipher"
	config.CredentialEncryptionVersion = portainer.PlatformObservabilityCredentialEncryptionVersion
	config.CredentialHash = "hash"
	config.HasCredentials = true
	require.NoError(t, store.PlatformObservabilityConfig().Create(&config))

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	require.NoError(t, store.Export(backupFile))
	_, importedStore := MustNewTestStore(t, true, false)
	require.NoError(t, importedStore.Import(backupFile))

	imported, err := importedStore.PlatformObservabilityConfig().Read(config.ID)
	require.NoError(t, err)
	require.False(t, imported.HasCredentials)
	require.Empty(t, imported.BearerTokenCipherText)
}

func TestPlatformCanaryPolicyCRUD(t *testing.T) {
	t.Parallel()
	_, store := MustNewTestStore(t, true, false)

	policy := portainer.NewPlatformCanaryPolicy()
	policy.ProjectID = 1
	policy.EnvironmentID = 1
	policy.GatewayRouteID = 1
	policy.StableReleaseID = 1
	policy.CanaryReleaseID = 2
	require.NoError(t, store.PlatformCanaryPolicy().Create(&policy))

	stored, err := store.PlatformCanaryPolicy().Read(policy.ID)
	require.NoError(t, err)
	require.Equal(t, 0, stored.CurrentWeight)
}

func samplePlatformProject() *portainer.PlatformProject {
	return &portainer.PlatformProject{
		Name:              "Demo",
		Slug:              "demo",
		Description:       "demo project",
		MemberPolicies:    map[portainer.UserID]portainer.PlatformProjectRole{},
		TeamPolicies:      map[portainer.TeamID]portainer.PlatformProjectRole{},
		PlatformLifecycle: portainer.NewPlatformLifecycle(),
	}
}

func samplePlatformEnvironment(projectID portainer.PlatformProjectID) *portainer.PlatformEnvironment {
	return &portainer.PlatformEnvironment{
		ProjectID:    projectID,
		Name:         "Production",
		Slug:         "prod",
		Type:         portainer.PlatformEnvironmentTypeProd,
		IsProduction: true,
		TargetMode:   portainer.PlatformTargetModeSingle,
		Targets: []portainer.PlatformDeploymentTarget{
			{
				EndpointID: 1,
				Role:       portainer.PlatformDeploymentTargetRoleWorkload,
				Enabled:    true,
			},
		},
		ReleasePolicy:     portainer.NewPlatformReleasePolicy(),
		PlatformLifecycle: portainer.NewPlatformLifecycle(),
	}
}

func samplePlatformApplication(projectID portainer.PlatformProjectID) *portainer.PlatformApplication {
	return &portainer.PlatformApplication{
		ProjectID:         projectID,
		Name:              "Order",
		Slug:              "order",
		PlatformLifecycle: portainer.NewPlatformLifecycle(),
	}
}

func samplePlatformServiceDefinition(projectID portainer.PlatformProjectID, applicationID portainer.PlatformApplicationID) *portainer.PlatformServiceDefinition {
	return &portainer.PlatformServiceDefinition{
		ProjectID:         projectID,
		ApplicationID:     applicationID,
		Name:              "Order API",
		Slug:              "order-api",
		Type:              portainer.PlatformServiceTypeBackend,
		PlatformLifecycle: portainer.NewPlatformLifecycle(),
	}
}

func samplePlatformServiceDeployment(
	projectID portainer.PlatformProjectID,
	environmentID portainer.PlatformEnvironmentID,
	applicationID portainer.PlatformApplicationID,
	definitionID portainer.PlatformServiceDefinitionID,
) *portainer.PlatformServiceDeployment {
	return &portainer.PlatformServiceDeployment{
		ProjectID:           projectID,
		EnvironmentID:       environmentID,
		ApplicationID:       applicationID,
		ServiceDefinitionID: definitionID,
		DesiredSpec:         samplePlatformDesiredSpec(),
		SpecRevision:        1,
		DriftStatus:         portainer.PlatformDeploymentDriftNone,
		PlatformLifecycle:   portainer.NewPlatformLifecycle(),
	}
}

func samplePlatformDesiredSpec() portainer.PlatformDeploymentDesiredSpec {
	spec := portainer.NewPlatformDeploymentDesiredSpec()
	spec.Image.Image = "registry.example.com/order-api:20260711"
	spec.Ports = []portainer.PlatformPortSpec{
		{ContainerPort: 8080, HostPort: 18080},
	}
	spec.HealthCheck.Port = 8080
	portainer.NormalizePlatformDeploymentDesiredSpec(&spec)
	return spec
}

func samplePlatformConfigSet(projectID portainer.PlatformProjectID) *portainer.PlatformConfigSet {
	configSet := portainer.NewPlatformConfigSet()
	configSet.ProjectID = projectID
	configSet.ScopeType = portainer.PlatformConfigScopeProject
	configSet.ScopeID = int(projectID)
	configSet.Entries = []portainer.PlatformConfigEntry{
		{
			Key:   "APP_ENV",
			Value: "production",
		},
	}
	return &configSet
}

func samplePlatformArtifact(
	projectID portainer.PlatformProjectID,
	applicationID portainer.PlatformApplicationID,
	definitionID portainer.PlatformServiceDefinitionID,
) *portainer.PlatformArtifact {
	return &portainer.PlatformArtifact{
		ProjectID:           projectID,
		ApplicationID:       applicationID,
		ServiceDefinitionID: definitionID,
		Name:                "order-api",
		Version:             "20260711",
		Type:                portainer.PlatformArtifactTypeImage,
		SourceType:          portainer.PlatformArtifactSourceImageReference,
		ImageRef:            "registry.example.com/order-api:20260711",
		Traceability:        portainer.PlatformTraceabilityWeak,
		PlatformLifecycle:   portainer.NewPlatformLifecycle(),
	}
}

func samplePlatformArtifactStorage() *portainer.PlatformArtifactStorage {
	storage := portainer.NewPlatformArtifactStorage()
	storage.Name = "delivery-minio"
	storage.Endpoint = "https://minio.example.com"
	storage.Region = "us-east-1"
	storage.Bucket = "artifacts"
	storage.PathPrefix = "releases"
	storage.AuthorizedProjectIDs = []portainer.PlatformProjectID{1}
	storage.AccessKeyCipherText = "encrypted-access-key"
	storage.SecretKeyCipherText = "encrypted-secret-key"
	storage.CredentialEncryptionVersion = portainer.PlatformArtifactStorageCredentialEncryptionVersion
	storage.CredentialHash = "credential-hash"
	return &storage
}

func samplePlatformDatabaseResource(projectID portainer.PlatformProjectID, environmentID portainer.PlatformEnvironmentID) *portainer.PlatformDatabaseResource {
	resource := portainer.NewPlatformDatabaseResource()
	resource.ProjectID = projectID
	resource.EnvironmentID = environmentID
	resource.EndpointID = 1
	resource.Name = "orders-db"
	resource.Type = portainer.PlatformDatabaseTypePostgres
	resource.Host = "orders-db.internal"
	resource.Port = 5432
	resource.Database = "orders"
	resource.Username = "orders_app"
	resource.PasswordCipherText = "encrypted-password"
	resource.CredentialEncryptionVersion = portainer.PlatformDatabaseCredentialEncryptionVersion
	resource.CredentialHash = "password-hash"
	resource.HasPassword = true
	return &resource
}

func samplePlatformRelease(
	projectID portainer.PlatformProjectID,
	environmentID portainer.PlatformEnvironmentID,
	applicationID portainer.PlatformApplicationID,
	definitionID portainer.PlatformServiceDefinitionID,
	deploymentID portainer.PlatformServiceDeploymentID,
	artifactID portainer.PlatformArtifactID,
) *portainer.PlatformRelease {
	return &portainer.PlatformRelease{
		ProjectID:            projectID,
		EnvironmentID:        environmentID,
		ApplicationID:        applicationID,
		ServiceDefinitionID:  definitionID,
		ServiceDeploymentID:  deploymentID,
		ArtifactID:           artifactID,
		Version:              "20260711",
		TriggerType:          portainer.PlatformReleaseTriggerDeploy,
		Strategy:             portainer.NewPlatformReleaseStrategy(),
		Status:               portainer.PlatformReleaseStatusQueued,
		ExpectedSpecRevision: 1,
		Image:                "registry.example.com/order-api:20260711",
		Traceability:         portainer.PlatformTraceabilityWeak,
		CreatedAt:            100,
		QueueExpiresAt:       700,
	}
}

func samplePlatformAuditLog(
	projectID portainer.PlatformProjectID,
	environmentID portainer.PlatformEnvironmentID,
	applicationID portainer.PlatformApplicationID,
	definitionID portainer.PlatformServiceDefinitionID,
	deploymentID portainer.PlatformServiceDeploymentID,
	artifactID portainer.PlatformArtifactID,
	releaseID portainer.PlatformReleaseID,
) *portainer.PlatformAuditLog {
	return &portainer.PlatformAuditLog{
		Timestamp:           100,
		OperatorUserID:      1,
		OperatorUsername:    "admin",
		Action:              portainer.PlatformAuditActionReleaseCreated,
		Result:              portainer.PlatformAuditResultSuccess,
		ProjectID:           projectID,
		EnvironmentID:       environmentID,
		ApplicationID:       applicationID,
		ServiceDefinitionID: definitionID,
		ServiceDeploymentID: deploymentID,
		ArtifactID:          artifactID,
		ReleaseID:           releaseID,
		AfterSummary:        map[string]any{"status": string(portainer.PlatformReleaseStatusQueued)},
	}
}
