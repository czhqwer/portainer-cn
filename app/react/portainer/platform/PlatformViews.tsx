import { FormEvent, type ReactNode, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  FileText,
  Package,
  Plus,
  RefreshCw,
  RotateCcw,
  Rocket,
  Save,
  SlidersHorizontal,
  Wrench,
} from 'lucide-react';

import { Alert } from '@@/Alert';
import { Button } from '@@/buttons';
import { PageHeader } from '@@/PageHeader';

import { useCurrentUser } from '@/react/hooks/useUser';

import {
  useArchivePlatformConfigSetMutation,
  useCreateImageReferenceArtifactMutation,
  useCreatePlatformApplicationMutation,
  useCreatePlatformConfigSetMutation,
  useCreatePlatformEnvironmentMutation,
  useCreatePlatformProjectMutation,
  useCreatePlatformReleaseMutation,
  useCreatePlatformServiceDefinitionMutation,
  useCreatePlatformServiceDeploymentMutation,
  usePlatformApplications,
  usePlatformAuditLogs,
  usePlatformArtifacts,
  usePlatformConfigSets,
  usePlatformEffectiveConfig,
  usePlatformEnvironments,
  usePlatformProjects,
  usePlatformReleaseRollbackDiff,
  usePlatformReleases,
  usePlatformServiceDeploymentLogs,
  usePlatformServiceDeployments,
  usePlatformServiceDeploymentStatus,
  usePlatformServices,
  useReadPlatformConfigSecretMutation,
  useResolvePlatformReleaseMutation,
  useRollbackPlatformReleaseMutation,
  useUpdatePlatformConfigSetMutation,
  useUpdatePlatformServiceDeploymentMutation,
	useUploadPlatformArtifactMutation,
	useBuildJavaArtifactMutation,
	useBuildStaticArtifactMutation,
	usePushPlatformArtifactMutation,
	useCleanupPlatformArtifactOriginalMutation,
  useValidatePlatformReleaseMutation,
} from './queries';
import {
  CreatePlatformReleasePayload,
  PlatformApplication,
  PlatformAuditLog,
  PlatformArtifact,
  PlatformConfigEntry,
  PlatformConfigScopeType,
  PlatformConfigSet,
  PlatformDeploymentDesiredSpec,
  PlatformEffectiveConfigResponse,
  PlatformEnvironment,
  PlatformPublishedPort,
  PlatformProject,
  PlatformRelease,
  PlatformReleaseCreateResponse,
  PlatformReleaseRollbackDiff,
  PlatformReleaseValidateResponse,
  PlatformReleaseResolutionAction,
  PlatformRuntimeRef,
  PlatformServiceDeployment,
  PlatformServiceDefinition,
} from './types';

export function PlatformProjectsView() {
  const { t } = useTranslation();
  const { isPureAdmin } = useCurrentUser();
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];
  const [selectedProjectId, setSelectedProjectId] = useState<number>();
  const [isProjectFormOpen, setIsProjectFormOpen] = useState(false);
  const [isEnvironmentFormOpen, setIsEnvironmentFormOpen] = useState(false);
  const currentProject =
    projects.find((project) => project.Id === selectedProjectId) ?? projects[0];
  const canManageCurrentProject =
    !!currentProject?.Permissions?.CanManageResources;
  const canViewProjectAudit = !!currentProject?.Permissions?.CanManageProject;
  const environmentsQuery = usePlatformEnvironments(currentProject?.Id);
  const environments = environmentsQuery.data ?? [];
  const auditLogsQuery = usePlatformAuditLogs(
    currentProject?.Id,
    canViewProjectAudit
  );
  const auditLogs = auditLogsQuery.data ?? [];

  return (
    <PlatformPage
      titleKey="platform.pages.projects.title"
      titleDefault="Projects"
    >
      <PlatformNoticeStack />
      <ActionBar>
        {isPureAdmin && (
          <Button
            color="primary"
            icon={Plus}
            onClick={() => setIsProjectFormOpen((value) => !value)}
            data-cy="platform-create-project-open"
          >
            {t('platform.actions.createProject', {
              defaultValue: 'Create project',
            })}
          </Button>
        )}
        <Button
          color="light"
          icon={Plus}
          disabled={!currentProject || !canManageCurrentProject}
          onClick={() => setIsEnvironmentFormOpen((value) => !value)}
          data-cy="platform-create-environment-open"
        >
          {t('platform.actions.createEnvironment', {
            defaultValue: 'Create environment',
          })}
        </Button>
      </ActionBar>
      {isProjectFormOpen && (
        <CreateProjectPanel onDone={() => setIsProjectFormOpen(false)} />
      )}
      {isEnvironmentFormOpen && canManageCurrentProject && (
        <CreateEnvironmentPanel
          projects={projects}
          projectId={currentProject?.Id}
          onProjectChange={setSelectedProjectId}
          onDone={() => setIsEnvironmentFormOpen(false)}
        />
      )}
      <SummaryStrip
        items={[
          {
            label: t('platform.summary.projects', { defaultValue: 'Projects' }),
            value: projects.length,
          },
          {
            label: t('platform.summary.active', { defaultValue: 'Active' }),
            value: projects.filter(
              (project) => project.LifecycleStatus === 'active'
            ).length,
          },
          {
            label: t('platform.summary.archived', { defaultValue: 'Archived' }),
            value: projects.filter(
              (project) => project.LifecycleStatus === 'archived'
            ).length,
          },
        ]}
      />
      <div className="mx-4 mb-4 max-w-md">
        <SelectField
          label={t('platform.filters.project', { defaultValue: 'Project' })}
          value={currentProject?.Id}
          disabled={projects.length === 0}
          onChange={setSelectedProjectId}
        >
          {projects.map((project) => (
            <option key={project.Id} value={project.Id}>
              {project.Name}
            </option>
          ))}
        </SelectField>
      </div>
      <DataSection
        title={t('platform.projects.tableTitle', {
          defaultValue: 'Project list',
        })}
        isLoading={projectsQuery.isLoading}
        empty={projects.length === 0}
        emptyMessage={t('platform.empty.projects', {
          defaultValue:
            'No projects yet. Create a project to start the deployment flow.',
        })}
      >
        <PlatformTable
          columns={[
            t('platform.columns.name', { defaultValue: 'Name' }),
            t('platform.columns.slug', { defaultValue: 'Slug' }),
            t('platform.columns.status', { defaultValue: 'Status' }),
            t('platform.columns.resourceVersion', {
              defaultValue: 'Resource version',
            }),
          ]}
          rows={projects.map((project) => ({
            key: String(project.Id),
            cells: [
              project.Name,
              project.Slug,
              project.LifecycleStatus,
              String(project.ResourceVersion),
            ],
          }))}
        />
      </DataSection>
      <DataSection
        title={t('platform.environments.tableTitle', {
          defaultValue: 'Environments',
        })}
        isLoading={environmentsQuery.isLoading}
        empty={environments.length === 0}
        emptyMessage={t('platform.empty.environments', {
          defaultValue:
            'No environments yet. Create an environment for the selected project.',
        })}
      >
        <EnvironmentsTable environments={environments} />
      </DataSection>
      {canViewProjectAudit && (
        <DataSection
          title={t('platform.audit.tableTitle')}
          isLoading={auditLogsQuery.isLoading}
          empty={auditLogs.length === 0}
          emptyMessage={t('platform.audit.empty')}
        >
          <AuditLogsTable logs={auditLogs} />
        </DataSection>
      )}
    </PlatformPage>
  );
}

