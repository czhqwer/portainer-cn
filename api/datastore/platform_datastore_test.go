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

	importedLock, err := importedStore.PlatformReleaseLock().Read(lock.ID)
	require.NoError(t, err)
	require.Equal(t, release.ID, importedLock.ReleaseID)

	importedAudit, err := importedStore.PlatformAuditLog().Read(audit.ID)
	require.NoError(t, err)
	require.Equal(t, release.ID, importedAudit.ReleaseID)
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
