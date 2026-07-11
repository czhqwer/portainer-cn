import { type ReactNode, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  FileText,
  Package,
  RefreshCw,
  Rocket,
  Wrench,
} from 'lucide-react';

import { Alert } from '@@/Alert';
import { Button } from '@@/buttons';
import { PageHeader } from '@@/PageHeader';

import {
  usePlatformApplications,
  usePlatformArtifacts,
  usePlatformProjects,
  usePlatformReleases,
  usePlatformServiceDeploymentLogs,
  usePlatformServiceDeployments,
  usePlatformServiceDeploymentStatus,
  usePlatformServices,
  useResolvePlatformReleaseMutation,
} from './queries';
import {
  PlatformApplication,
  PlatformArtifact,
  PlatformPublishedPort,
  PlatformRelease,
  PlatformReleaseResolutionAction,
  PlatformRuntimeRef,
  PlatformServiceDeployment,
  PlatformServiceDefinition,
} from './types';

export function PlatformProjectsView() {
  const { t } = useTranslation();
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];

  return (
    <PlatformPage
      titleKey="platform.pages.projects.title"
      titleDefault="Projects"
    >
      <PlatformNoticeStack />
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
      <DataSection
        title={t('platform.projects.tableTitle', {
          defaultValue: 'Project list',
        })}
        isLoading={projectsQuery.isLoading}
        empty={projects.length === 0}
        emptyMessage={t('platform.empty.projects', {
          defaultValue:
            'No projects yet. Create projects through the API or later forms, then return here to continue the deployment flow.',
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
  const currentProject = selectedProjectId
    ? projects.find((project) => project.Id === selectedProjectId)
    : projects[0];
  const applicationsQuery = usePlatformApplications(currentProject?.Id);
  const applications = applicationsQuery.data ?? [];
  const currentApplication = selectedApplicationId
    ? applications.find(
        (application) => application.Id === selectedApplicationId
      )
    : applications[0];
  const servicesQuery = usePlatformServices(currentApplication?.Id);
  const services = servicesQuery.data ?? [];
  const currentService = selectedServiceId
    ? services.find((service) => service.Id === selectedServiceId)
    : services[0];
  const deploymentsQuery = usePlatformServiceDeployments(currentService?.Id);
  const deployments = deploymentsQuery.data ?? [];
  const currentDeployment = selectedDeploymentId
    ? deployments.find((deployment) => deployment.Id === selectedDeploymentId)
    : deployments[0];

  return (
    <PlatformPage
      titleKey="platform.pages.applications.title"
      titleDefault="Applications"
    >
      <PlatformNoticeStack />
      <div className="mx-4 mb-4 grid gap-3 lg:grid-cols-4">
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
        <SelectField
          label={t('platform.filters.application', {
            defaultValue: 'Application',
          })}
          value={currentApplication?.Id}
          disabled={applications.length === 0}
          onChange={setSelectedApplicationId}
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
          onChange={setSelectedServiceId}
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

  return (
    <PlatformPage
      titleKey="platform.pages.artifacts.title"
      titleDefault="Artifacts"
    >
      <PlatformNoticeStack />
      <DataSection
        title={t('platform.artifacts.tableTitle', {
          defaultValue: 'Artifacts',
        })}
        isLoading={artifactsQuery.isLoading}
        empty={artifacts.length === 0}
        emptyMessage={t('platform.empty.artifacts', {
          defaultValue:
            'No image-reference artifacts yet. Use the deployment wizard or API to register an existing image.',
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
            label: t('platform.summary.blocked', { defaultValue: 'Blocked' }),
            value: releases.filter(
              (release) => release.FailureReason === 'GATE_0B_REQUIRED'
            ).length,
          },
        ]}
      />
      <DataSection
        title={t('platform.releases.tableTitle', {
          defaultValue: 'Release records',
        })}
        isLoading={releasesQuery.isLoading}
        empty={releases.length === 0}
        emptyMessage={t('platform.empty.releases', {
          defaultValue:
            'No release records yet. Gate 0B is still required before real Docker release execution.',
        })}
      >
        <ReleasesTable releases={releases} />
      </DataSection>
    </PlatformPage>
  );
}

export function PlatformDeployView() {
  const { t } = useTranslation();
  const [step, setStep] = useState(0);
  const steps = useMemo(
    () => [
      t('platform.deploy.steps.basic', { defaultValue: 'Basic information' }),
      t('platform.deploy.steps.image', { defaultValue: 'Existing image' }),
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

  return (
    <PlatformPage titleKey="platform.pages.deploy.title" titleDefault="Deploy">
      <PlatformNoticeStack />
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
          <WizardStepContent step={step} />
          <div className="mt-5 flex items-center justify-between gap-3">
            <Button
              color="light"
              disabled={step === 0}
              onClick={() => setStep((value) => Math.max(0, value - 1))}
              data-cy="platform-deploy-prev"
            >
              {t('platform.actions.previous', { defaultValue: 'Previous' })}
            </Button>
            <Button
              color="primary"
              onClick={() =>
                setStep((value) => Math.min(steps.length - 1, value + 1))
              }
              disabled={step === steps.length - 1}
              data-cy="platform-deploy-next"
            >
              {t('platform.actions.next', { defaultValue: 'Next' })}
            </Button>
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
                'customer-a',
              ],
              [
                t('platform.deploy.summary.environment', {
                  defaultValue: 'Environment',
                }),
                'dev',
              ],
              [
                t('platform.deploy.summary.service', {
                  defaultValue: 'Service',
                }),
                'orders-api',
              ],
              [
                t('platform.deploy.summary.image', { defaultValue: 'Image' }),
                'registry.example.com/customer-a/orders-api:2026.07.11-build-001',
              ],
              [
                t('platform.deploy.summary.validation', {
                  defaultValue: 'Validation',
                }),
                'GATE_0B_REQUIRED',
              ],
            ]}
          />
          <Button
            color="primary"
            className="mt-4 w-full justify-center"
            icon={Rocket}
            disabled
            data-cy="platform-real-release-disabled"
          >
            {t('platform.actions.realReleaseDisabled', {
              defaultValue: 'Real release disabled before Gate 0B',
            })}
          </Button>
        </aside>
      </div>
    </PlatformPage>
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
      <Alert color="default" title={t('platform.alerts.adminOnly.title')}>
        {t('platform.alerts.adminOnly.body', {
          defaultValue:
            'All /api/platform business APIs, including read requests, are limited to administrators in V0.1.',
        })}
      </Alert>
      <Alert color="info" title={t('platform.alerts.gate0b.title')}>
        {t('platform.alerts.gate0b.body', {
          defaultValue:
            'Gate 0B has not passed. Docker RuntimeDriver, DeploymentExecutor, ContainerAdapter, candidate validation, switching, and recovery remain disabled.',
        })}
      </Alert>
    </div>
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

function ArtifactsTable({ artifacts }: { artifacts: PlatformArtifact[] }) {
  const { t } = useTranslation();
  return (
    <PlatformTable
      columns={[
        t('platform.columns.name', { defaultValue: 'Name' }),
        t('platform.columns.version', { defaultValue: 'Version' }),
        t('platform.columns.source', { defaultValue: 'Source' }),
        t('platform.columns.image', { defaultValue: 'Image' }),
        t('platform.columns.sha256', { defaultValue: 'SHA256' }),
      ]}
      rows={artifacts.map((artifact) => ({
        key: String(artifact.Id),
        cells: [
          artifact.Name,
          artifact.Version,
          artifact.SourceType,
          artifact.ImageRef ?? '',
          artifact.SHA256 ?? artifact.ImageDigest ?? '',
        ],
      }))}
    />
  );
}

function ReleasesTable({ releases }: { releases: PlatformRelease[] }) {
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
          <ReleaseManualActions key="actions" release={release} />,
        ],
      }))}
    />
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

function ReleaseManualActions({ release }: { release: PlatformRelease }) {
  const { t } = useTranslation();
  const mutation = useResolvePlatformReleaseMutation();
  const requiresManualAction =
    release.ManualActionRequired ||
    release.Status === 'interrupted' ||
    release.Status === 'recovery-failed';
  const hasCurrentRuntime =
    !!release.RuntimeSnapshot?.CurrentRuntimeRef?.ResourceId;

  if (!requiresManualAction) {
    return <span className="text-muted">-</span>;
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

function WizardStepContent({ step }: { step: number }) {
  const { t } = useTranslation();
  const content = [
    [
      t('platform.deploy.basic.title', { defaultValue: 'Select model scope' }),
      t('platform.deploy.basic.body', {
        defaultValue:
          'Choose project, environment, application, and service before registering an artifact.',
      }),
    ],
    [
      t('platform.deploy.image.title', { defaultValue: 'Existing image only' }),
      t('platform.deploy.image.body', {
        defaultValue:
          'Stage 1 supports image-reference input. Git repositories, source builds, and embedded registries are intentionally out of scope.',
      }),
    ],
    [
      t('platform.deploy.registry.title', {
        defaultValue: 'Registry and traceability',
      }),
      t('platform.deploy.registry.body', {
        defaultValue:
          'Select an existing Portainer registry when needed. Digest resolution waits for the Docker executor batch.',
      }),
    ],
    [
      t('platform.deploy.health.title', {
        defaultValue: 'Ports and health check',
      }),
      t('platform.deploy.health.body', {
        defaultValue:
          'Capture host port, container port, health level, check path, retry count, and timeout.',
      }),
    ],
    [
      t('platform.deploy.env.title', {
        defaultValue: 'Environment variables',
      }),
      t('platform.deploy.env.body', {
        defaultValue:
          'Literal variables can be captured now. Sensitive config inheritance and masking will expand in later batches.',
      }),
    ],
    [
      t('platform.deploy.strategy.title', {
        defaultValue: 'Replace strategy',
      }),
      t('platform.deploy.strategy.body', {
        defaultValue:
          'V0.1 only supports replace strategy and may briefly stop service. Production requires explicit warning.',
      }),
    ],
    [
      t('platform.deploy.validate.title', {
        defaultValue: 'Validation result',
      }),
      t('platform.deploy.validate.body', {
        defaultValue:
          'Control-plane validation can run, but real release execution returns GATE_0B_REQUIRED until the Docker/Agent Spike passes.',
      }),
    ],
    [
      t('platform.deploy.confirm.title', {
        defaultValue: 'Confirm blocked execution',
      }),
      t('platform.deploy.confirm.body', {
        defaultValue:
          'The final release button is disabled in this batch. This page is ready for data entry and later executor wiring.',
      }),
    ],
  ][step];

  return (
    <div className="rounded border border-solid border-gray-5 p-4">
      <div className="mb-2 flex items-center gap-2 text-lg font-semibold">
        <Package className="icon" />
        {content[0]}
      </div>
      <p className="text-muted max-w-3xl text-sm">{content[1]}</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <input
          className="form-control"
          value="registry.example.com/customer-a/orders-api:2026.07.11-build-001"
          readOnly
          aria-label={t('platform.deploy.sample.image', {
            defaultValue: 'Sample image reference',
          })}
        />
        <input
          className="form-control"
          value="sha256:6bb4f7f9a46d7b4b43d6602d6b1a0a7e6fdc9d72f9fdbf1e5d6c2f6d8d9b2a01"
          readOnly
          aria-label={t('platform.deploy.sample.digest', {
            defaultValue: 'Sample digest',
          })}
        />
      </div>
    </div>
  );
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