export function PlatformApplicationsView() {
  const { t } = useTranslation();
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];
  const [selectedProjectId, setSelectedProjectId] = useState<number>();
  const [selectedApplicationId, setSelectedApplicationId] = useState<number>();
  const [selectedServiceId, setSelectedServiceId] = useState<number>();
  const [selectedDeploymentId, setSelectedDeploymentId] = useState<number>();
  const [isApplicationFormOpen, setIsApplicationFormOpen] = useState(false);
  const [isServiceFormOpen, setIsServiceFormOpen] = useState(false);
  const currentProject =
    projects.find((project) => project.Id === selectedProjectId) ?? projects[0];
  const canManageCurrentProject =
    !!currentProject?.Permissions?.CanManageResources;
  const applicationsQuery = usePlatformApplications(currentProject?.Id);
  const applications = applicationsQuery.data ?? [];
  const currentApplication =
    applications.find(
      (application) => application.Id === selectedApplicationId
    ) ?? applications[0];
  const servicesQuery = usePlatformServices(currentApplication?.Id);
  const services = servicesQuery.data ?? [];
  const currentService =
    services.find((service) => service.Id === selectedServiceId) ?? services[0];
  const deploymentsQuery = usePlatformServiceDeployments(currentService?.Id);
  const deployments = deploymentsQuery.data ?? [];
  const currentDeployment =
    deployments.find((deployment) => deployment.Id === selectedDeploymentId) ??
    deployments[0];

  return (
    <PlatformPage
      titleKey="platform.pages.applications.title"
      titleDefault="Applications"
    >
      <PlatformNoticeStack />
      <ActionBar>
        <Button
          color="primary"
          icon={Plus}
          disabled={!currentProject || !canManageCurrentProject}
          onClick={() => setIsApplicationFormOpen((value) => !value)}
          data-cy="platform-create-application-open"
        >
          {t('platform.actions.createApplication', {
            defaultValue: 'Create application',
          })}
        </Button>
        <Button
          color="light"
          icon={Plus}
          disabled={!currentApplication || !canManageCurrentProject}
          onClick={() => setIsServiceFormOpen((value) => !value)}
          data-cy="platform-create-service-open"
        >
          {t('platform.actions.createService', {
            defaultValue: 'Create service',
          })}
        </Button>
      </ActionBar>
      {isApplicationFormOpen && canManageCurrentProject && (
        <CreateApplicationPanel
          projects={projects}
          projectId={currentProject?.Id}
          onProjectChange={(projectId) => {
            setSelectedProjectId(projectId);
            setSelectedApplicationId(undefined);
            setSelectedServiceId(undefined);
            setSelectedDeploymentId(undefined);
          }}
          onDone={() => setIsApplicationFormOpen(false)}
        />
      )}
      {isServiceFormOpen && canManageCurrentProject && (
        <CreateServicePanel
          applications={applications}
          applicationId={currentApplication?.Id}
          onApplicationChange={(applicationId) => {
            setSelectedApplicationId(applicationId);
            setSelectedServiceId(undefined);
            setSelectedDeploymentId(undefined);
          }}
          onDone={() => setIsServiceFormOpen(false)}
        />
      )}
      <div className="mx-4 mb-4 grid gap-3 lg:grid-cols-4">
        <SelectField
          label={t('platform.filters.project', { defaultValue: 'Project' })}
          value={currentProject?.Id}
          disabled={projects.length === 0}
          onChange={(projectId) => {
            setSelectedProjectId(projectId);
            setSelectedApplicationId(undefined);
            setSelectedServiceId(undefined);
            setSelectedDeploymentId(undefined);
          }}
        >
          {projects.map((project) => (
            <option key={project.Id} value={project.Id}>
              {project.Name}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('platform.filters.application', {
            defaultValue: 'Application',
          })}
          value={currentApplication?.Id}
          disabled={applications.length === 0}
          onChange={(applicationId) => {
            setSelectedApplicationId(applicationId);
            setSelectedServiceId(undefined);
            setSelectedDeploymentId(undefined);
          }}
        >
          {applications.map((application) => (
            <option key={application.Id} value={application.Id}>
              {application.Name}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('platform.filters.service', { defaultValue: 'Service' })}
          value={currentService?.Id}
          disabled={services.length === 0}
          onChange={(serviceId) => {
            setSelectedServiceId(serviceId);
            setSelectedDeploymentId(undefined);
          }}
        >
          {services.map((service) => (
            <option key={service.Id} value={service.Id}>
              {service.Name}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('platform.filters.deployment', {
            defaultValue: 'Deployment config',
          })}
          value={currentDeployment?.Id}
          disabled={deployments.length === 0}
          onChange={setSelectedDeploymentId}
        >
          {deployments.map((deployment) => (
            <option key={deployment.Id} value={deployment.Id}>
              #{deployment.Id} / env {deployment.EnvironmentId}
            </option>
          ))}
        </SelectField>
      </div>
      <div className="mx-4 grid gap-4 xl:grid-cols-2">
        <DataSection
          title={t('platform.applications.tableTitle', {
            defaultValue: 'Applications',
          })}
          isLoading={projectsQuery.isLoading || applicationsQuery.isLoading}
          empty={applications.length === 0}
          emptyMessage={t('platform.empty.applications', {
            defaultValue:
              'Select a project with applications to view application-level deployment status.',
          })}
        >
          <ApplicationsTable applications={applications} />
        </DataSection>
        <DataSection
          title={t('platform.services.tableTitle', {
            defaultValue: 'Services',
          })}
          isLoading={servicesQuery.isLoading}
          empty={services.length === 0}
          emptyMessage={t('platform.empty.services', {
            defaultValue:
              'Services appear after an application is selected. Each service can later own one deployment config per environment.',
          })}
        >
          <ServicesTable services={services} />
        </DataSection>
      </div>
      <ServiceRuntimePanel
        deployment={currentDeployment}
        isDeploymentLoading={deploymentsQuery.isLoading}
      />
    </PlatformPage>
  );
}

export function PlatformArtifactsView() {
  const { t } = useTranslation();
  const artifactsQuery = usePlatformArtifacts();
  const artifacts = artifactsQuery.data ?? [];
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];
  const artifactProjects = projects.filter(
    (project) => project.Permissions?.CanDeploy
  );
  const [isArtifactFormOpen, setIsArtifactFormOpen] = useState(false);
  const [isJavaBuildFormOpen, setIsJavaBuildFormOpen] = useState(false);
  const [isStaticBuildFormOpen, setIsStaticBuildFormOpen] = useState(false);
	const [isArtifactPushFormOpen, setIsArtifactPushFormOpen] = useState(false);
	const [isArtifactCleanupFormOpen, setIsArtifactCleanupFormOpen] = useState(false);
	const buildableStaticArtifacts = artifacts.filter(
		(artifact) =>
			artifact.Type === 'frontend-dist' &&
			projects.some(
				(project) =>
					project.Id === artifact.ProjectId && project.Permissions?.CanDeploy
			)
	);
	const pushableArtifacts = artifacts.filter(
		(artifact) =>
			artifact.Status === 'built' &&
			!!artifact.CandidateImageRef &&
			projects.some(
				(project) =>
					project.Id === artifact.ProjectId && project.Permissions?.CanDeploy
			)
	);
	const cleanupableArtifacts = artifacts.filter(
		(artifact) =>
			artifact.Retained &&
			artifact.Cleanable &&
			projects.some(
				(project) =>
					project.Id === artifact.ProjectId &&
					project.Permissions?.CanManageProject
			)
	);

  return (
    <PlatformPage
      titleKey="platform.pages.artifacts.title"
      titleDefault="Artifacts"
    >
      <PlatformNoticeStack />
      <ActionBar>
        <Button
          color="primary"
          icon={Plus}
          disabled={artifactProjects.length === 0}
          onClick={() => setIsArtifactFormOpen((value) => !value)}
          data-cy="platform-create-artifact-open"
        >
          {t('platform.actions.createArtifact', {
            defaultValue: 'Register image artifact',
          })}
        </Button>
		<Button
			color="light"
			disabled={!artifacts.some((artifact) => artifact.Type === 'java-jar')}
			onClick={() => setIsJavaBuildFormOpen((value) => !value)}
			data-cy="platform-java-build-open"
		>
			{t('platform.actions.packageJava', {
				defaultValue: 'Package Java 8 artifact',
			})}
		</Button>
		<Button
			color="light"
			disabled={buildableStaticArtifacts.length === 0}
			onClick={() => setIsStaticBuildFormOpen((value) => !value)}
			data-cy="platform-static-build-open"
		>
			{t('platform.actions.packageStatic', {
				defaultValue: 'Package frontend dist',
			})}
		</Button>
		<Button
			color="light"
			disabled={pushableArtifacts.length === 0}
			onClick={() => setIsArtifactPushFormOpen((value) => !value)}
			data-cy="platform-artifact-push-open"
		>
			{t('platform.actions.pushArtifact', {
				defaultValue: 'Push candidate image',
			})}
		</Button>
		<Button
			color="danger"
			disabled={cleanupableArtifacts.length === 0}
			onClick={() => setIsArtifactCleanupFormOpen((value) => !value)}
			data-cy="platform-artifact-cleanup-open"
		>
			{t('platform.actions.cleanupArtifactOriginal', {
				defaultValue: 'Clean original artifact',
			})}
		</Button>
		</ActionBar>
      {isArtifactFormOpen && (
        <CreateArtifactPanel
          projects={artifactProjects}
          onDone={() => setIsArtifactFormOpen(false)}
        />
      )}
		{isJavaBuildFormOpen && <JavaBuildPanel artifacts={artifacts} onDone={() => setIsJavaBuildFormOpen(false)} />}
		{isStaticBuildFormOpen && (
			<StaticBuildPanel
				artifacts={buildableStaticArtifacts}
				onDone={() => setIsStaticBuildFormOpen(false)}
			/>
		)}
		{isArtifactPushFormOpen && (
			<ArtifactPushPanel
				artifacts={pushableArtifacts}
				onDone={() => setIsArtifactPushFormOpen(false)}
			/>
		)}
		{isArtifactCleanupFormOpen && (
			<ArtifactCleanupPanel
				artifacts={cleanupableArtifacts}
				onDone={() => setIsArtifactCleanupFormOpen(false)}
			/>
		)}
      <DataSection
        title={t('platform.artifacts.tableTitle', {
          defaultValue: 'Artifacts',
        })}
        isLoading={artifactsQuery.isLoading || projectsQuery.isLoading}
        empty={artifacts.length === 0}
        emptyMessage={t('platform.empty.artifacts', {
          defaultValue:
            'No image-reference artifacts yet. Register an existing image to use it in a release.',
        })}
      >
        <ArtifactsTable artifacts={artifacts} />
      </DataSection>
    </PlatformPage>
  );
}

export function PlatformReleasesView() {
  const { t } = useTranslation();
  const releasesQuery = usePlatformReleases();
  const releases = releasesQuery.data ?? [];
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];
  const failedCount = releases.filter(
    (release) => release.Status === 'failed'
  ).length;

  return (
    <PlatformPage
      titleKey="platform.pages.releases.title"
      titleDefault="Releases"
    >
      <PlatformNoticeStack />
      <SummaryStrip
        items={[
          {
            label: t('platform.summary.releases', { defaultValue: 'Releases' }),
            value: releases.length,
          },
          {
            label: t('platform.summary.failed', { defaultValue: 'Failed' }),
            value: failedCount,
          },
          {
            label: t('platform.summary.manualAction', {
              defaultValue: 'Manual action',
            }),
            value: releases.filter(
              (release) =>
                release.ManualActionRequired ||
                release.Status === 'interrupted' ||
                release.Status === 'recovery-failed'
            ).length,
          },
        ]}
      />
      <DataSection
        title={t('platform.releases.tableTitle', {
          defaultValue: 'Release records',
        })}
        isLoading={releasesQuery.isLoading || projectsQuery.isLoading}
        empty={releases.length === 0}
        emptyMessage={t('platform.empty.releases', {
          defaultValue:
            'No release records yet. Create a release from the deployment wizard after selecting a service and image.',
        })}
      >
        <ReleasesTable releases={releases} projects={projects} />
      </DataSection>
    </PlatformPage>
  );
}

// 配置中心只允许项目、环境和服务部署三级作用域，并把当前作用域随请求交给服务端复核；
// 这样浏览器本地状态即使被篡改，也不能把项目级表单越权写成环境级或服务部署级配置。
export function PlatformConfigView() {
  const { t } = useTranslation();
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];
  const [selectedProjectId, setSelectedProjectId] = useState<number>();
  const [scopeType, setScopeType] =
    useState<PlatformConfigScopeType>('project');
  const [selectedEnvironmentId, setSelectedEnvironmentId] = useState<number>();
  const [selectedApplicationId, setSelectedApplicationId] = useState<number>();
  const [selectedServiceId, setSelectedServiceId] = useState<number>();
  const [selectedDeploymentId, setSelectedDeploymentId] = useState<number>();
  const [selectedConfigSetId, setSelectedConfigSetId] = useState<number>();
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const currentProject =
    projects.find((project) => project.Id === selectedProjectId) ?? projects[0];
  const canManage = !!currentProject?.Permissions?.CanManageResources;
  const canReveal = !!currentProject?.Permissions?.CanRevealSensitive;
  const environmentsQuery = usePlatformEnvironments(currentProject?.Id);
  const environments = environmentsQuery.data ?? [];
  const currentEnvironment =
    environments.find(
      (environment) => environment.Id === selectedEnvironmentId
    ) ?? environments[0];
  const applicationsQuery = usePlatformApplications(currentProject?.Id);
  const applications = applicationsQuery.data ?? [];
  const currentApplication =
    applications.find(
      (application) => application.Id === selectedApplicationId
    ) ?? applications[0];
  const servicesQuery = usePlatformServices(currentApplication?.Id);
  const services = servicesQuery.data ?? [];
  const currentService =
    services.find((service) => service.Id === selectedServiceId) ?? services[0];
  const deploymentsQuery = usePlatformServiceDeployments(currentService?.Id);
  const deployments = deploymentsQuery.data ?? [];
  const currentDeployment =
    deployments.find((deployment) => deployment.Id === selectedDeploymentId) ??
    deployments[0];
  const scopeId =
    scopeType === 'project'
      ? currentProject?.Id
      : scopeType === 'environment'
        ? currentEnvironment?.Id
        : currentDeployment?.Id;
  const configSetsQuery = usePlatformConfigSets({
    projectId: currentProject?.Id,
    scopeType,
    scopeId,
  });
  const configSets = configSetsQuery.data ?? [];
  const selectedConfigSet = configSets.find(
    (configSet) => configSet.Id === selectedConfigSetId
  );
  const effectiveConfigQuery = usePlatformEffectiveConfig(
    currentDeployment?.Id
  );
  const archiveMutation = useArchivePlatformConfigSetMutation();

  useEffect(() => {
    if (
      selectedConfigSetId &&
      !configSets.some((configSet) => configSet.Id === selectedConfigSetId)
    ) {
      setSelectedConfigSetId(undefined);
      setIsEditorOpen(false);
    }
  }, [configSets, selectedConfigSetId]);

  function selectScope(nextScope: string) {
    setScopeType(nextScope as PlatformConfigScopeType);
    setSelectedConfigSetId(undefined);
    setIsEditorOpen(false);
  }

  function archive(configSet: PlatformConfigSet) {
    if (
      !window.confirm(
        t('platform.config.archiveConfirm', {
          defaultValue:
            'Archive config set {{name}}? It will no longer affect new releases.',
          name: configSet.Name,
        })
      )
    ) {
      return;
    }
    archiveMutation.mutate(configSet.Id, {
      onSuccess: () => {
        if (selectedConfigSetId === configSet.Id) {
          setSelectedConfigSetId(undefined);
          setIsEditorOpen(false);
        }
      },
    });
  }

  return (
    <PlatformPage
      titleKey="platform.pages.config.title"
      titleDefault="Configuration center"
    >
      <PlatformNoticeStack />
      <div className="mx-4 mb-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section className="rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
          <div className="mb-4 flex items-center gap-2 text-lg font-semibold">
            <SlidersHorizontal className="icon" />
            {t('platform.config.scopeTitle', {
              defaultValue: 'Select configuration scope',
            })}
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <SelectField
              label={t('platform.filters.project', { defaultValue: 'Project' })}
              value={currentProject?.Id}
              disabled={projects.length === 0}
              onChange={(projectId) => {
                setSelectedProjectId(projectId);
                setSelectedEnvironmentId(undefined);
                setSelectedApplicationId(undefined);
                setSelectedServiceId(undefined);
                setSelectedDeploymentId(undefined);
                setSelectedConfigSetId(undefined);
                setIsEditorOpen(false);
              }}
            >
              {projects.map((project) => (
                <option key={project.Id} value={project.Id}>
                  {project.Name}
                </option>
              ))}
            </SelectField>
            <StringSelectField
              label={t('platform.config.scope', { defaultValue: 'Scope' })}
              value={scopeType}
              onChange={selectScope}
              options={[
                [
                  'project',
                  t('platform.config.scopes.project', {
                    defaultValue: 'Project',
                  }),
                ],
                [
                  'environment',
                  t('platform.config.scopes.environment', {
                    defaultValue: 'Environment',
                  }),
                ],
                [
                  'service-deployment',
                  t('platform.config.scopes.serviceDeployment', {
                    defaultValue: 'Service deployment',
                  }),
                ],
              ]}
            />
            {scopeType === 'environment' && (
              <SelectField
                label={t('platform.config.environment', {
                  defaultValue: 'Environment',
                })}
                value={currentEnvironment?.Id}
                disabled={environments.length === 0}
                onChange={setSelectedEnvironmentId}
              >
                {environments.map((environment) => (
                  <option key={environment.Id} value={environment.Id}>
                    {environment.Name}
                  </option>
                ))}
              </SelectField>
            )}
            {scopeType === 'service-deployment' && (
              <>
                <SelectField
                  label={t('platform.filters.application', {
                    defaultValue: 'Application',
                  })}
                  value={currentApplication?.Id}
                  disabled={applications.length === 0}
                  onChange={(applicationId) => {
                    setSelectedApplicationId(applicationId);
                    setSelectedServiceId(undefined);
                    setSelectedDeploymentId(undefined);
                  }}
                >
                  {applications.map((application) => (
                    <option key={application.Id} value={application.Id}>
                      {application.Name}
                    </option>
                  ))}
                </SelectField>
                <SelectField
                  label={t('platform.filters.service', {
                    defaultValue: 'Service',
                  })}
                  value={currentService?.Id}
                  disabled={services.length === 0}
                  onChange={(serviceId) => {
                    setSelectedServiceId(serviceId);
                    setSelectedDeploymentId(undefined);
                  }}
                >
                  {services.map((service) => (
                    <option key={service.Id} value={service.Id}>
                      {service.Name}
                    </option>
                  ))}
                </SelectField>
                <SelectField
                  label={t('platform.filters.deployment', {
                    defaultValue: 'Deployment config',
                  })}
                  value={currentDeployment?.Id}
                  disabled={deployments.length === 0}
                  onChange={setSelectedDeploymentId}
                >
                  {deployments.map((deployment) => (
                    <option key={deployment.Id} value={deployment.Id}>
                      #{deployment.Id} · {deployment.EnvironmentId}
                    </option>
                  ))}
                </SelectField>
              </>
            )}
          </div>
        </section>
        <EffectiveConfigDetails
          response={effectiveConfigQuery.data}
          isLoading={effectiveConfigQuery.isLoading}
          emptyMessage={t('platform.config.effectiveEmpty', {
            defaultValue:
              'Select a service deployment to preview its effective configuration and drift status.',
          })}
        />
      </div>
      <DataSection
        title={t('platform.config.tableTitle', { defaultValue: 'Config sets' })}
        isLoading={configSetsQuery.isLoading}
        empty={!scopeId || configSets.length === 0}
        emptyMessage={t('platform.config.empty', {
          defaultValue: 'No active config sets exist for this scope.',
        })}
      >
        <ConfigSetsTable
          configSets={configSets}
          canManage={canManage}
          onEdit={(configSet) => {
            setSelectedConfigSetId(configSet.Id);
            setIsEditorOpen(true);
          }}
          onArchive={archive}
          isArchiving={archiveMutation.isLoading}
        />
      </DataSection>
      {canManage && scopeId && (
        <ActionBar>
          <Button
            color="primary"
            icon={Plus}
            onClick={() => {
              setSelectedConfigSetId(undefined);
              setIsEditorOpen(true);
            }}
            data-cy="platform-config-set-create"
          >
            {t('platform.actions.createConfigSet', {
              defaultValue: 'Create config set',
            })}
          </Button>
        </ActionBar>
      )}
      {isEditorOpen && currentProject && scopeId && (
        <ConfigSetEditor
          configSet={selectedConfigSet}
          projectId={currentProject.Id}
          scopeType={scopeType}
          scopeId={scopeId}
          canReveal={canReveal}
          onDone={(configSetId) => {
            setSelectedConfigSetId(configSetId);
            setIsEditorOpen(false);
          }}
        />
      )}
    </PlatformPage>
  );
}

export function PlatformDeployView() {
  const { t } = useTranslation();
  const [step, setStep] = useState(0);
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];
  const [selectedProjectId, setSelectedProjectId] = useState<number>();
  const currentProject =
    projects.find((project) => project.Id === selectedProjectId) ?? projects[0];
  const canDeployCurrentProject = !!currentProject?.Permissions?.CanDeploy;
  const environmentsQuery = usePlatformEnvironments(currentProject?.Id);
  const environments = environmentsQuery.data ?? [];
  const [selectedEnvironmentId, setSelectedEnvironmentId] = useState<number>();
  const currentEnvironment =
    environments.find(
      (environment) => environment.Id === selectedEnvironmentId
    ) ?? environments[0];
  const applicationsQuery = usePlatformApplications(currentProject?.Id);
  const applications = applicationsQuery.data ?? [];
  const [selectedApplicationId, setSelectedApplicationId] = useState<number>();
  const currentApplication =
    applications.find(
      (application) => application.Id === selectedApplicationId
    ) ?? applications[0];
  const servicesQuery = usePlatformServices(currentApplication?.Id);
  const services = servicesQuery.data ?? [];
  const [selectedServiceId, setSelectedServiceId] = useState<number>();
  const currentService =
    services.find((service) => service.Id === selectedServiceId) ?? services[0];
  const deploymentsQuery = usePlatformServiceDeployments(currentService?.Id);
  const deployments = deploymentsQuery.data ?? [];
  const currentDeployment = deployments.find(
    (deployment) => deployment.EnvironmentId === currentEnvironment?.Id
  );
  const effectiveConfigQuery = usePlatformEffectiveConfig(
    currentDeployment?.Id
  );
  const artifactsQuery = usePlatformArtifacts();
  const artifacts = artifactsQuery.data ?? [];
	const [selectedArtifactId, setSelectedArtifactId] = useState<number>();
	const readyArtifacts = artifacts.filter(
		(artifact) =>
			artifact.ProjectId === currentProject?.Id &&
			artifact.ApplicationId === currentApplication?.Id &&
			artifact.ServiceDefinitionId === currentService?.Id &&
			artifact.Status === 'ready' &&
			!!artifact.ImageRef &&
			artifactMatchesService(artifact, currentService)
	);
	const selectedArtifact = readyArtifacts.find(
		(artifact) => artifact.Id === selectedArtifactId
	);
  const createDeploymentMutation = useCreatePlatformServiceDeploymentMutation();
  const updateDeploymentMutation = useUpdatePlatformServiceDeploymentMutation();
  const createArtifactMutation = useCreateImageReferenceArtifactMutation();
  const validateReleaseMutation = useValidatePlatformReleaseMutation();
  const createReleaseMutation = useCreatePlatformReleaseMutation();
  const [artifactName, setArtifactName] = useState('');
  const [version, setVersion] = useState('');
  const [imageRef, setImageRef] = useState('');
  const [imageDigest, setImageDigest] = useState('');
  const [containerPort, setContainerPort] = useState(80);
  const [hostPort, setHostPort] = useState(18080);
  const [healthType, setHealthType] = useState('http');
  const [healthPath, setHealthPath] = useState('/');
  const [healthPort, setHealthPort] = useState(80);
  const [healthVerification, setHealthVerification] = useState('verified');
  const [envText, setEnvText] = useState('');
  const [validationResult, setValidationResult] =
    useState<PlatformReleaseValidateResponse>();
  const [releaseResult, setReleaseResult] =
    useState<PlatformReleaseCreateResponse>();
  const steps = useMemo(
    () => [
      t('platform.deploy.steps.basic', { defaultValue: 'Basic information' }),
      t('platform.deploy.steps.image', { defaultValue: 'Artifact' }),
      t('platform.deploy.steps.registry', {
        defaultValue: 'Image and registry',
      }),
      t('platform.deploy.steps.health', {
        defaultValue: 'Ports and health check',
      }),
      t('platform.deploy.steps.env', {
        defaultValue: 'Environment variables',
      }),
      t('platform.deploy.steps.strategy', {
        defaultValue: 'Release strategy',
      }),
      t('platform.deploy.steps.validate', {
        defaultValue: 'Validation result',
      }),
      t('platform.deploy.steps.confirm', { defaultValue: 'Confirm' }),
    ],
    [t]
  );
  const desiredSpec = useMemo(
    () =>
      buildDeployDesiredSpec({
        imageRef,
        imageDigest,
        containerPort,
        hostPort,
        healthType,
        healthPath,
        healthPort,
        healthVerification,
        envText,
      }),
    [
      imageDigest,
      imageRef,
      containerPort,
      hostPort,
      healthType,
      healthPath,
      healthPort,
      healthVerification,
      envText,
    ]
  );
  const isReady =
    !!currentProject &&
    !!currentEnvironment &&
    !!currentApplication &&
    !!currentService &&
    !!imageRef.trim() &&
    !!version.trim() &&
    containerPort > 0 &&
    hostPort > 0;

	useEffect(() => {
		if (
			selectedArtifactId !== undefined &&
			!readyArtifacts.some((artifact) => artifact.Id === selectedArtifactId)
		) {
			setSelectedArtifactId(undefined);
		}
	}, [readyArtifacts, selectedArtifactId]);
  const isBusy =
    createDeploymentMutation.isLoading ||
    updateDeploymentMutation.isLoading ||
    createArtifactMutation.isLoading ||
    validateReleaseMutation.isLoading ||
    createReleaseMutation.isLoading;

  async function ensureDeploymentAndArtifact() {
    if (!currentProject || !currentEnvironment || !currentApplication) {
      throw new Error('Missing project, environment, or application');
    }
    if (!currentService) {
      throw new Error('Missing service');
    }

    // 发布创建依赖部署配置修订号和制品 ID；先持久化 DesiredSpec 与镜像制品，
    // 再组装 release payload，避免用页面临时状态触发不一致的发布。
    const deployment = currentDeployment
      ? await updateDeploymentMutation.mutateAsync({
          deploymentId: currentDeployment.Id,
          payload: {
            ResourceVersion: currentDeployment.ResourceVersion,
            DesiredSpec: desiredSpec,
          },
        })
      : await createDeploymentMutation.mutateAsync({
          serviceDefinitionId: currentService.Id,
          payload: {
            EnvironmentId: currentEnvironment.Id,
            DesiredSpec: desiredSpec,
          },
        });
    const reusableArtifact = selectedArtifact ?? artifacts.find(
      (artifact) =>
        artifact.ProjectId === currentProject.Id &&
        artifact.ApplicationId === currentApplication.Id &&
        artifact.ServiceDefinitionId === currentService.Id &&
			artifact.Type === 'image' &&
			artifact.SourceType === 'image-reference' &&
        artifact.ImageRef === imageRef.trim() &&
        artifact.Version === version.trim()
    );
    const artifact =
      reusableArtifact ??
      (await createArtifactMutation.mutateAsync({
        ProjectId: currentProject.Id,
        ApplicationId: currentApplication.Id,
        ServiceDefinitionId: currentService.Id,
        Name:
          artifactName.trim() ||
          currentService.Slug ||
          currentService.Name ||
          imageRef.trim(),
        Version: version.trim(),
        ImageRef: imageRef.trim(),
        ImageDigest: imageDigest.trim() || undefined,
        Traceability: 'weak',
      }));

    return {
      deployment,
      artifact,
      payload: buildReleasePayload({
        project: currentProject,
        environment: currentEnvironment,
        application: currentApplication,
        service: currentService,
        deployment,
        artifact,
        version: version.trim(),
      }),
    };
  }

  async function handleValidateRelease() {
    const { payload } = await ensureDeploymentAndArtifact();
    const result = await validateReleaseMutation.mutateAsync(payload);
    setValidationResult(result);
    setReleaseResult(undefined);
    setStep(6);
  }

  async function handleCreateRelease() {
    const confirmMessage = currentEnvironment?.IsProduction
      ? t('platform.deploy.confirmProductionRelease', {
          defaultValue:
            'This is a production environment. V0.1 replace strategy may cause short downtime. Continue creating the release?',
        })
      : t('platform.deploy.confirmRelease', {
          defaultValue: 'Create this release now?',
        });
    if (!window.confirm(confirmMessage)) {
      return;
    }

    const { payload } = await ensureDeploymentAndArtifact();
    const result = await createReleaseMutation.mutateAsync({
      payload,
      idempotencyKey: createIdempotencyKey(payload),
    });
    setReleaseResult(result);
    setValidationResult(undefined);
    setStep(7);
  }

  return (
    <PlatformPage titleKey="platform.pages.deploy.title" titleDefault="Deploy">
      <PlatformNoticeStack />
      {currentProject && !canDeployCurrentProject && (
        <div className="mx-4 mb-4">
          <Alert
            color="warn"
            title={t('platform.alerts.permissionDenied.title')}
          >
            {t('platform.alerts.permissionDenied.deployBody')}
          </Alert>
        </div>
      )}
      <div className="mx-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <section className="rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
          <ol className="mb-4 grid gap-2 md:grid-cols-4">
            {steps.map((label, index) => (
              <li key={label}>
                <button
                  type="button"
                  className={`w-full rounded border border-solid px-3 py-2 text-left text-sm ${
                    index === step
                      ? 'border-blue-6 bg-blue-2 text-blue-9 th-dark:bg-blue-10 th-dark:text-white'
                      : 'border-gray-5 bg-gray-2 text-gray-8 th-dark:bg-gray-10 th-dark:text-white'
                  }`}
                  onClick={() => setStep(index)}
                >
                  <span className="font-semibold">{index + 1}.</span> {label}
                </button>
              </li>
            ))}
          </ol>
          <WizardStepContent
            step={step}
            projects={projects}
            environments={environments}
            applications={applications}
            services={services}
            currentProject={currentProject}
            currentEnvironment={currentEnvironment}
            currentApplication={currentApplication}
            currentService={currentService}
			readyArtifacts={readyArtifacts}
			selectedArtifact={selectedArtifact}
			onArtifactSelect={(artifactId) => {
				setSelectedArtifactId(artifactId);
				const artifact = readyArtifacts.find((item) => item.Id === artifactId);
				if (artifact) {
					setArtifactName(artifact.Name);
					setVersion(artifact.Version);
					setImageRef(artifact.ImageRef ?? '');
					setImageDigest(artifact.ImageDigest ?? '');
				}
				setValidationResult(undefined);
				setReleaseResult(undefined);
			}}
            selectedProjectId={currentProject?.Id}
            selectedEnvironmentId={currentEnvironment?.Id}
            selectedApplicationId={currentApplication?.Id}
            selectedServiceId={currentService?.Id}
            onProjectChange={(projectId) => {
              setSelectedProjectId(projectId);
              setSelectedEnvironmentId(undefined);
              setSelectedApplicationId(undefined);
              setSelectedServiceId(undefined);
				setSelectedArtifactId(undefined);
              setValidationResult(undefined);
              setReleaseResult(undefined);
            }}
            onEnvironmentChange={(environmentId) => {
              setSelectedEnvironmentId(environmentId);
				setSelectedArtifactId(undefined);
              setValidationResult(undefined);
              setReleaseResult(undefined);
            }}
            onApplicationChange={(applicationId) => {
              setSelectedApplicationId(applicationId);
              setSelectedServiceId(undefined);
				setSelectedArtifactId(undefined);
              setValidationResult(undefined);
              setReleaseResult(undefined);
            }}
            onServiceChange={(serviceId) => {
              setSelectedServiceId(serviceId);
				setSelectedArtifactId(undefined);
              setValidationResult(undefined);
              setReleaseResult(undefined);
            }}
            artifactName={artifactName}
            onArtifactNameChange={setArtifactName}
            version={version}
            onVersionChange={setVersion}
            imageRef={imageRef}
            onImageRefChange={setImageRef}
            imageDigest={imageDigest}
            onImageDigestChange={setImageDigest}
            containerPort={containerPort}
            onContainerPortChange={setContainerPort}
            hostPort={hostPort}
            onHostPortChange={setHostPort}
            healthType={healthType}
            onHealthTypeChange={setHealthType}
            healthPath={healthPath}
            onHealthPathChange={setHealthPath}
            healthPort={healthPort}
            onHealthPortChange={setHealthPort}
            healthVerification={healthVerification}
            onHealthVerificationChange={setHealthVerification}
            envText={envText}
            onEnvTextChange={setEnvText}
            desiredSpec={desiredSpec}
            currentDeployment={currentDeployment}
            validationResult={validationResult}
            releaseResult={releaseResult}
          />
          <div className="mt-5 flex items-center justify-between gap-3">
            <Button
              color="light"
              disabled={step === 0}
              onClick={() => setStep((value) => Math.max(0, value - 1))}
              data-cy="platform-deploy-prev"
            >
              {t('platform.actions.previous', { defaultValue: 'Previous' })}
            </Button>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                color="light"
                onClick={() =>
                  setStep((value) => Math.min(steps.length - 1, value + 1))
                }
                disabled={step === steps.length - 1}
                data-cy="platform-deploy-next"
              >
                {t('platform.actions.next', { defaultValue: 'Next' })}
              </Button>
              <Button
                color="light"
                icon={CheckCircle2}
                disabled={!isReady || isBusy || !canDeployCurrentProject}
                onClick={handleValidateRelease}
                data-cy="platform-release-validate"
              >
                {t('platform.actions.validateRelease', {
                  defaultValue: 'Validate release',
                })}
              </Button>
              <Button
                color="primary"
                icon={Rocket}
                disabled={!isReady || isBusy || !canDeployCurrentProject}
                onClick={handleCreateRelease}
                data-cy="platform-release-create"
              >
                {t('platform.actions.createRelease', {
                  defaultValue: 'Create release',
                })}
              </Button>
            </div>
          </div>
        </section>
        <aside className="rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
          <h3 className="mb-3 text-base font-semibold">
            {t('platform.deploy.summary.title', {
              defaultValue: 'Deployment summary',
            })}
          </h3>
          <SummaryList
            rows={[
              [
                t('platform.deploy.summary.project', {
                  defaultValue: 'Project',
                }),
                currentProject?.Name ?? '',
              ],
              [
                t('platform.deploy.summary.environment', {
                  defaultValue: 'Environment',
                }),
                currentEnvironment?.Name ?? '',
              ],
              [
                t('platform.deploy.summary.service', {
                  defaultValue: 'Service',
                }),
                currentService?.Name ?? '',
              ],
              [
                t('platform.deploy.summary.image', { defaultValue: 'Image' }),
                imageRef,
              ],
				[
					t('platform.deploy.summary.artifact', {
						defaultValue: 'Artifact',
					}),
					selectedArtifact
						? `${selectedArtifact.Name} · ${selectedArtifact.Version}`
						: t('platform.deploy.summary.manualImage', {
							defaultValue: 'Manual image reference',
						}),
				],
				[
					t('platform.deploy.summary.digest', {
						defaultValue: 'Digest',
					}),
					imageDigest ||
						t('platform.deploy.summary.notAvailable', {
							defaultValue: 'Not available',
						}),
				],
              [
                t('platform.deploy.summary.validation', {
                  defaultValue: 'Validation',
                }),
                validationResult?.Status ||
                  validationResult?.Reason ||
                  releaseResult?.Status ||
                  t('platform.deploy.summary.notValidated', {
                    defaultValue: 'Not validated',
                  }),
              ],
            ]}
          />
          {currentDeployment && (
            <div className="text-muted mt-4 text-xs">
              {t('platform.deploy.summary.existingDeployment', {
                defaultValue:
                  'Existing deployment config #{{id}} will be updated before release.',
                id: currentDeployment.Id,
              })}
            </div>
          )}
          <div className="mt-4">
            <EffectiveConfigDetails
              response={effectiveConfigQuery.data}
              isLoading={effectiveConfigQuery.isLoading}
              emptyMessage={t('platform.config.effectiveDeployEmpty', {
                defaultValue:
                  'Save a service deployment configuration to preview the merged project, environment, and deployment values.',
              })}
            />
          </div>
          {releaseResult?.Release && (
            <Alert
              className="mt-4"
              color="success"
              title={t('platform.deploy.releaseCreated', {
                defaultValue: 'Release created',
              })}
            >
              {t('platform.deploy.releaseCreatedBody', {
                defaultValue: 'Release #{{id}} was created.',
                id: releaseResult.Release.Id,
              })}
            </Alert>
          )}
        </aside>
      </div>
    </PlatformPage>
  );
}

function CreateProjectPanel({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation();
  const mutation = useCreatePlatformProjectMutation();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [isSlugTouched, setIsSlugTouched] = useState(false);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    mutation.mutate(
      {
        Name: name.trim(),
        Slug: slug.trim(),
        Description: description.trim() || undefined,
      },
      {
        onSuccess: () => {
          setName('');
          setSlug('');
          setDescription('');
          setIsSlugTouched(false);
          onDone();
        },
      }
    );
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.project', {
        defaultValue: 'Create project',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={handleSubmit}>
        <TextInputField
          label={t('platform.forms.name', { defaultValue: 'Name' })}
          value={name}
          required
          onChange={(value) => {
            setName(value);
            if (!isSlugTouched) {
              setSlug(slugify(value));
            }
          }}
        />
        <TextInputField
          label={t('platform.forms.slug', { defaultValue: 'Slug' })}
          value={slug}
          required
          onChange={(value) => {
            setSlug(value);
            setIsSlugTouched(true);
          }}
        />
        <TextInputField
          label={t('platform.forms.description', {
            defaultValue: 'Description',
          })}
          value={description}
          onChange={setDescription}
        />
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.createProject', {
            defaultValue: 'Create project',
          })}
          submitDisabled={!name.trim() || !slug.trim()}
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function CreateEnvironmentPanel({
  projects,
  projectId,
  onProjectChange,
  onDone,
}: {
  projects: PlatformProject[];
  projectId?: number;
  onProjectChange: (projectId: number | undefined) => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = useCreatePlatformEnvironmentMutation();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [isSlugTouched, setIsSlugTouched] = useState(false);
  const [type, setType] = useState('dev');
  const [endpointId, setEndpointId] = useState('');
  const [healthCheckHost, setHealthCheckHost] = useState('127.0.0.1');
  const [targetHost, setTargetHost] = useState('127.0.0.1');
  const parsedEndpointId = Number(endpointId);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!projectId) {
      return;
    }
    mutation.mutate(
      {
        projectId,
        payload: {
          Name: name.trim(),
          Slug: slug.trim(),
          Type: type,
          IsProduction: type === 'prod',
          TargetMode: 'single',
          HealthCheckHost: healthCheckHost.trim() || '127.0.0.1',
          Targets: [
            {
              EndpointId: parsedEndpointId,
              Role: 'workload',
              HostAddress: targetHost.trim() || undefined,
              Enabled: true,
            },
          ],
        },
      },
      {
        onSuccess: () => {
          setName('');
          setSlug('');
          setIsSlugTouched(false);
          setEndpointId('');
          onDone();
        },
      }
    );
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.environment', {
        defaultValue: 'Create environment',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={handleSubmit}>
        <SelectField
          label={t('platform.forms.project', { defaultValue: 'Project' })}
          value={projectId}
          disabled={projects.length === 0}
          onChange={onProjectChange}
        >
          {projects.map((project) => (
            <option key={project.Id} value={project.Id}>
              {project.Name}
            </option>
          ))}
        </SelectField>
        <TextInputField
          label={t('platform.forms.name', { defaultValue: 'Name' })}
          value={name}
          required
          onChange={(value) => {
            setName(value);
            if (!isSlugTouched) {
              setSlug(slugify(value));
            }
          }}
        />
        <TextInputField
          label={t('platform.forms.slug', { defaultValue: 'Slug' })}
          value={slug}
          required
          onChange={(value) => {
            setSlug(value);
            setIsSlugTouched(true);
          }}
        />
        <StringSelectField
          label={t('platform.forms.type', { defaultValue: 'Type' })}
          value={type}
          onChange={setType}
          options={[
            [
              'dev',
              t('platform.environmentTypes.dev', { defaultValue: 'Dev' }),
            ],
            [
              'test',
              t('platform.environmentTypes.test', { defaultValue: 'Test' }),
            ],
            [
              'prod',
              t('platform.environmentTypes.prod', {
                defaultValue: 'Production',
              }),
            ],
            [
              'custom',
              t('platform.environmentTypes.custom', {
                defaultValue: 'Custom',
              }),
            ],
          ]}
        />
        <TextInputField
          label={t('platform.forms.endpointId', {
            defaultValue: 'Endpoint ID',
          })}
          value={endpointId}
          required
          onChange={setEndpointId}
          inputMode="numeric"
        />
        <TextInputField
          label={t('platform.forms.healthCheckHost', {
            defaultValue: 'Health check host',
          })}
          value={healthCheckHost}
          onChange={setHealthCheckHost}
        />
        <TextInputField
          label={t('platform.forms.targetHost', {
            defaultValue: 'Target host',
          })}
          value={targetHost}
          onChange={setTargetHost}
        />
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.createEnvironment', {
            defaultValue: 'Create environment',
          })}
          submitDisabled={
            !projectId ||
            !name.trim() ||
            !slug.trim() ||
            !Number.isInteger(parsedEndpointId) ||
            parsedEndpointId <= 0
          }
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function CreateApplicationPanel({
  projects,
  projectId,
  onProjectChange,
  onDone,
}: {
  projects: PlatformProject[];
  projectId?: number;
  onProjectChange: (projectId: number | undefined) => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = useCreatePlatformApplicationMutation();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [isSlugTouched, setIsSlugTouched] = useState(false);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!projectId) {
      return;
    }
    mutation.mutate(
      {
        projectId,
        payload: {
          Name: name.trim(),
          Slug: slug.trim(),
          Description: description.trim() || undefined,
        },
      },
      {
        onSuccess: () => {
          setName('');
          setSlug('');
          setDescription('');
          setIsSlugTouched(false);
          onDone();
        },
      }
    );
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.application', {
        defaultValue: 'Create application',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={handleSubmit}>
        <SelectField
          label={t('platform.forms.project', { defaultValue: 'Project' })}
          value={projectId}
          disabled={projects.length === 0}
          onChange={onProjectChange}
        >
          {projects.map((project) => (
            <option key={project.Id} value={project.Id}>
              {project.Name}
            </option>
          ))}
        </SelectField>
        <TextInputField
          label={t('platform.forms.name', { defaultValue: 'Name' })}
          value={name}
          required
          onChange={(value) => {
            setName(value);
            if (!isSlugTouched) {
              setSlug(slugify(value));
            }
          }}
        />
        <TextInputField
          label={t('platform.forms.slug', { defaultValue: 'Slug' })}
          value={slug}
          required
          onChange={(value) => {
            setSlug(value);
            setIsSlugTouched(true);
          }}
        />
        <TextInputField
          label={t('platform.forms.description', {
            defaultValue: 'Description',
          })}
          value={description}
          onChange={setDescription}
        />
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.createApplication', {
            defaultValue: 'Create application',
          })}
          submitDisabled={!projectId || !name.trim() || !slug.trim()}
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function CreateServicePanel({
  applications,
  applicationId,
  onApplicationChange,
  onDone,
}: {
  applications: PlatformApplication[];
  applicationId?: number;
  onApplicationChange: (applicationId: number | undefined) => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = useCreatePlatformServiceDefinitionMutation();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [type, setType] = useState('backend');
  const [isSlugTouched, setIsSlugTouched] = useState(false);

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!applicationId) {
      return;
    }
    mutation.mutate(
      {
        applicationId,
        payload: {
          Name: name.trim(),
          Slug: slug.trim(),
          Type: type,
          Description: description.trim() || undefined,
        },
      },
      {
        onSuccess: () => {
          setName('');
          setSlug('');
          setDescription('');
          setType('backend');
          setIsSlugTouched(false);
          onDone();
        },
      }
    );
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.service', {
        defaultValue: 'Create service',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={handleSubmit}>
        <SelectField
          label={t('platform.forms.application', {
            defaultValue: 'Application',
          })}
          value={applicationId}
          disabled={applications.length === 0}
          onChange={onApplicationChange}
        >
          {applications.map((application) => (
            <option key={application.Id} value={application.Id}>
              {application.Name}
            </option>
          ))}
        </SelectField>
        <TextInputField
          label={t('platform.forms.name', { defaultValue: 'Name' })}
          value={name}
          required
          onChange={(value) => {
            setName(value);
            if (!isSlugTouched) {
              setSlug(slugify(value));
            }
          }}
        />
        <TextInputField
          label={t('platform.forms.slug', { defaultValue: 'Slug' })}
          value={slug}
          required
          onChange={(value) => {
            setSlug(value);
            setIsSlugTouched(true);
          }}
        />
        <StringSelectField
          label={t('platform.forms.type', { defaultValue: 'Type' })}
          value={type}
          onChange={setType}
          options={[
            [
              'backend',
              t('platform.serviceTypes.backend', { defaultValue: 'Backend' }),
            ],
            [
              'frontend',
              t('platform.serviceTypes.frontend', {
                defaultValue: 'Frontend',
              }),
            ],
            [
              'worker',
              t('platform.serviceTypes.worker', { defaultValue: 'Worker' }),
            ],
          ]}
        />
        <TextInputField
          label={t('platform.forms.description', {
            defaultValue: 'Description',
          })}
          value={description}
          onChange={setDescription}
        />
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.createService', {
            defaultValue: 'Create service',
          })}
          submitDisabled={!applicationId || !name.trim() || !slug.trim()}
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function CreateArtifactPanel({
  projects,
  onDone,
}: {
  projects: PlatformProject[];
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = useCreateImageReferenceArtifactMutation();
	const uploadMutation = useUploadPlatformArtifactMutation();
  const [selectedProjectId, setSelectedProjectId] = useState<
    number | undefined
  >(projects[0]?.Id);
  const applicationsQuery = usePlatformApplications(selectedProjectId);
  const applications = applicationsQuery.data ?? [];
  const [selectedApplicationId, setSelectedApplicationId] = useState<number>();
  const currentApplication =
    applications.find(
      (application) => application.Id === selectedApplicationId
    ) ?? applications[0];
  const servicesQuery = usePlatformServices(currentApplication?.Id);
  const services = servicesQuery.data ?? [];
  const [selectedServiceId, setSelectedServiceId] = useState<number>();
  const currentService =
    services.find((service) => service.Id === selectedServiceId) ?? services[0];
  const [name, setName] = useState('');
  const [version, setVersion] = useState('');
  const [imageRef, setImageRef] = useState('');
  const [imageDigest, setImageDigest] = useState('');
	const [source, setSource] = useState<'image' | 'upload'>('image');
	const [uploadType, setUploadType] = useState<
		'java-jar' | 'frontend-dist' | 'docker-image-tar' | 'oci-archive'
	>('java-jar');
	const [uploadFile, setUploadFile] = useState<File>();
	const [expectedSHA256, setExpectedSHA256] = useState('');

  async function handleSubmit(event: FormEvent) {
	 event.preventDefault();
	 if (!selectedProjectId || !currentService) {
		return;
	 }
	 if (source === 'upload') {
		if (!uploadFile) {
			return;
		}
		await uploadMutation.mutateAsync({
			ProjectId: selectedProjectId,
			ApplicationId: currentApplication?.Id,
			ServiceDefinitionId: currentService.Id,
			Name: name.trim(),
			Version: version.trim(),
			Type: uploadType,
			ExpectedSHA256: expectedSHA256.trim() || undefined,
			File: uploadFile,
		});
		setUploadFile(undefined);
		setExpectedSHA256('');
		onDone();
		return;
	 }
	 await mutation.mutateAsync({
		ProjectId: selectedProjectId,
        ApplicationId: currentApplication?.Id,
        ServiceDefinitionId: currentService?.Id,
        Name: name.trim(),
        Version: version.trim(),
        ImageRef: imageRef.trim(),
        ImageDigest: imageDigest.trim() || undefined,
        Traceability: 'weak',
	 });
	 setName('');
	 setVersion('');
	 setImageRef('');
	 setImageDigest('');
	 onDone();
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.artifact', {
        defaultValue: 'Register image artifact',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={handleSubmit}>
		<label className="form-control-label self-end">
		  {t('platform.forms.artifactSource', {
			defaultValue: 'Artifact source',
		  })}
		  <select
			className="form-control mt-1"
			value={source}
			onChange={(event) =>
			  setSource(event.target.value as 'image' | 'upload')
			}
		  >
			<option value="image">
			  {t('platform.artifacts.sourceImage', {
				defaultValue: 'Existing image reference',
			  })}
			</option>
			<option value="upload">
			  {t('platform.artifacts.sourceUpload', {
				defaultValue: 'Upload Jar, dist ZIP, or image archive',
			  })}
			</option>
		  </select>
		</label>
		<SelectField
          label={t('platform.forms.project', { defaultValue: 'Project' })}
          value={selectedProjectId}
          disabled={projects.length === 0}
          onChange={(projectId) => {
            setSelectedProjectId(projectId);
            setSelectedApplicationId(undefined);
            setSelectedServiceId(undefined);
          }}
        >
          {projects.map((project) => (
            <option key={project.Id} value={project.Id}>
              {project.Name}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('platform.forms.application', {
            defaultValue: 'Application',
          })}
          value={currentApplication?.Id}
          disabled={applications.length === 0}
          onChange={(applicationId) => {
            setSelectedApplicationId(applicationId);
            setSelectedServiceId(undefined);
          }}
        >
          {applications.map((application) => (
            <option key={application.Id} value={application.Id}>
              {application.Name}
            </option>
          ))}
        </SelectField>
        <SelectField
          label={t('platform.forms.service', { defaultValue: 'Service' })}
          value={currentService?.Id}
          disabled={services.length === 0}
          onChange={setSelectedServiceId}
        >
          {services.map((service) => (
            <option key={service.Id} value={service.Id}>
              {service.Name}
            </option>
          ))}
        </SelectField>
        <TextInputField
          label={t('platform.forms.artifactName', {
            defaultValue: 'Artifact name',
          })}
          value={name}
          required
          onChange={setName}
        />
        <TextInputField
          label={t('platform.forms.version', { defaultValue: 'Version' })}
          value={version}
          required
          onChange={setVersion}
        />
		{source === 'image' ? (
		  <>
			<TextInputField
			  label={t('platform.forms.imageRef', {
				defaultValue: 'Image reference',
			  })}
			  value={imageRef}
			  required
			  onChange={setImageRef}
			/>
			<TextInputField
			  label={t('platform.forms.imageDigest', {
				defaultValue: 'Image digest',
			  })}
			  value={imageDigest}
			  onChange={setImageDigest}
			/>
			<div className="text-muted self-end text-sm">
			  {t('platform.forms.traceabilityWeak', {
				defaultValue: 'Traceability: weak / image-reference only',
			  })}
			</div>
		  </>
		) : (
		  <>
			<label className="form-control-label self-end">
			  {t('platform.forms.artifactType', {
				defaultValue: 'Artifact type',
			  })}
			  <select
				className="form-control mt-1"
				value={uploadType}
				onChange={(event) =>
				  setUploadType(event.target.value as typeof uploadType)
				}
			  >
				<option value="java-jar">
					{t('platform.artifacts.types.javaJar', {
						defaultValue: 'Java 8 Jar',
					})}
				</option>
				<option value="frontend-dist">
					{t('platform.artifacts.types.frontendDist', {
						defaultValue: 'Frontend dist ZIP',
					})}
				</option>
				<option value="docker-image-tar">
					{t('platform.artifacts.types.dockerTar', {
						defaultValue: 'Docker image tar',
					})}
				</option>
				<option value="oci-archive">
					{t('platform.artifacts.types.ociArchive', {
						defaultValue: 'OCI archive',
					})}
				</option>
			  </select>
			</label>
			<label className="form-control-label self-end">
			  {t('platform.forms.artifactFile', {
				defaultValue: 'Artifact file',
			  })}
			  <input
				type="file"
				className="form-control mt-1"
				accept=".jar,.zip,.tar"
				onChange={(event) => setUploadFile(event.target.files?.[0])}
			  />
			</label>
			<TextInputField
			  label={t('platform.forms.expectedSHA256', {
				defaultValue: 'Expected SHA256 (optional)',
			  })}
			  value={expectedSHA256}
			  onChange={setExpectedSHA256}
			/>
		  </>
		)}
		<FormActions
		  isSubmitting={mutation.isLoading || uploadMutation.isLoading}
          submitLabel={t('platform.actions.createArtifact', {
            defaultValue: 'Register image artifact',
          })}
          submitDisabled={
            !selectedProjectId ||
            !name.trim() ||
            !version.trim() ||
			(source === 'image' ? !imageRef.trim() : !uploadFile)
          }
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function PlatformPage({
  titleKey,
  titleDefault,
  children,
}: {
  titleKey: string;
  titleDefault: string;
  children: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <>
      <PageHeader
        title={t(titleKey, { defaultValue: titleDefault })}
        breadcrumbs={t('platform.navigation.section', {
          defaultValue: 'App Delivery',
        })}
        reload
      />
      <main className="pb-6">{children}</main>
    </>
  );
}

function PlatformNoticeStack() {
  const { t } = useTranslation();

  return (
    <div className="mx-4 mb-4 space-y-3">
      <Alert color="warn" title={t('platform.alerts.productionRisk.title')}>
        {t('platform.alerts.productionRisk.body', {
          defaultValue:
            'V0.1 is a technical preview. Production deployments must assume short downtime and should not be treated as lossless releases.',
        })}
      </Alert>
      <Alert color="default" title={t('platform.alerts.roleAccess.title')}>
        {t('platform.alerts.roleAccess.body')}
      </Alert>
    </div>
  );
}

function ActionBar({ children }: { children: ReactNode }) {
  return <div className="mx-4 mb-4 flex flex-wrap gap-2">{children}</div>;
}

function ActionPanel({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="mx-4 mb-4 rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
      <h2 className="mb-3 text-lg font-semibold">{title}</h2>
      {children}
    </section>
  );
}

function TextInputField({
  label,
  value,
  onChange,
  required,
  inputMode,
  type,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  inputMode?: 'numeric';
  type?: 'password';
}) {
  return (
    <label className="block">
      <span className="text-muted mb-1 block text-sm">{label}</span>
      <input
        className="form-control"
        type={type}
        value={value}
        required={required}
        inputMode={inputMode}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function NumberInputField({
  label,
  value,
  onChange,
  min = 1,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min?: number;
}) {
  return (
    <label className="block">
      <span className="text-muted mb-1 block text-sm">{label}</span>
      <input
        className="form-control"
        type="number"
        min={min}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  );
}

function TextAreaField({
  label,
  value,
  onChange,
  rows = 6,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  rows?: number;
  placeholder?: string;
}) {
  return (
    <label className="block">
      <span className="text-muted mb-1 block text-sm">{label}</span>
      <textarea
        className="form-control"
        rows={rows}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function SelectField({
  label,
  value,
  disabled,
  onChange,
  children,
}: {
  label: string;
  value?: number;
  disabled: boolean;
  onChange: (value: number | undefined) => void;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="text-muted mb-1 block text-sm">{label}</span>
      <select
        className="form-control"
        value={value ?? ''}
        onChange={(event) =>
          onChange(event.target.value ? Number(event.target.value) : undefined)
        }
        disabled={disabled}
      >
        {children}
      </select>
    </label>
  );
}

function StringSelectField({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<[string, string]>;
  onChange: (value: string) => void;
}) {
  return (
    <label className="block">
      <span className="text-muted mb-1 block text-sm">{label}</span>
      <select
        className="form-control"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map(([optionValue, optionLabel]) => (
          <option key={optionValue} value={optionValue}>
            {optionLabel}
          </option>
        ))}
      </select>
    </label>
  );
}

function FormActions({
  isSubmitting,
  submitLabel,
  submitDisabled,
  onCancel,
}: {
  isSubmitting: boolean;
  submitLabel: string;
  submitDisabled?: boolean;
  onCancel: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex items-end gap-2 self-end md:col-span-3">
      <Button
        color="primary"
        icon={Save}
        type="submit"
        disabled={isSubmitting || submitDisabled}
        data-cy="platform-form-submit"
      >
        {submitLabel}
      </Button>
      <Button
        color="light"
        type="button"
        onClick={onCancel}
        data-cy="platform-form-cancel"
      >
        {t('platform.actions.cancel', { defaultValue: 'Cancel' })}
      </Button>
    </div>
  );
}

function SummaryStrip({
  items,
}: {
  items: Array<{ label: string; value: number }>;
}) {
  return (
    <div className="mx-4 mb-4 grid gap-3 md:grid-cols-3">
      {items.map((item) => (
        <div
          key={item.label}
          className="rounded border border-solid border-gray-5 bg-white p-3 th-highcontrast:bg-black th-dark:bg-gray-11"
        >
          <div className="text-muted text-xs uppercase">{item.label}</div>
          <div className="mt-1 text-2xl font-semibold">{item.value}</div>
        </div>
      ))}
    </div>
  );
}

function DataSection({
  title,
  isLoading,
  empty,
  emptyMessage,
  children,
}: {
  title: string;
  isLoading: boolean;
  empty: boolean;
  emptyMessage: string;
  children: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <section className="mx-4 mb-4 rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
      <h2 className="mb-3 text-lg font-semibold">{title}</h2>
      {isLoading && (
        <div className="text-muted text-sm">
          {t('common.loading', { defaultValue: 'Loading...' })}
        </div>
      )}
      {!isLoading && empty && (
        <div className="text-muted rounded border border-dashed border-gray-5 p-4 text-sm">
          {emptyMessage}
        </div>
      )}
      {!isLoading && !empty && children}
    </section>
  );
}

function PlatformTable({
  columns,
  rows,
}: {
  columns: string[];
  rows: Array<{ key: string; cells: ReactNode[] }>;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="table-hover table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key}>
              {row.cells.map((cell, index) => (
                <td
                  key={`${row.key}-${index}`}
                  className="max-w-[360px] break-all"
                >
                  {cell === undefined || cell === null || cell === ''
                    ? '-'
                    : cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EnvironmentsTable({
  environments,
}: {
  environments: PlatformEnvironment[];
}) {
  const { t } = useTranslation();

  return (
    <PlatformTable
      columns={[
        t('platform.columns.name', { defaultValue: 'Name' }),
        t('platform.columns.slug', { defaultValue: 'Slug' }),
        t('platform.columns.type', { defaultValue: 'Type' }),
        t('platform.columns.target', { defaultValue: 'Target' }),
        t('platform.columns.status', { defaultValue: 'Status' }),
      ]}
      rows={environments.map((environment) => ({
        key: String(environment.Id),
        cells: [
          environment.Name,
          environment.Slug,
          environment.Type,
          formatEnvironmentTargets(environment.Targets),
          environment.LifecycleStatus,
        ],
      }))}
    />
  );
}

function ApplicationsTable({
  applications,
}: {
  applications: PlatformApplication[];
}) {
  const { t } = useTranslation();
  return (
    <PlatformTable
      columns={[
        t('platform.columns.name', { defaultValue: 'Name' }),
        t('platform.columns.slug', { defaultValue: 'Slug' }),
        t('platform.columns.status', { defaultValue: 'Status' }),
      ]}
      rows={applications.map((application) => ({
        key: String(application.Id),
        cells: [
          application.Name,
          application.Slug,
          application.LifecycleStatus,
        ],
      }))}
    />
  );
}

function ServicesTable({
  services,
}: {
  services: PlatformServiceDefinition[];
}) {
  const { t } = useTranslation();
  return (
    <PlatformTable
      columns={[
        t('platform.columns.name', { defaultValue: 'Name' }),
        t('platform.columns.type', { defaultValue: 'Type' }),
        t('platform.columns.slug', { defaultValue: 'Slug' }),
        t('platform.columns.status', { defaultValue: 'Status' }),
      ]}
      rows={services.map((service) => ({
        key: String(service.Id),
        cells: [
          service.Name,
          service.Type,
          service.Slug,
          service.LifecycleStatus,
        ],
      }))}
    />
  );
}

function AuditLogsTable({ logs }: { logs: PlatformAuditLog[] }) {
  const { t } = useTranslation();
  return (
    <PlatformTable
      columns={[
        t('platform.audit.columns.time'),
        t('platform.audit.columns.action'),
        t('platform.audit.columns.result'),
        t('platform.audit.columns.operator'),
        t('platform.audit.columns.details'),
      ]}
      rows={logs.map((log) => ({
        key: String(log.Id),
        cells: [
          formatUnixTime(log.Timestamp),
          log.Action,
          <StatusPill key="result" value={log.Result} />,
          log.OperatorUsername ?? String(log.OperatorUserId),
          [
            log.FailureReason,
            log.SensitiveFields?.length
              ? t('platform.audit.sensitiveFields', {
                  fields: log.SensitiveFields.join(', '),
                })
              : '',
          ]
            .filter(Boolean)
            .join(' / '),
        ],
      }))}
    />
  );
}

function JavaBuildPanel({ artifacts, onDone }: { artifacts: PlatformArtifact[]; onDone: () => void }) {
  const { t } = useTranslation();
  const mutation = useBuildJavaArtifactMutation();
  const javaArtifacts = artifacts.filter((artifact) => artifact.Type === 'java-jar' && artifact.Status !== 'building');
  const [artifactId, setArtifactId] = useState<number | undefined>(javaArtifacts[0]?.Id);
  const [endpointId, setEndpointId] = useState('');
  const [port, setPort] = useState('8080');
  const [jvmArgs, setJvmArgs] = useState('');
  const [appArgs, setAppArgs] = useState('');
  const tokens = (value: string) => value.split(/\s+/).map((item) => item.trim()).filter(Boolean);
  async function submit(event: FormEvent) { event.preventDefault(); if (!artifactId || !Number(endpointId) || !Number(port)) return; await mutation.mutateAsync({ artifactId, payload: { EndpointId: Number(endpointId), Port: Number(port), JvmArgs: tokens(jvmArgs), AppArgs: tokens(appArgs) } }); onDone(); }
  return <ActionPanel title={t('platform.formTitles.javaBuild', { defaultValue: 'Package Java 8 artifact' })}><form className="grid gap-3 md:grid-cols-3" onSubmit={submit}>
    <label className="form-control-label">{t('platform.forms.javaArtifact', { defaultValue: 'Java artifact' })}<select className="form-control mt-1" value={artifactId} onChange={(event) => setArtifactId(Number(event.target.value))}>{javaArtifacts.map((artifact) => <option key={artifact.Id} value={artifact.Id}>{artifact.Name} · {artifact.Version}</option>)}</select></label>
    <TextInputField label={t('platform.forms.endpointId', { defaultValue: 'Docker endpoint ID' })} value={endpointId} required onChange={setEndpointId} />
    <TextInputField label={t('platform.forms.javaPort', { defaultValue: 'Container port' })} value={port} required onChange={setPort} />
    <TextInputField label={t('platform.forms.jvmArgs', { defaultValue: 'JVM arguments (space-separated tokens)' })} value={jvmArgs} onChange={setJvmArgs} />
    <TextInputField label={t('platform.forms.appArgs', { defaultValue: 'Application arguments (space-separated tokens)' })} value={appArgs} onChange={setAppArgs} />
    <div className="text-muted self-end text-sm">{t('platform.javaBuild.restriction', { defaultValue: 'Uses the platform Java 8 template. Dockerfile, shell commands, and custom base images are not accepted.' })}</div>
    <FormActions isSubmitting={mutation.isLoading} submitLabel={t('platform.actions.packageJava', { defaultValue: 'Package Java 8 artifact' })} submitDisabled={!artifactId || !endpointId || !port} onCancel={onDone} />
  </form></ActionPanel>;
}

function StaticBuildPanel({
  artifacts,
  onDone,
}: {
  artifacts: PlatformArtifact[];
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = useBuildStaticArtifactMutation();
  const [artifactId, setArtifactId] = useState<number | undefined>(
    artifacts[0]?.Id
  );
  const [endpointId, setEndpointId] = useState('');
  const [mode, setMode] = useState<'spa' | 'mpa'>('spa');
  const [cachePolicy, setCachePolicy] = useState<
    '' | 'no-cache' | 'immutable'
  >('');
  const [notFoundPage, setNotFoundPage] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!artifactId || !Number(endpointId)) {
      return;
    }
    await mutation.mutateAsync({
      artifactId,
      payload: {
        EndpointId: Number(endpointId),
        Mode: mode,
        CachePolicy: cachePolicy,
        NotFoundPage: notFoundPage.trim() || undefined,
      },
    });
    onDone();
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.staticBuild', {
        defaultValue: 'Package frontend dist',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={submit}>
        <label className="form-control-label">
          {t('platform.forms.staticArtifact', {
            defaultValue: 'Frontend dist artifact',
          })}
          <select
            className="form-control mt-1"
            value={artifactId}
            onChange={(event) => setArtifactId(Number(event.target.value))}
          >
            {artifacts.map((artifact) => (
              <option key={artifact.Id} value={artifact.Id}>
                {artifact.Name} · {artifact.Version}
              </option>
            ))}
          </select>
        </label>
        <TextInputField
          label={t('platform.forms.endpointId', {
            defaultValue: 'Docker endpoint ID',
          })}
          value={endpointId}
          required
          onChange={setEndpointId}
        />
        <label className="form-control-label">
          {t('platform.forms.staticMode', { defaultValue: 'Site mode' })}
          <select
            className="form-control mt-1"
            value={mode}
            onChange={(event) => setMode(event.target.value as 'spa' | 'mpa')}
          >
            <option value="spa">
              {t('platform.staticBuild.spa', { defaultValue: 'SPA' })}
            </option>
            <option value="mpa">
              {t('platform.staticBuild.mpa', {
                defaultValue: 'MPA / static files',
              })}
            </option>
          </select>
        </label>
        <label className="form-control-label">
          {t('platform.forms.staticCachePolicy', {
            defaultValue: 'Static cache policy',
          })}
          <select
            className="form-control mt-1"
            value={cachePolicy}
            onChange={(event) =>
              setCachePolicy(
                event.target.value as '' | 'no-cache' | 'immutable'
              )
            }
          >
            <option value="">
              {t('platform.staticBuild.cacheDefault', {
                defaultValue: 'Platform default',
              })}
            </option>
            <option value="no-cache">
              {t('platform.staticBuild.cacheNoStore', {
                defaultValue: 'No-store',
              })}
            </option>
            <option value="immutable">
              {t('platform.staticBuild.cacheImmutable', {
                defaultValue: 'Immutable static assets',
              })}
            </option>
          </select>
        </label>
        <TextInputField
          label={t('platform.forms.staticNotFoundPage', {
            defaultValue: 'Optional 404 page in dist',
          })}
          value={notFoundPage}
          onChange={setNotFoundPage}
        />
        <div className="text-muted self-end text-sm">
          {t('platform.staticBuild.restriction', {
            defaultValue:
              'SPA requires index.html. The platform validates ZIP entries and uses a fixed Nginx image; custom Nginx configuration, Node, and SSR are not accepted.',
          })}
        </div>
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.packageStatic', {
            defaultValue: 'Package frontend dist',
          })}
          submitDisabled={!artifactId || !endpointId}
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function ArtifactPushPanel({
  artifacts,
  onDone,
}: {
  artifacts: PlatformArtifact[];
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = usePushPlatformArtifactMutation();
  const [artifactId, setArtifactId] = useState<number | undefined>(
    artifacts[0]?.Id
  );
  const [endpointId, setEndpointId] = useState('');
  const [registryId, setRegistryId] = useState('');

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!artifactId || !Number(endpointId) || !Number(registryId)) {
      return;
    }
    await mutation.mutateAsync({
      artifactId,
      payload: {
        EndpointId: Number(endpointId),
        RegistryId: Number(registryId),
      },
    });
    onDone();
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.artifactPush', {
        defaultValue: 'Push candidate image',
      })}
    >
      <form className="grid gap-3 md:grid-cols-3" onSubmit={submit}>
        <label className="form-control-label">
          {t('platform.forms.pushArtifact', {
            defaultValue: 'Candidate artifact',
          })}
          <select
            className="form-control mt-1"
            value={artifactId}
            onChange={(event) => setArtifactId(Number(event.target.value))}
          >
            {artifacts.map((artifact) => (
              <option key={artifact.Id} value={artifact.Id}>
                {artifact.Name} · {artifact.Version}
              </option>
            ))}
          </select>
        </label>
        <TextInputField
          label={t('platform.forms.endpointId', {
            defaultValue: 'Docker endpoint ID',
          })}
          value={endpointId}
          required
          onChange={setEndpointId}
        />
        <TextInputField
          label={t('platform.forms.registryId', {
            defaultValue: 'Configured registry ID',
          })}
          value={registryId}
          required
          onChange={setRegistryId}
        />
        <div className="text-muted self-end text-sm">
          {t('platform.artifactPush.restriction', {
            defaultValue:
              'The platform re-tags the candidate with a unique project/service/version tag and records the registry digest. Credentials remain in the configured Portainer registry.',
          })}
        </div>
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.pushArtifact', {
            defaultValue: 'Push candidate image',
          })}
          submitDisabled={!artifactId || !endpointId || !registryId}
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function ArtifactCleanupPanel({
  artifacts,
  onDone,
}: {
  artifacts: PlatformArtifact[];
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const mutation = useCleanupPlatformArtifactOriginalMutation();
  const [artifactId, setArtifactId] = useState<number | undefined>(
    artifacts[0]?.Id
  );
  const [confirmed, setConfirmed] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!artifactId || !confirmed) {
      return;
    }
    await mutation.mutateAsync(artifactId);
    onDone();
  }

  return (
    <ActionPanel
      title={t('platform.formTitles.artifactCleanup', {
        defaultValue: 'Clean original artifact',
      })}
    >
      <form className="grid gap-3 md:grid-cols-2" onSubmit={submit}>
        <label className="form-control-label">
          {t('platform.forms.cleanupArtifact', {
            defaultValue: 'Original artifact',
          })}
          <select
            className="form-control mt-1"
            value={artifactId}
            onChange={(event) => setArtifactId(Number(event.target.value))}
          >
            {artifacts.map((artifact) => (
              <option key={artifact.Id} value={artifact.Id}>
                {artifact.Name} · {artifact.Version}
              </option>
            ))}
          </select>
        </label>
        <Alert
          color="warn"
          title={t('platform.artifactCleanup.warningTitle', {
            defaultValue: 'Delete only the retained original',
          })}
        >
          {t('platform.artifactCleanup.warningBody', {
            defaultValue:
              'The final registry image and Artifact facts are retained. The server blocks cleanup for active tasks and Release references.',
          })}
        </Alert>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={confirmed}
            onChange={(event) => setConfirmed(event.target.checked)}
          />
          {t('platform.artifactCleanup.confirm', {
            defaultValue: 'I understand this removes the retained original file.',
          })}
        </label>
        <FormActions
          isSubmitting={mutation.isLoading}
          submitLabel={t('platform.actions.cleanupArtifactOriginal', {
            defaultValue: 'Clean original artifact',
          })}
          submitDisabled={!artifactId || !confirmed}
          onCancel={onDone}
        />
      </form>
    </ActionPanel>
  );
}

function ArtifactsTable({ artifacts }: { artifacts: PlatformArtifact[] }) {
  const { t } = useTranslation();
  return (
    <PlatformTable
      columns={[
        t('platform.columns.name', { defaultValue: 'Name' }),
        t('platform.columns.version', { defaultValue: 'Version' }),
		t('platform.columns.type', { defaultValue: 'Type' }),
        t('platform.columns.source', { defaultValue: 'Source' }),
        t('platform.columns.status', { defaultValue: 'Status' }),
        t('platform.columns.image', { defaultValue: 'Image' }),
		t('platform.columns.imageDigest', { defaultValue: 'Image digest' }),
		t('platform.columns.retention', { defaultValue: 'Retention' }),
        t('platform.columns.sha256', { defaultValue: 'SHA256' }),
		t('platform.columns.failureReason', { defaultValue: 'Failure reason' }),
      ]}
      rows={artifacts.map((artifact) => ({
        key: String(artifact.Id),
        cells: [
          artifact.Name,
          artifact.Version,
			artifact.Type,
          artifact.SourceType,
          <StatusPill key="status" value={artifact.Status ?? ''} />,
          artifact.CandidateImageRef ?? artifact.ImageRef ?? '',
			artifact.ImageDigest ?? '',
			artifact.Retained
				? artifact.Cleanable
					? t('platform.artifacts.retention.cleanable', {
						defaultValue: 'Retained / cleanable',
					})
					: t('platform.artifacts.retention.protected', {
						defaultValue: 'Retained / protected',
					})
				: t('platform.artifacts.retention.cleaned', {
						defaultValue: 'Original cleaned',
					}),
          artifact.SHA256 ?? artifact.ImageDigest ?? '',
			artifact.FailureReason ?? '',
        ],
      }))}
    />
  );
}

function ReleasesTable({
  releases,
  projects,
}: {
  releases: PlatformRelease[];
  projects: PlatformProject[];
}) {
  const { t } = useTranslation();
  return (
    <PlatformTable
      columns={[
        t('platform.columns.releaseId', { defaultValue: 'Release ID' }),
        t('platform.columns.version', { defaultValue: 'Version' }),
        t('platform.columns.status', { defaultValue: 'Status' }),
        t('platform.columns.image', { defaultValue: 'Image' }),
        t('platform.columns.failureReason', { defaultValue: 'Failure reason' }),
        t('platform.columns.runtime', { defaultValue: 'Runtime' }),
        t('platform.columns.execution', { defaultValue: 'Execution' }),
        t('platform.columns.actions', { defaultValue: 'Actions' }),
      ]}
      rows={releases.map((release) => ({
        key: String(release.Id),
        cells: [
          String(release.Id),
          release.Version,
          <StatusPill key="status" value={release.Status} />,
          release.Image ?? '',
          release.FailureReason ?? '',
          <RuntimeRefSummary
            key="runtime"
            runtimeRef={release.RuntimeSnapshot?.CurrentRuntimeRef}
            ports={release.RuntimeSnapshot?.PublishedPorts}
          />,
          <ReleaseExecutionSummary key="execution" release={release} />,
          <ReleaseActions
            key="actions"
            release={release}
            canManage={
              !!projects.find((project) => project.Id === release.ProjectId)
                ?.Permissions?.CanManageResources
            }
            canRollback={
              !!projects.find((project) => project.Id === release.ProjectId)
                ?.Permissions?.CanDeploy
            }
          />,
        ],
      }))}
    />
  );
}

function ConfigSetsTable({
  configSets,
  canManage,
  onEdit,
  onArchive,
  isArchiving,
}: {
  configSets: PlatformConfigSet[];
  canManage: boolean;
  onEdit: (configSet: PlatformConfigSet) => void;
  onArchive: (configSet: PlatformConfigSet) => void;
  isArchiving: boolean;
}) {
  const { t } = useTranslation();

  return (
    <PlatformTable
      columns={[
        t('platform.columns.name', { defaultValue: 'Name' }),
        t('platform.config.revision', { defaultValue: 'Revision' }),
        t('platform.config.entries', { defaultValue: 'Entries' }),
        t('platform.columns.status', { defaultValue: 'Status' }),
        t('platform.columns.actions', { defaultValue: 'Actions' }),
      ]}
      rows={configSets.map((configSet) => ({
        key: String(configSet.Id),
        cells: [
          configSet.Name,
          String(configSet.Revision),
          `${configSet.Entries.length}`,
          configSet.LifecycleStatus,
          canManage ? (
            <div
              key={`actions-${configSet.Id}`}
              className="flex flex-wrap gap-2"
            >
              <Button
                color="light"
                size="xsmall"
                onClick={() => onEdit(configSet)}
                data-cy={`platform-config-set-${configSet.Id}-edit`}
              >
                {t('platform.actions.edit', { defaultValue: 'Edit' })}
              </Button>
              <Button
                color="dangerlight"
                size="xsmall"
                disabled={isArchiving}
                onClick={() => onArchive(configSet)}
                data-cy={`platform-config-set-${configSet.Id}-archive`}
              >
                {t('platform.actions.archive', { defaultValue: 'Archive' })}
              </Button>
            </div>
          ) : (
            '-'
          ),
        ],
      }))}
    />
  );
}

// 普通更新只回传敏感条目的元数据，不回填其值；替换敏感值必须显式提交键和值，
// 避免脱敏 API 响应在编辑保存时意外清空已经加密保存的旧值。
function ConfigSetEditor({
  configSet,
  projectId,
  scopeType,
  scopeId,
  canReveal,
  onDone,
}: {
  configSet?: PlatformConfigSet;
  projectId: number;
  scopeType: PlatformConfigScopeType;
  scopeId: number;
  canReveal: boolean;
  onDone: (configSetId?: number) => void;
}) {
  const { t } = useTranslation();
  const createMutation = useCreatePlatformConfigSetMutation();
  const updateMutation = useUpdatePlatformConfigSetMutation();
  const secretMutation = useReadPlatformConfigSecretMutation();
  const [name, setName] = useState(configSet?.Name ?? 'default');
  const [plainEntriesText, setPlainEntriesText] = useState('');
  const [secretKey, setSecretKey] = useState('');
  const [secretValue, setSecretValue] = useState('');
  const [revealedValues, setRevealedValues] = useState<Record<string, string>>(
    {}
  );

  useEffect(() => {
    setName(configSet?.Name ?? 'default');
    setPlainEntriesText(formatPlainConfigEntries(configSet?.Entries ?? []));
    setSecretKey('');
    setSecretValue('');
    setRevealedValues({});
  }, [configSet]);

  const sensitiveEntries = (configSet?.Entries ?? []).filter(
    (entry) => entry.Sensitive
  );
  const isSubmitting = createMutation.isLoading || updateMutation.isLoading;

  function configEntries(): PlatformConfigEntry[] {
    const entries = parsePlainConfigEntries(plainEntriesText);
    const preservedSecrets = sensitiveEntries.map((entry) => ({
      Key: entry.Key,
      ValueType: entry.ValueType,
      Sensitive: true,
      Required: entry.Required,
      HasValue: entry.HasValue,
    }));
    if (secretKey.trim()) {
      return [
        ...entries.filter((entry) => entry.Key !== secretKey.trim()),
        ...preservedSecrets.filter((entry) => entry.Key !== secretKey.trim()),
        {
          Key: secretKey.trim(),
          ValueType: 'plain',
          Value: secretValue,
          Sensitive: true,
          Required: true,
        },
      ];
    }
    return [...entries, ...preservedSecrets];
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const entries = configEntries();
    if (configSet) {
      updateMutation.mutate(
        {
          configSetId: configSet.Id,
          payload: {
            ResourceVersion: configSet.ResourceVersion,
            Name: name.trim(),
            Entries: entries,
          },
        },
        {
          onSuccess: (updated) => onDone(updated.Id),
        }
      );
      return;
    }
    createMutation.mutate(
      {
        ProjectId: projectId,
        ScopeType: scopeType,
        ScopeId: scopeId,
        Name: name.trim(),
        Entries: entries,
      },
      {
        onSuccess: (created) => onDone(created.Id),
      }
    );
  }

  async function readSensitive(
    entry: PlatformConfigEntry,
    action: 'reveal' | 'copy'
  ) {
    if (!configSet) {
      return;
    }
    const message =
      action === 'reveal'
        ? t('platform.config.revealConfirm', {
            defaultValue:
              'Reveal {{key}} only when it is safe to display the value. Continue?',
            key: entry.Key,
          })
        : t('platform.config.copyConfirm', {
            defaultValue:
              'Copy {{key}} to the system clipboard only when it is safe. Continue?',
            key: entry.Key,
          });
    if (!window.confirm(message)) {
      return;
    }
    const value = await secretMutation.mutateAsync({
      configSetId: configSet.Id,
      key: entry.Key,
      action,
    });
    if (action === 'reveal') {
      setRevealedValues((values) => ({ ...values, [entry.Key]: value }));
      return;
    }
    await navigator.clipboard?.writeText(value);
  }

  return (
    <section className="mx-4 mb-4 rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
      <h2 className="mb-3 text-lg font-semibold">
        {configSet
          ? t('platform.config.editTitle', { defaultValue: 'Edit config set' })
          : t('platform.config.createTitle', {
              defaultValue: 'Create config set',
            })}
      </h2>
      <form className="grid gap-4 md:grid-cols-2" onSubmit={handleSubmit}>
        <TextInputField
          label={t('platform.forms.name', { defaultValue: 'Name' })}
          value={name}
          onChange={setName}
          required
        />
        <div className="text-muted self-end text-sm">
          {t('platform.config.scopeSummary', {
            defaultValue: 'Scope: {{scope}} #{{id}}',
            scope: scopeType,
            id: scopeId,
          })}
        </div>
        <div className="md:col-span-2">
          <TextAreaField
            label={t('platform.config.plainEntries', {
              defaultValue: 'Plain configuration entries',
            })}
            value={plainEntriesText}
            onChange={setPlainEntriesText}
            placeholder="APP_ENV=production"
          />
        </div>
        <TextInputField
          label={t('platform.config.sensitiveKey', {
            defaultValue: 'Sensitive variable key',
          })}
          value={secretKey}
          onChange={setSecretKey}
        />
        <TextInputField
          label={t('platform.config.sensitiveValue', {
            defaultValue: 'Sensitive variable value',
          })}
          value={secretValue}
          onChange={setSecretValue}
          type="password"
        />
        <div className="md:col-span-2">
          <Alert
            color="default"
            title={t('platform.config.sensitiveNoticeTitle', {
              defaultValue: 'Sensitive values are encrypted',
            })}
          >
            {t('platform.config.sensitiveNoticeBody', {
              defaultValue:
                'Sensitive values are sent only to the save request, encrypted by the server, and then cleared from this form. API lists remain redacted.',
            })}
          </Alert>
        </div>
        {sensitiveEntries.length > 0 && (
          <div className="md:col-span-2">
            <div className="mb-2 text-sm font-semibold">
              {t('platform.config.savedSensitive', {
                defaultValue: 'Saved sensitive variables',
              })}
            </div>
            <div className="space-y-2">
              {sensitiveEntries.map((entry) => (
                <div
                  key={entry.Key}
                  className="flex flex-wrap items-center gap-2 rounded border border-solid border-gray-5 p-2 text-sm"
                >
                  <span className="font-medium">{entry.Key}</span>
                  <span className="text-muted">
                    {entry.HasValue
                      ? t('platform.config.hasValue', {
                          defaultValue: 'value saved',
                        })
                      : t('platform.config.noValue', {
                          defaultValue: 'no value',
                        })}
                  </span>
                  {canReveal && configSet && (
                    <>
                      <Button
                        color="light"
                        size="xsmall"
                        disabled={secretMutation.isLoading}
                        onClick={() => readSensitive(entry, 'reveal')}
                        data-cy={`platform-config-${configSet.Id}-${entry.Key}-reveal`}
                      >
                        {t('platform.actions.reveal', {
                          defaultValue: 'Reveal',
                        })}
                      </Button>
                      <Button
                        color="light"
                        size="xsmall"
                        disabled={secretMutation.isLoading}
                        onClick={() => readSensitive(entry, 'copy')}
                        data-cy={`platform-config-${configSet.Id}-${entry.Key}-copy`}
                      >
                        {t('platform.actions.copy', { defaultValue: 'Copy' })}
                      </Button>
                    </>
                  )}
                  {revealedValues[entry.Key] && (
                    <code className="max-w-full break-all rounded bg-gray-2 px-2 py-1 th-dark:bg-gray-10">
                      {revealedValues[entry.Key]}
                    </code>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
        <FormActions
          isSubmitting={isSubmitting}
          submitDisabled={!name.trim() || (!!secretKey.trim() && !secretValue)}
          submitLabel={
            configSet
              ? t('platform.actions.save', { defaultValue: 'Save' })
              : t('platform.actions.createConfigSet', {
                  defaultValue: 'Create config set',
                })
          }
          onCancel={() => onDone(configSet?.Id)}
        />
      </form>
    </section>
  );
}

function EffectiveConfigDetails({
  response,
  isLoading,
  emptyMessage,
}: {
  response?: PlatformEffectiveConfigResponse;
  isLoading: boolean;
  emptyMessage: string;
}) {
  const { t } = useTranslation();
  const entries = response?.EffectiveConfig.Entries ?? [];
  const revisions = Object.entries(
    response?.EffectiveConfig.ConfigSetRevisions ?? {}
  );

  return (
    <section className="rounded border border-solid border-gray-5 bg-white p-4 th-highcontrast:bg-black th-dark:bg-gray-11">
      <h2 className="mb-3 text-lg font-semibold">
        {t('platform.config.effectiveTitle', {
          defaultValue: 'Effective configuration preview',
        })}
      </h2>
      {isLoading && (
        <div className="text-muted text-sm">
          {t('common.loading', { defaultValue: 'Loading...' })}
        </div>
      )}
      {!isLoading && !response && (
        <div className="text-muted text-sm">{emptyMessage}</div>
      )}
      {!isLoading && response && (
        <div className="space-y-3">
          <SummaryList
            rows={[
              [
                t('platform.config.driftStatus', {
                  defaultValue: 'Drift status',
                }),
                response.DriftStatus || '-',
              ],
              [
                t('platform.config.configHash', {
                  defaultValue: 'Config hash',
                }),
                response.EffectiveConfig.Hash || '-',
              ],
              [
                t('platform.config.specRevision', {
                  defaultValue: 'Spec revision',
                }),
                String(response.EffectiveConfig.SpecRevision),
              ],
            ]}
          />
          {revisions.length > 0 && (
            <div className="text-muted text-xs">
              {t('platform.config.revisions', {
                defaultValue: 'Config set revisions: {{revisions}}',
                revisions: revisions
                  .map(([key, revision]) => `${key}: ${revision}`)
                  .join(', '),
              })}
            </div>
          )}
          {entries.length > 0 ? (
            <PlatformTable
              columns={[
                t('platform.config.key', { defaultValue: 'Key' }),
                t('platform.config.value', { defaultValue: 'Value' }),
                t('platform.config.source', { defaultValue: 'Source' }),
              ]}
              rows={entries.map((entry) => ({
                key: entry.Key,
                cells: [
                  entry.Key,
                  entry.Sensitive
                    ? t('platform.config.maskedValue', {
                        defaultValue: '••••••',
                      })
                    : (entry.Value ?? ''),
                  entry.Source,
                ],
              }))}
            />
          ) : (
            <div className="text-muted text-sm">
              {t('platform.config.noEffectiveEntries', {
                defaultValue: 'No effective configuration entries.',
              })}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function ServiceRuntimePanel({
  deployment,
  isDeploymentLoading,
}: {
  deployment?: PlatformServiceDeployment;
  isDeploymentLoading: boolean;
}) {
  const { t } = useTranslation();
  const statusQuery = usePlatformServiceDeploymentStatus(deployment?.Id);
  const logsQuery = usePlatformServiceDeploymentLogs(deployment?.Id, 100);
  const status = statusQuery.data;
  const logs = logsQuery.data;
  const runtimeRef = status?.RuntimeRef ?? deployment?.CurrentRuntimeRef;

  return (
    <DataSection
      title={t('platform.runtime.title', {
        defaultValue: 'Runtime status and logs',
      })}
      isLoading={
        isDeploymentLoading || statusQuery.isLoading || logsQuery.isLoading
      }
      empty={!deployment}
      emptyMessage={t('platform.empty.runtime', {
        defaultValue:
          'Select a service deployment config to inspect runtime status and recent logs.',
      })}
    >
      <div className="space-y-4">
        {status?.Reason === 'RUNTIME_MISSING' && (
          <Alert
            color="warn"
            title={t('platform.runtime.runtimeMissing.title', {
              defaultValue: 'Runtime missing',
            })}
          >
            {t('platform.runtime.runtimeMissing.body', {
              defaultValue:
                'The current runtime reference no longer exists. Drift has been marked as runtime-missing so an operator can resolve or redeploy safely.',
            })}
          </Alert>
        )}
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <div>
            <div className="mb-2 flex items-center gap-2 text-base font-semibold">
              <Activity className="icon" />
              {t('platform.runtime.status', { defaultValue: 'Status' })}
            </div>
            <SummaryList
              rows={[
                [
                  t('platform.runtime.deploymentId', {
                    defaultValue: 'Deployment ID',
                  }),
                  deployment ? String(deployment.Id) : '',
                ],
                [
                  t('platform.runtime.image', {
                    defaultValue: 'Current image',
                  }),
                  status?.CurrentImage ?? deployment?.CurrentImage ?? '',
                ],
                [
                  t('platform.runtime.runtimeRef', {
                    defaultValue: 'Runtime reference',
                  }),
                  runtimeRefLabel(runtimeRef),
                ],
                [
                  t('platform.runtime.state', {
                    defaultValue: 'Runtime state',
                  }),
                  status?.RuntimeState ?? status?.Reason ?? '',
                ],
                [
                  t('platform.runtime.restartCount', {
                    defaultValue: 'Restart count',
                  }),
                  status ? String(status.RestartCount) : '',
                ],
                [
                  t('platform.runtime.drift', { defaultValue: 'Drift status' }),
                  status?.DriftStatus ?? deployment?.DriftStatus ?? '',
                ],
                [
                  t('platform.runtime.ports', {
                    defaultValue: 'Published ports',
                  }),
                  formatPublishedPorts(
                    status?.PublishedPorts ?? deployment?.DesiredSpec.Ports
                  ),
                ],
                [
                  t('platform.runtime.lastDeployedAt', {
                    defaultValue: 'Last deployed at',
                  }),
                  formatUnixTime(status?.LastDeployedAt),
                ],
              ]}
            />
          </div>
          <div>
            <div className="mb-2 flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 text-base font-semibold">
                <FileText className="icon" />
                {t('platform.runtime.logs', { defaultValue: 'Recent logs' })}
              </div>
              <Button
                color="light"
                size="xsmall"
                icon={RefreshCw}
                onClick={() => logsQuery.refetch()}
                disabled={!deployment || logsQuery.isFetching}
                data-cy="platform-runtime-refresh-logs"
              >
                {t('platform.actions.refresh', { defaultValue: 'Refresh' })}
              </Button>
            </div>
            <pre className="max-h-72 overflow-auto rounded border border-solid border-gray-5 bg-gray-1 p-3 text-xs th-highcontrast:bg-black th-dark:bg-gray-10">
              {logs?.Logs ||
                logs?.Reason ||
                t('platform.runtime.logsEmpty', {
                  defaultValue:
                    'No logs are available for the selected runtime yet.',
                })}
            </pre>
          </div>
        </div>
      </div>
    </DataSection>
  );
}

function StatusPill({ value }: { value?: string }) {
  const label = value || '-';

  return (
    <span
      className={`inline-flex items-center gap-1 rounded border border-solid px-2 py-0.5 text-xs font-semibold ${statusPillClass(
        label
      )}`}
    >
      {statusIcon(label)}
      <span>{label}</span>
    </span>
  );
}

function RuntimeRefSummary({
  runtimeRef,
  ports,
}: {
  runtimeRef?: PlatformRuntimeRef;
  ports?: PlatformPublishedPort[];
}) {
  return (
    <div className="space-y-1 text-xs">
      <div>{runtimeRefLabel(runtimeRef)}</div>
      <div className="text-muted">{formatPublishedPorts(ports)}</div>
    </div>
  );
}

function ReleaseExecutionSummary({ release }: { release: PlatformRelease }) {
  const { t } = useTranslation();
  const recentSteps = release.Steps?.slice(-3) ?? [];
  const health = release.HealthCheckResult;

  return (
    <div className="space-y-2 text-xs">
      {health?.Status && (
        <div className="space-y-1">
          <StatusPill value={health.Status} />
          <div className="text-muted break-all">
            {[
              health.Target,
              health.StatusCode ? String(health.StatusCode) : '',
              health.ErrorMessage,
            ]
              .filter(Boolean)
              .join(' / ')}
          </div>
        </div>
      )}
      {recentSteps.length > 0 && (
        <ol className="space-y-1">
          {recentSteps.map((step) => (
            <li key={`${step.Name}-${step.StartedAt ?? 0}`}>
              <span className="font-medium">{step.Name}</span>{' '}
              <StatusPill value={step.Status} />
              {step.Reason && (
                <span className="text-muted ml-1">{step.Reason}</span>
              )}
            </li>
          ))}
        </ol>
      )}
      {release.ResolutionAction && (
        <div className="text-muted">
          {t('platform.release.resolvedByAction', {
            defaultValue: 'Resolved by {{action}}',
            action: release.ResolutionAction,
          })}
        </div>
      )}
      {!health?.Status && recentSteps.length === 0 && '-'}
    </div>
  );
}

function ReleaseActions({
  release,
  canManage,
  canRollback,
}: {
  release: PlatformRelease;
  canManage: boolean;
  canRollback: boolean;
}) {
  const requiresManualAction =
    release.ManualActionRequired ||
    release.Status === 'interrupted' ||
    release.Status === 'recovery-failed';

  if (
    !requiresManualAction &&
    !(canRollback && release.Status === 'succeeded')
  ) {
    return <span className="text-muted">-</span>;
  }

  return (
    <div className="flex flex-col gap-2">
      {canRollback && release.Status === 'succeeded' && (
        <ReleaseRollbackAction release={release} />
      )}
      <ReleaseManualActions release={release} canManage={canManage} />
    </div>
  );
}

function ReleaseManualActions({
  release,
  canManage,
}: {
  release: PlatformRelease;
  canManage: boolean;
}) {
  const { t } = useTranslation();
  const mutation = useResolvePlatformReleaseMutation();
  const requiresManualAction =
    release.ManualActionRequired ||
    release.Status === 'interrupted' ||
    release.Status === 'recovery-failed';
  const hasCurrentRuntime =
    !!release.RuntimeSnapshot?.CurrentRuntimeRef?.ResourceId;

  if (!requiresManualAction || !canManage) {
    return null;
  }

  function resolve(action: PlatformReleaseResolutionAction) {
    const confirmed = window.confirm(
      t('platform.release.resolveConfirm', {
        defaultValue:
          'This will mark the release as resolved and release the deployment lock. Continue?',
      })
    );
    if (!confirmed) {
      return;
    }

    mutation.mutate({
      releaseId: release.Id,
      action,
      comment: t('platform.release.resolveComment', {
        defaultValue: 'Resolved from platform release page.',
      }),
    });
  }

  return (
    <div className="flex flex-col gap-2">
      {hasCurrentRuntime && (
        <Button
          color="warninglight"
          size="xsmall"
          icon={CheckCircle2}
          disabled={mutation.isLoading}
          onClick={() => resolve('accept-current')}
          data-cy={`platform-release-${release.Id}-accept-current`}
        >
          {t('platform.actions.acceptCurrent', {
            defaultValue: 'Accept current',
          })}
        </Button>
      )}
      <Button
        color="light"
        size="xsmall"
        icon={Wrench}
        disabled={mutation.isLoading}
        onClick={() => resolve('mark-handled')}
        data-cy={`platform-release-${release.Id}-mark-handled`}
      >
        {t('platform.actions.markHandled', {
          defaultValue: 'Mark handled',
        })}
      </Button>
    </div>
  );
}

function ReleaseRollbackAction({ release }: { release: PlatformRelease }) {
  const { t } = useTranslation();
  const diffQuery = usePlatformReleaseRollbackDiff(release.Id);
  const rollbackMutation = useRollbackPlatformReleaseMutation();
  const [diff, setDiff] = useState<PlatformReleaseRollbackDiff>();

  async function previewRollback() {
    const result = await diffQuery.refetch();
    if (result.data) {
      setDiff(result.data);
    }
  }

  function confirmRollback() {
    if (!diff || diff.SourceReleaseId === diff.CurrentReleaseId) {
      return;
    }
    if (
      diff.Production &&
      !window.confirm(
        t('platform.rollback.productionConfirm', {
          defaultValue:
            'This is a production rollback. The current service can be replaced and briefly interrupted. Continue?',
        })
      )
    ) {
      return;
    }

    rollbackMutation.mutate({
      releaseId: release.Id,
      payload: { ConfirmProduction: diff.Production },
      idempotencyKey: createRollbackIdempotencyKey(release.Id),
    });
    setDiff(undefined);
  }

  if (!diff) {
    return (
      <Button
        color="light"
        size="xsmall"
        icon={RotateCcw}
        disabled={diffQuery.isFetching}
        onClick={previewRollback}
        data-cy={`platform-release-${release.Id}-preview-rollback`}
      >
        {t('platform.actions.previewRollback', {
          defaultValue: 'Preview rollback',
        })}
      </Button>
    );
  }

  const changedParts = [
    diff.ImageChanged &&
      t('platform.rollback.changedImage', { defaultValue: 'image' }),
    diff.ConfigChanged &&
      t('platform.rollback.changedConfig', { defaultValue: 'config' }),
    diff.PortsChanged &&
      t('platform.rollback.changedPorts', { defaultValue: 'ports' }),
    diff.EnvironmentChanged &&
      t('platform.rollback.changedEnvironment', {
        defaultValue: 'environment variables',
      }),
  ].filter(Boolean);

  return (
    <div className="min-w-56 rounded border border-solid border-gray-5 bg-gray-1 p-2 text-xs th-dark:bg-gray-9">
      <div className="font-semibold">
        {t('platform.rollback.previewTitle', {
          defaultValue: 'Rollback to release #{{id}}',
          id: diff.SourceReleaseId,
        })}
      </div>
      <div className="text-muted mt-1 break-all">
        {t('platform.rollback.imageSummary', {
          defaultValue: '{{current}} -> {{source}}',
          current: diff.CurrentImage || '-',
          source: diff.SourceImage,
        })}
      </div>
      {changedParts.length > 0 && (
        <div className="text-muted mt-1">
          {t('platform.rollback.changedSummary', {
            defaultValue: 'Changes: {{changes}}',
            changes: changedParts.join(', '),
          })}
        </div>
      )}
      {diff.ChangedConfigKeys?.length ? (
        <div className="text-muted mt-1">
          {t('platform.rollback.configKeys', {
            defaultValue: 'Config keys: {{keys}}',
            keys: diff.ChangedConfigKeys.join(', '),
          })}
        </div>
      ) : null}
      {diff.ChangedEnvironmentNames?.length ? (
        <div className="text-muted mt-1">
          {t('platform.rollback.environmentNames', {
            defaultValue: 'Environment variables: {{names}}',
            names: diff.ChangedEnvironmentNames.join(', '),
          })}
        </div>
      ) : null}
      {diff.SensitiveVariables?.length ? (
        <div className="text-muted mt-1">
          {t('platform.rollback.sensitiveNames', {
            defaultValue: 'Sensitive variable names: {{names}}',
            names: diff.SensitiveVariables.map(
              (variable) => variable.Name
            ).join(', '),
          })}
        </div>
      ) : null}
      {diff.SourceReleaseId === diff.CurrentReleaseId ? (
        <div className="text-muted mt-2">
          {t('platform.rollback.alreadyServing', {
            defaultValue: 'This historical release is already serving.',
          })}
        </div>
      ) : (
        <div className="mt-2 flex gap-2">
          <Button
            color="warninglight"
            size="xsmall"
            icon={RotateCcw}
            disabled={rollbackMutation.isLoading}
            onClick={confirmRollback}
            data-cy={`platform-release-${release.Id}-confirm-rollback`}
          >
            {t('platform.actions.confirmRollback', {
              defaultValue: 'Confirm rollback',
            })}
          </Button>
          <Button
            color="light"
            size="xsmall"
            disabled={rollbackMutation.isLoading}
            onClick={() => setDiff(undefined)}
            data-cy={`platform-release-${release.Id}-cancel-rollback`}
          >
            {t('platform.actions.cancel', { defaultValue: 'Cancel' })}
          </Button>
        </div>
      )}
    </div>
  );
}

function WizardStepContent({
  step,
  projects,
  environments,
  applications,
  services,
	readyArtifacts,
	selectedArtifact,
  currentEnvironment,
  currentDeployment,
  selectedProjectId,
  selectedEnvironmentId,
  selectedApplicationId,
  selectedServiceId,
  onProjectChange,
  onEnvironmentChange,
  onApplicationChange,
  onServiceChange,
	onArtifactSelect,
  artifactName,
  onArtifactNameChange,
  version,
  onVersionChange,
  imageRef,
  onImageRefChange,
  imageDigest,
  onImageDigestChange,
  containerPort,
  onContainerPortChange,
  hostPort,
  onHostPortChange,
  healthType,
  onHealthTypeChange,
  healthPath,
  onHealthPathChange,
  healthPort,
  onHealthPortChange,
  healthVerification,
  onHealthVerificationChange,
  envText,
  onEnvTextChange,
  desiredSpec,
  validationResult,
  releaseResult,
}: {
  step: number;
  projects: PlatformProject[];
  environments: PlatformEnvironment[];
  applications: PlatformApplication[];
  services: PlatformServiceDefinition[];
	readyArtifacts: PlatformArtifact[];
	selectedArtifact?: PlatformArtifact;
  currentProject?: PlatformProject;
  currentEnvironment?: PlatformEnvironment;
  currentApplication?: PlatformApplication;
  currentService?: PlatformServiceDefinition;
  currentDeployment?: PlatformServiceDeployment;
  selectedProjectId?: number;
  selectedEnvironmentId?: number;
  selectedApplicationId?: number;
  selectedServiceId?: number;
  onProjectChange: (projectId: number | undefined) => void;
  onEnvironmentChange: (environmentId: number | undefined) => void;
  onApplicationChange: (applicationId: number | undefined) => void;
  onServiceChange: (serviceId: number | undefined) => void;
	onArtifactSelect: (artifactId: number | undefined) => void;
  artifactName: string;
  onArtifactNameChange: (value: string) => void;
  version: string;
  onVersionChange: (value: string) => void;
  imageRef: string;
  onImageRefChange: (value: string) => void;
  imageDigest: string;
  onImageDigestChange: (value: string) => void;
  containerPort: number;
  onContainerPortChange: (value: number) => void;
  hostPort: number;
  onHostPortChange: (value: number) => void;
  healthType: string;
  onHealthTypeChange: (value: string) => void;
  healthPath: string;
  onHealthPathChange: (value: string) => void;
  healthPort: number;
  onHealthPortChange: (value: number) => void;
  healthVerification: string;
  onHealthVerificationChange: (value: string) => void;
  envText: string;
  onEnvTextChange: (value: string) => void;
  desiredSpec: PlatformDeploymentDesiredSpec;
  validationResult?: PlatformReleaseValidateResponse;
  releaseResult?: PlatformReleaseCreateResponse;
}) {
  const { t } = useTranslation();

  if (step === 0) {
    return (
      <WizardPanel
        title={t('platform.deploy.basic.title', {
          defaultValue: 'Select model scope',
        })}
        description={t('platform.deploy.basic.body', {
          defaultValue:
            'Choose project, environment, application, and service before registering an artifact.',
        })}
      >
        <div className="grid gap-3 md:grid-cols-2">
          <SelectField
            label={t('platform.forms.project', { defaultValue: 'Project' })}
            value={selectedProjectId}
            disabled={projects.length === 0}
            onChange={onProjectChange}
          >
            {projects.map((project) => (
              <option key={project.Id} value={project.Id}>
                {project.Name}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t('platform.forms.environment', {
              defaultValue: 'Environment',
            })}
            value={selectedEnvironmentId}
            disabled={environments.length === 0}
            onChange={onEnvironmentChange}
          >
            {environments.map((environment) => (
              <option key={environment.Id} value={environment.Id}>
                {environment.Name}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t('platform.forms.application', {
              defaultValue: 'Application',
            })}
            value={selectedApplicationId}
            disabled={applications.length === 0}
            onChange={onApplicationChange}
          >
            {applications.map((application) => (
              <option key={application.Id} value={application.Id}>
                {application.Name}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t('platform.forms.service', { defaultValue: 'Service' })}
            value={selectedServiceId}
            disabled={services.length === 0}
            onChange={onServiceChange}
          >
            {services.map((service) => (
              <option key={service.Id} value={service.Id}>
                {service.Name}
              </option>
            ))}
          </SelectField>
        </div>
      </WizardPanel>
    );
  }

  if (step === 1 || step === 2) {
    return (
      <WizardPanel
        title={
          step === 1
            ? t('platform.deploy.image.title', {
                defaultValue: 'Ready artifact or existing image',
              })
            : t('platform.deploy.registry.title', {
                defaultValue: 'Final image and traceability',
              })
        }
        description={
          step === 1
            ? t('platform.deploy.image.body', {
                defaultValue:
                  'Select a ready artifact that matches the service type, or register an existing image reference. Source builds, Git, and embedded registries remain out of scope.',
              })
            : t('platform.deploy.registry.body', {
                defaultValue:
                  'Ready artifacts carry their final registry tag and digest. A manual image reference remains weakly traceable until a digest is provided.',
              })
        }
      >
        <div className="grid gap-3 md:grid-cols-2">
          <label className="form-control-label">
            {t('platform.forms.readyArtifact', {
              defaultValue: 'Ready artifact',
            })}
            <select
              className="form-control mt-1"
              value={selectedArtifact?.Id ?? ''}
              onChange={(event) =>
                onArtifactSelect(
                  event.target.value ? Number(event.target.value) : undefined
                )
              }
            >
              <option value="">
                {t('platform.deploy.manualImageOption', {
                  defaultValue: 'Use a manual existing image reference',
                })}
              </option>
              {readyArtifacts.map((artifact) => (
                <option key={artifact.Id} value={artifact.Id}>
                  {artifact.Name} · {artifact.Version} · {artifact.Type}
                </option>
              ))}
            </select>
          </label>
          {selectedArtifact ? (
            <SummaryList
              rows={[
                [
                  t('platform.columns.image', { defaultValue: 'Image' }),
                  selectedArtifact.ImageTag ?? selectedArtifact.ImageRef ?? '',
                ],
                [
                  t('platform.columns.imageDigest', {
                    defaultValue: 'Image digest',
                  }),
                  selectedArtifact.ImageDigest ?? '',
                ],
                [
                  t('platform.columns.status', { defaultValue: 'Status' }),
                  selectedArtifact.Status ?? '',
                ],
              ]}
            />
          ) : (
            <>
              <TextInputField
                label={t('platform.forms.imageRef', {
                  defaultValue: 'Image reference',
                })}
                value={imageRef}
                required
                onChange={onImageRefChange}
              />
              <TextInputField
                label={t('platform.forms.version', {
                  defaultValue: 'Version',
                })}
                value={version}
                required
                onChange={onVersionChange}
              />
              <TextInputField
                label={t('platform.forms.artifactName', {
                  defaultValue: 'Artifact name',
                })}
                value={artifactName}
                onChange={onArtifactNameChange}
              />
              <TextInputField
                label={t('platform.forms.imageDigest', {
                  defaultValue: 'Image digest',
                })}
                value={imageDigest}
                onChange={onImageDigestChange}
              />
            </>
          )}
          {!selectedArtifact && readyArtifacts.length === 0 && (
            <div className="text-muted text-sm md:col-span-2">
              {t('platform.deploy.noReadyArtifact', {
                defaultValue:
                  'No ready artifact matches this service. Upload, prepare, and push a supported artifact first, or use an existing image reference.',
              })}
            </div>
          )}
        </div>
      </WizardPanel>
    );
  }

  if (step === 3) {
    return (
      <WizardPanel
        title={t('platform.deploy.health.title', {
          defaultValue: 'Ports and health check',
        })}
        description={t('platform.deploy.health.body', {
          defaultValue:
            'Capture published port, container port, health level, check path, retry count, and timeout.',
        })}
      >
        <div className="grid gap-3 md:grid-cols-3">
          <NumberInputField
            label={t('platform.forms.containerPort', {
              defaultValue: 'Container port',
            })}
            value={containerPort}
            onChange={onContainerPortChange}
          />
          <NumberInputField
            label={t('platform.forms.hostPort', {
              defaultValue: 'Host port',
            })}
            value={hostPort}
            onChange={onHostPortChange}
          />
          <NumberInputField
            label={t('platform.forms.healthPort', {
              defaultValue: 'Health port',
            })}
            value={healthPort}
            onChange={onHealthPortChange}
          />
          <StringSelectField
            label={t('platform.forms.healthType', {
              defaultValue: 'Health type',
            })}
            value={healthType}
            onChange={onHealthTypeChange}
            options={[
              ['http', 'HTTP'],
              ['tcp', 'TCP'],
              [
                'none',
                t('platform.healthTypes.none', { defaultValue: 'None' }),
              ],
            ]}
          />
          <StringSelectField
            label={t('platform.forms.healthVerification', {
              defaultValue: 'Health verification',
            })}
            value={healthVerification}
            onChange={onHealthVerificationChange}
            options={[
              [
                'verified',
                t('platform.healthLevels.verified', {
                  defaultValue: 'Verified',
                }),
              ],
              [
                'basic',
                t('platform.healthLevels.basic', { defaultValue: 'Basic' }),
              ],
              [
                'none',
                t('platform.healthLevels.none', { defaultValue: 'None' }),
              ],
            ]}
          />
          <TextInputField
            label={t('platform.forms.healthPath', {
              defaultValue: 'Health path',
            })}
            value={healthPath}
            onChange={onHealthPathChange}
          />
        </div>
      </WizardPanel>
    );
  }

  if (step === 4) {
    return (
      <WizardPanel
        title={t('platform.deploy.env.title', {
          defaultValue: 'Environment variables',
        })}
        description={t('platform.deploy.env.body', {
          defaultValue:
            'Literal variables can be captured now. Enter one KEY=VALUE pair per line.',
        })}
      >
        <TextAreaField
          label={t('platform.forms.envOverrides', {
            defaultValue: 'Environment variables',
          })}
          value={envText}
          onChange={onEnvTextChange}
          placeholder="NODE_ENV=production"
        />
      </WizardPanel>
    );
  }

  if (step === 5) {
    return (
      <WizardPanel
        title={t('platform.deploy.strategy.title', {
          defaultValue: 'Replace strategy',
        })}
        description={t('platform.deploy.strategy.body', {
          defaultValue:
            'V0.1 uses docker-container runtime, one replica, unless-stopped restart policy, and replace strategy.',
        })}
      >
        <SummaryList
          rows={[
            [
              t('platform.deploy.summary.runtimeDriver', {
                defaultValue: 'Runtime driver',
              }),
              desiredSpec.Runtime?.RuntimeDriver ?? '',
            ],
            [
              t('platform.deploy.summary.replicas', {
                defaultValue: 'Replicas',
              }),
              String(desiredSpec.Runtime?.Replicas ?? ''),
            ],
            [
              t('platform.deploy.summary.restartPolicy', {
                defaultValue: 'Restart policy',
              }),
              desiredSpec.Runtime?.RestartPolicy ?? '',
            ],
            [
              t('platform.deploy.summary.strategy', {
                defaultValue: 'Strategy',
              }),
              desiredSpec.Strategy?.Type ?? '',
            ],
            [
              t('platform.deploy.summary.deploymentConfig', {
                defaultValue: 'Deployment config',
              }),
              currentDeployment
                ? `#${currentDeployment.Id}`
                : t('platform.deploy.summary.willCreateDeployment', {
                    defaultValue: 'Will create',
                  }),
            ],
          ]}
        />
      </WizardPanel>
    );
  }

  if (step === 6) {
    return (
      <WizardPanel
        title={t('platform.deploy.validate.title', {
          defaultValue: 'Validation result',
        })}
        description={t('platform.deploy.validate.body', {
          defaultValue:
            'Click Validate release to persist the deployment config, register or reuse the image artifact, and run control-plane validation.',
        })}
      >
        {validationResult ? (
          <SummaryList
            rows={[
              [
                t('platform.columns.status', { defaultValue: 'Status' }),
                validationResult.Status ?? '',
              ],
              [
                t('platform.columns.failureReason', {
                  defaultValue: 'Failure reason',
                }),
                validationResult.Reason ?? '',
              ],
              [
                t('platform.columns.message', { defaultValue: 'Message' }),
                validationResult.Message ?? '',
              ],
            ]}
          />
        ) : (
          <div className="text-muted text-sm">
            {t('platform.deploy.validate.empty', {
              defaultValue: 'No validation has been run yet.',
            })}
          </div>
        )}
      </WizardPanel>
    );
  }

  return (
    <WizardPanel
      title={t('platform.deploy.confirm.title', {
        defaultValue: 'Confirm release',
      })}
      description={t('platform.deploy.confirm.body', {
        defaultValue:
          'Create release will trigger the configured release executor. Production environments show an extra downtime warning before submission.',
      })}
    >
      {currentEnvironment?.IsProduction && (
        <Alert
          color="warn"
          title={t('platform.deploy.productionWarningTitle', {
            defaultValue: 'Production warning',
          })}
        >
          {t('platform.deploy.productionWarningBody', {
            defaultValue:
              'V0.1 replace strategy may briefly stop service. Confirm only when the window is acceptable.',
          })}
        </Alert>
      )}
      {releaseResult?.Release ? (
        <SummaryList
          rows={[
            [
              t('platform.columns.releaseId', { defaultValue: 'Release ID' }),
              String(releaseResult.Release.Id),
            ],
            [
              t('platform.columns.status', { defaultValue: 'Status' }),
              releaseResult.Release.Status,
            ],
            [
              t('platform.columns.image', { defaultValue: 'Image' }),
              releaseResult.Release.Image ?? imageRef,
            ],
          ]}
        />
      ) : (
        <div className="text-muted text-sm">
          {t('platform.deploy.confirm.ready', {
            defaultValue:
              'Review the summary, then create the release when ready.',
          })}
        </div>
      )}
    </WizardPanel>
  );
}

function WizardPanel({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="rounded border border-solid border-gray-5 p-4">
      <div className="mb-2 flex items-center gap-2 text-lg font-semibold">
        <Package className="icon" />
        {title}
      </div>
      <p className="text-muted max-w-3xl text-sm">{description}</p>
      <div className="mt-4">{children}</div>
    </div>
  );
}

function buildDeployDesiredSpec({
  imageRef,
  imageDigest,
  containerPort,
  hostPort,
  healthType,
  healthPath,
  healthPort,
  healthVerification,
  envText,
}: {
  imageRef: string;
  imageDigest: string;
  containerPort: number;
  hostPort: number;
  healthType: string;
  healthPath: string;
  healthPort: number;
  healthVerification: string;
  envText: string;
}): PlatformDeploymentDesiredSpec {
  return {
    Image: {
      Image: imageRef.trim(),
      PullPolicy: 'if-not-present',
      ResolvedDigest: imageDigest.trim() || undefined,
      Traceability: 'weak',
    },
    Ports: [
      {
        Name: 'http',
        ContainerPort: containerPort,
        HostPort: hostPort,
        Protocol: 'tcp',
        ExposeMode: 'published',
      },
    ],
    HealthCheck:
      healthType === 'none'
        ? {
            VerificationLevel: 'none',
            Type: 'none',
          }
        : {
            VerificationLevel: healthVerification,
            Type: healthType,
            Path: healthType === 'http' ? healthPath.trim() || '/' : undefined,
            Port: healthPort || containerPort,
            Retries: 3,
            TimeoutSeconds: 3,
            IntervalSeconds: 10,
            StartPeriodSeconds: 30,
          },
    EnvOverrides: parseEnvOverrides(envText),
    Runtime: {
      RuntimeDriver: 'docker-container',
      Replicas: 1,
      RestartPolicy: 'unless-stopped',
      StopTimeoutSeconds: 10,
      ContainerRetentionCount: 1,
      ContainerRetentionDays: 7,
    },
    Strategy: {
      Type: 'replace',
    },
  };
}

// 制品页已完成的镜像仍需按服务类型筛选，避免前端把 Java/dist 原始输入误导为可投放到不匹配服务的运行时；后端发布校验仍是最终边界。
function artifactMatchesService(
  artifact: PlatformArtifact,
  service?: PlatformServiceDefinition
) {
  if (!service) {
    return false;
  }
  if (artifact.Type === 'java-jar') {
    return service.Type === 'java-service';
  }
  if (artifact.Type === 'frontend-dist') {
    return service.Type === 'frontend' || service.Type === 'static-site';
  }
  return (
    artifact.Type === 'image' ||
    artifact.Type === 'docker-image-tar' ||
    artifact.Type === 'oci-archive'
  );
}

function parseEnvOverrides(envText: string) {
  return envText
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const separatorIndex = line.indexOf('=');
      const key =
        separatorIndex >= 0 ? line.slice(0, separatorIndex).trim() : line;
      const value = separatorIndex >= 0 ? line.slice(separatorIndex + 1) : '';

      return {
        Name: key,
        Value: value,
        Source: 'literal',
        IsSecret: false,
        HasValue: true,
      };
    })
    .filter((item) => item.Name);
}

function parsePlainConfigEntries(entriesText: string): PlatformConfigEntry[] {
  return entriesText
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const separatorIndex = line.indexOf('=');
      const key =
        separatorIndex >= 0 ? line.slice(0, separatorIndex).trim() : line;
      const value = separatorIndex >= 0 ? line.slice(separatorIndex + 1) : '';

      return {
        Key: key,
        ValueType: 'plain' as const,
        Value: value,
        Sensitive: false,
        Required: false,
      };
    })
    .filter((entry) => entry.Key);
}

function formatPlainConfigEntries(entries: PlatformConfigEntry[]) {
  return entries
    .filter((entry) => !entry.Sensitive && entry.ValueType === 'plain')
    .map((entry) => `${entry.Key}=${entry.Value ?? ''}`)
    .join('\n');
}

function buildReleasePayload({
  project,
  environment,
  application,
  service,
  deployment,
  artifact,
  version,
}: {
  project: PlatformProject;
  environment: PlatformEnvironment;
  application: PlatformApplication;
  service: PlatformServiceDefinition;
  deployment: PlatformServiceDeployment;
  artifact: PlatformArtifact;
  version: string;
}): CreatePlatformReleasePayload {
  return {
    ProjectId: project.Id,
    EnvironmentId: environment.Id,
    ApplicationId: application.Id,
    ServiceDefinitionId: service.Id,
    ServiceDeploymentId: deployment.Id,
    ArtifactId: artifact.Id,
    Version: version,
    ExpectedSpecRevision: deployment.SpecRevision,
    Strategy: {
      Type: 'replace',
    },
    TriggerType: 'manual',
  };
}

function createIdempotencyKey(payload: CreatePlatformReleasePayload) {
  const randomId =
    typeof window !== 'undefined' && window.crypto?.randomUUID
      ? window.crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;

  return [
    'platform',
    payload.ServiceDeploymentId,
    payload.ArtifactId,
    payload.Version,
    randomId,
  ].join('-');
}

function createRollbackIdempotencyKey(releaseId: number) {
  const randomId =
    typeof window !== 'undefined' && window.crypto?.randomUUID
      ? window.crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;

  return ['platform', 'rollback', releaseId, randomId].join('-');
}

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/(^-|-$)/g, '');
}

function formatEnvironmentTargets(targets?: PlatformEnvironment['Targets']) {
  if (!targets?.length) {
    return '';
  }

  return targets
    .map((target) =>
      [
        `endpoint ${target.EndpointId}`,
        target.HostAddress,
        target.Role,
        target.Enabled ? 'enabled' : 'disabled',
      ]
        .filter(Boolean)
        .join(' / ')
    )
    .join(', ');
}

function SummaryList({ rows }: { rows: string[][] }) {
  return (
    <dl className="space-y-3 text-sm">
      {rows.map(([label, value]) => (
        <div key={label}>
          <dt className="text-muted">{label}</dt>
          <dd className="break-all font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function statusPillClass(value: string) {
  const normalized = value.toLowerCase();

  if (
    ['succeeded', 'passed', 'running', 'active', 'resolved'].includes(
      normalized
    )
  ) {
    return 'border-green-5 bg-green-2 text-green-9 th-dark:bg-green-10 th-dark:text-white';
  }

  if (
    normalized.includes('failed') ||
    normalized.includes('missing') ||
    normalized === 'interrupted'
  ) {
    return 'border-red-5 bg-red-2 text-red-9 th-dark:bg-red-10 th-dark:text-white';
  }

  if (normalized === 'canceled' || normalized === 'archived') {
    return 'border-gray-5 bg-gray-2 text-gray-8 th-dark:bg-gray-10 th-dark:text-white';
  }

  return 'border-blue-5 bg-blue-2 text-blue-9 th-dark:bg-blue-10 th-dark:text-white';
}

function statusIcon(value: string) {
  const normalized = value.toLowerCase();

  if (['succeeded', 'passed', 'running', 'resolved'].includes(normalized)) {
    return <CheckCircle2 className="h-3 w-3" />;
  }

  if (
    normalized.includes('failed') ||
    normalized.includes('missing') ||
    normalized === 'interrupted'
  ) {
    return <AlertTriangle className="h-3 w-3" />;
  }

  return null;
}

function runtimeRefLabel(runtimeRef?: PlatformRuntimeRef) {
  if (!runtimeRef?.ResourceId) {
    return '';
  }

  return [
    runtimeRef.Name || runtimeRef.ResourceId,
    runtimeRef.DriverId,
    runtimeRef.EndpointId ? `endpoint ${runtimeRef.EndpointId}` : '',
  ]
    .filter(Boolean)
    .join(' / ');
}

function formatPublishedPorts(ports?: PlatformPublishedPort[]) {
  if (!ports?.length) {
    return '';
  }

  return ports
    .map((port) => {
      const host = [port.HostIP, port.HostPort].filter(Boolean).join(':');
      const target = `${port.ContainerPort}/${port.Protocol || 'tcp'}`;
      return host ? `${host}->${target}` : target;
    })
    .join(', ');
}

function formatUnixTime(value?: number) {
  if (!value) {
    return '';
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value * 1000));
}
