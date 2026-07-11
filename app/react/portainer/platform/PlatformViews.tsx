import { type ReactNode, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Package, Rocket } from 'lucide-react';

import { Alert } from '@@/Alert';
import { Button } from '@@/buttons';
import { PageHeader } from '@@/PageHeader';

import {
  usePlatformApplications,
  usePlatformArtifacts,
  usePlatformProjects,
  usePlatformReleases,
  usePlatformServices,
} from './queries';
import {
  PlatformApplication,
  PlatformArtifact,
  PlatformRelease,
  PlatformServiceDefinition,
} from './types';

export function PlatformProjectsView() {
  const { t } = useTranslation();
  const projectsQuery = usePlatformProjects();
  const projects = projectsQuery.data ?? [];

  return (
    <PlatformPage titleKey="platform.pages.projects.title">
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
          rows={projects.map((project) => [
            project.Name,
            project.Slug,
            project.LifecycleStatus,
            String(project.ResourceVersion),
          ])}
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
  const currentProject = selectedProjectId
    ? projects.find((project) => project.Id === selectedProjectId)
    : projects[0];
  const applicationsQuery = usePlatformApplications(currentProject?.Id);
  const applications = applicationsQuery.data ?? [];
  const firstApplication = applications[0];
  const servicesQuery = usePlatformServices(firstApplication?.Id);
  const services = servicesQuery.data ?? [];

  return (
    <PlatformPage titleKey="platform.pages.applications.title">
      <PlatformNoticeStack />
      <div className="mx-4 mb-4 max-w-sm">
        <label className="text-muted mb-1 block text-sm">
          {t('platform.filters.project', { defaultValue: 'Project' })}
        </label>
        <select
          className="form-control"
          value={currentProject?.Id ?? ''}
          onChange={(event) => setSelectedProjectId(Number(event.target.value))}
          disabled={projects.length === 0}
        >
          {projects.map((project) => (
            <option key={project.Id} value={project.Id}>
              {project.Name}
            </option>
          ))}
        </select>
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
    </PlatformPage>
  );
}

export function PlatformArtifactsView() {
  const { t } = useTranslation();
  const artifactsQuery = usePlatformArtifacts();
  const artifacts = artifactsQuery.data ?? [];

  return (
    <PlatformPage titleKey="platform.pages.artifacts.title">
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
    <PlatformPage titleKey="platform.pages.releases.title">
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
    <PlatformPage titleKey="platform.pages.deploy.title">
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
  children,
}: {
  titleKey: string;
  children: ReactNode;
}) {
  return (
    <>
      <PageHeader
        title={`t('${titleKey}')`}
        breadcrumbs="t('platform.navigation.section')"
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
  rows: string[][];
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
            <tr key={row.join('-')}>
              {row.map((cell, index) => (
                <td
                  key={`${row[0]}-${index}`}
                  className="max-w-[360px] break-all"
                >
                  {cell || '-'}
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
      rows={applications.map((application) => [
        application.Name,
        application.Slug,
        application.LifecycleStatus,
      ])}
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
      rows={services.map((service) => [
        service.Name,
        service.Type,
        service.Slug,
        service.LifecycleStatus,
      ])}
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
      rows={artifacts.map((artifact) => [
        artifact.Name,
        artifact.Version,
        artifact.SourceType,
        artifact.ImageRef ?? '',
        artifact.SHA256 ?? artifact.ImageDigest ?? '',
      ])}
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
      ]}
      rows={releases.map((release) => [
        String(release.Id),
        release.Version,
        release.Status,
        release.Image ?? '',
        release.FailureReason ?? '',
      ])}
    />
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
