import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';

import {
  CreateImageReferenceArtifactPayload,
	UploadPlatformArtifactPayload,
	BuildJavaArtifactPayload,
	BuildStaticArtifactPayload,
	PushPlatformArtifactPayload,
  CreatePlatformApplicationPayload,
  CreatePlatformConfigSetPayload,
  CreatePlatformEnvironmentPayload,
  CreatePlatformProjectPayload,
  CreatePlatformReleasePayload,
  CreatePlatformServiceDefinitionPayload,
  CreatePlatformServiceDeploymentPayload,
  PlatformEnvironment,
  PlatformApplication,
  PlatformAuditLog,
  PlatformArtifact,
  PlatformConfigScopeType,
  PlatformConfigSet,
  PlatformEffectiveConfigResponse,
  PlatformProject,
  PlatformRelease,
  PlatformReleaseCreateResponse,
  PlatformReleaseRollbackDiff,
  PlatformReleaseResolutionAction,
  PlatformReleaseValidateResponse,
  RollbackPlatformReleasePayload,
  PlatformServiceDeployment,
  PlatformServiceDeploymentLogs,
  PlatformServiceDeploymentStatus,
  PlatformServiceDefinition,
  UpdatePlatformServiceDeploymentPayload,
  UpdatePlatformConfigSetPayload,
} from './types';

export const platformQueryKeys = {
  all: ['platform'] as const,
  projects: () => [...platformQueryKeys.all, 'projects'] as const,
  environments: (projectId?: number) =>
    [...platformQueryKeys.all, 'environments', projectId] as const,
  applications: (projectId?: number) =>
    [...platformQueryKeys.all, 'applications', projectId] as const,
  services: (applicationId?: number) =>
    [...platformQueryKeys.all, 'services', applicationId] as const,
  deployments: (serviceDefinitionId?: number) =>
    [...platformQueryKeys.all, 'deployments', serviceDefinitionId] as const,
  deploymentStatus: (deploymentId?: number) =>
    [...platformQueryKeys.all, 'deployment-status', deploymentId] as const,
  deploymentLogs: (deploymentId?: number, tail?: number) =>
    [...platformQueryKeys.all, 'deployment-logs', deploymentId, tail] as const,
  artifacts: () => [...platformQueryKeys.all, 'artifacts'] as const,
  configSets: (
    projectId?: number,
    scopeType?: PlatformConfigScopeType,
    scopeId?: number
  ) =>
    [
      ...platformQueryKeys.all,
      'config-sets',
      projectId,
      scopeType,
      scopeId,
    ] as const,
  effectiveConfig: (deploymentId?: number) =>
    [...platformQueryKeys.all, 'effective-config', deploymentId] as const,
  releases: () => [...platformQueryKeys.all, 'releases'] as const,
  release: (releaseId?: number) =>
    [...platformQueryKeys.all, 'releases', releaseId] as const,
  rollbackDiff: (releaseId?: number) =>
    [...platformQueryKeys.all, 'rollback-diff', releaseId] as const,
  auditLogs: (projectId?: number) =>
    [...platformQueryKeys.all, 'audit-logs', projectId] as const,
};

async function getProjects() {
  const response = await axios.get<PlatformProject[]>('/platform/projects');
  return response.data;
}

async function createProject(payload: CreatePlatformProjectPayload) {
  const response = await axios.post<PlatformProject>(
    '/platform/projects',
    payload
  );
  return response.data;
}

async function getEnvironments(projectId: number) {
  const response = await axios.get<PlatformEnvironment[]>(
    `/platform/projects/${projectId}/environments`
  );
  return response.data;
}

async function createEnvironment({
  projectId,
  payload,
}: {
  projectId: number;
  payload: CreatePlatformEnvironmentPayload;
}) {
  const response = await axios.post<PlatformEnvironment>(
    `/platform/projects/${projectId}/environments`,
    payload
  );
  return response.data;
}

async function getApplications(projectId: number) {
  const response = await axios.get<PlatformApplication[]>(
    `/platform/projects/${projectId}/applications`
  );
  return response.data;
}

async function createApplication({
  projectId,
  payload,
}: {
  projectId: number;
  payload: CreatePlatformApplicationPayload;
}) {
  const response = await axios.post<PlatformApplication>(
    `/platform/projects/${projectId}/applications`,
    payload
  );
  return response.data;
}

async function getServices(applicationId: number) {
  const response = await axios.get<PlatformServiceDefinition[]>(
    `/platform/applications/${applicationId}/services`
  );
  return response.data;
}

async function createServiceDefinition({
  applicationId,
  payload,
}: {
  applicationId: number;
  payload: CreatePlatformServiceDefinitionPayload;
}) {
  const response = await axios.post<PlatformServiceDefinition>(
    `/platform/applications/${applicationId}/services`,
    payload
  );
  return response.data;
}

async function getServiceDeployments(serviceDefinitionId: number) {
  const response = await axios.get<PlatformServiceDeployment[]>(
    `/platform/services/${serviceDefinitionId}/deployments`
  );
  return response.data;
}

async function createServiceDeployment({
  serviceDefinitionId,
  payload,
}: {
  serviceDefinitionId: number;
  payload: CreatePlatformServiceDeploymentPayload;
}) {
  const response = await axios.post<PlatformServiceDeployment>(
    `/platform/services/${serviceDefinitionId}/deployments`,
    payload
  );
  return response.data;
}

async function updateServiceDeployment({
  deploymentId,
  payload,
}: {
  deploymentId: number;
  payload: UpdatePlatformServiceDeploymentPayload;
}) {
  const response = await axios.put<PlatformServiceDeployment>(
    `/platform/service-deployments/${deploymentId}`,
    payload
  );
  return response.data;
}

async function getServiceDeploymentStatus(deploymentId: number) {
  const response = await axios.get<PlatformServiceDeploymentStatus>(
    `/platform/service-deployments/${deploymentId}/status`
  );
  return response.data;
}

async function getServiceDeploymentLogs(deploymentId: number, tail: number) {
  const response = await axios.get<PlatformServiceDeploymentLogs>(
    `/platform/service-deployments/${deploymentId}/logs`,
    { params: { tail } }
  );
  return response.data;
}

async function getArtifacts() {
  const response = await axios.get<PlatformArtifact[]>('/platform/artifacts');
  return response.data;
}

async function getConfigSets({
  projectId,
  scopeType,
  scopeId,
}: {
  projectId: number;
  scopeType?: PlatformConfigScopeType;
  scopeId?: number;
}) {
  const response = await axios.get<PlatformConfigSet[]>(
    '/platform/config-sets',
    {
      params: {
        projectId,
        scopeType,
        scopeId,
      },
    }
  );
  return response.data;
}

async function createConfigSet(payload: CreatePlatformConfigSetPayload) {
  const response = await axios.post<PlatformConfigSet>(
    '/platform/config-sets',
    payload
  );
  return response.data;
}

async function updateConfigSet({
  configSetId,
  payload,
}: {
  configSetId: number;
  payload: UpdatePlatformConfigSetPayload;
}) {
  const response = await axios.put<PlatformConfigSet>(
    `/platform/config-sets/${configSetId}`,
    payload
  );
  return response.data;
}

async function archiveConfigSet(configSetId: number) {
  await axios.delete(`/platform/config-sets/${configSetId}`);
}

async function readConfigSecret({
  configSetId,
  key,
  action,
}: {
  configSetId: number;
  key: string;
  action: 'reveal' | 'copy';
}) {
  const response = await axios.post<{ Value: string }>(
    `/platform/config-sets/${configSetId}/entries/${encodeURIComponent(key)}/${action}`
  );
  return response.data.Value;
}

async function getEffectiveConfig(deploymentId: number) {
  const response = await axios.get<PlatformEffectiveConfigResponse>(
    `/platform/service-deployments/${deploymentId}/effective-config`
  );
  return response.data;
}

async function createImageReferenceArtifact(
  payload: CreateImageReferenceArtifactPayload
) {
  const response = await axios.post<PlatformArtifact>(
    '/platform/artifacts/image-reference',
    payload
  );
  return response.data;
}

async function uploadPlatformArtifact(payload: UploadPlatformArtifactPayload) {
	const formData = new FormData();
	formData.set('ProjectId', String(payload.ProjectId));
	if (payload.ApplicationId) {
		formData.set('ApplicationId', String(payload.ApplicationId));
	}
	formData.set('ServiceDefinitionId', String(payload.ServiceDefinitionId));
	formData.set('Name', payload.Name);
	formData.set('Version', payload.Version);
	formData.set('Type', payload.Type);
	if (payload.ExpectedSHA256) {
		formData.set('ExpectedSHA256', payload.ExpectedSHA256);
	}
	formData.set('file', payload.File);

	const response = await axios.post<PlatformArtifact>(
		'/platform/artifacts/upload',
		formData
	);
	return response.data;
}

async function buildJavaArtifact({ artifactId, payload }: { artifactId: number; payload: BuildJavaArtifactPayload }) {
	const response = await axios.post<PlatformArtifact>(`/platform/artifacts/${artifactId}/build-java`, payload);
	return response.data;
}

async function buildStaticArtifact({ artifactId, payload }: { artifactId: number; payload: BuildStaticArtifactPayload }) {
	const response = await axios.post<PlatformArtifact>(`/platform/artifacts/${artifactId}/build-static`, payload);
	return response.data;
}

async function pushPlatformArtifact({ artifactId, payload }: { artifactId: number; payload: PushPlatformArtifactPayload }) {
	const response = await axios.post<PlatformArtifact>(`/platform/artifacts/${artifactId}/push`, payload);
	return response.data;
}

async function cleanupPlatformArtifactOriginal(artifactId: number) {
	await axios.post(`/platform/artifacts/${artifactId}/cleanup-original`);
}

async function getReleases() {
  const response = await axios.get<PlatformRelease[]>('/platform/releases');
  return response.data;
}

async function getRelease(releaseId: number) {
  const response = await axios.get<PlatformRelease>(
    `/platform/releases/${releaseId}`
  );
  return response.data;
}

async function getAuditLogs(projectId: number) {
  const response = await axios.get<PlatformAuditLog[]>('/platform/audit-logs', {
    params: { projectId },
  });
  return response.data;
}

async function validateRelease(payload: CreatePlatformReleasePayload) {
  const response = await axios.post<PlatformReleaseValidateResponse>(
    '/platform/releases/validate',
    payload
  );
  return response.data;
}

async function createRelease({
  payload,
  idempotencyKey,
}: {
  payload: CreatePlatformReleasePayload;
  idempotencyKey: string;
}) {
  const response = await axios.post<PlatformReleaseCreateResponse>(
    '/platform/releases',
    payload,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  );
  return response.data;
}

async function getReleaseRollbackDiff(releaseId: number) {
  const response = await axios.get<PlatformReleaseRollbackDiff>(
    `/platform/releases/${releaseId}/rollback-diff`
  );
  return response.data;
}

async function rollbackRelease({
  releaseId,
  payload,
  idempotencyKey,
}: {
  releaseId: number;
  payload: RollbackPlatformReleasePayload;
  idempotencyKey: string;
}) {
  const response = await axios.post<PlatformReleaseCreateResponse>(
    `/platform/releases/${releaseId}/rollback`,
    payload,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  );
  return response.data;
}

async function resolveRelease({
  releaseId,
  action,
  comment,
}: {
  releaseId: number;
  action: PlatformReleaseResolutionAction;
  comment?: string;
}) {
  const response = await axios.post<PlatformRelease>(
    `/platform/releases/${releaseId}/resolve`,
    {
      Action: action,
      Comment: comment,
    }
  );
  return response.data;
}

export function usePlatformProjects() {
  return useQuery({
    queryKey: platformQueryKeys.projects(),
    queryFn: getProjects,
    ...withError('Failed loading platform projects'),
  });
}

export function usePlatformEnvironments(projectId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.environments(projectId),
    queryFn: () => getEnvironments(projectId as number),
    enabled: !!projectId,
    ...withError('Failed loading platform environments'),
  });
}

export function usePlatformApplications(projectId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.applications(projectId),
    queryFn: () => getApplications(projectId as number),
    enabled: !!projectId,
    ...withError('Failed loading platform applications'),
  });
}

export function useCreatePlatformProjectMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createProject,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform project'),
  });
}

export function useCreatePlatformEnvironmentMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createEnvironment,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform environment'),
  });
}

export function useCreatePlatformApplicationMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createApplication,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform application'),
  });
}

export function useCreatePlatformServiceDefinitionMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createServiceDefinition,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform service'),
  });
}

export function useCreatePlatformServiceDeploymentMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createServiceDeployment,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform service deployment'),
  });
}

export function useUpdatePlatformServiceDeploymentMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateServiceDeployment,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed updating platform service deployment'),
  });
}

export function useCreateImageReferenceArtifactMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createImageReferenceArtifact,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed registering image artifact'),
  });
}

export function useUploadPlatformArtifactMutation() {
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: uploadPlatformArtifact,
		onSuccess: () =>
			queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
		...withError('Failed uploading platform artifact'),
	});
}

export function useBuildJavaArtifactMutation() {
	const queryClient = useQueryClient();
	return useMutation({ mutationFn: buildJavaArtifact, onSuccess: () => queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }), ...withError('Failed packaging Java artifact') });
}

export function useBuildStaticArtifactMutation() {
	const queryClient = useQueryClient();
	return useMutation({ mutationFn: buildStaticArtifact, onSuccess: () => queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }), ...withError('Failed packaging static artifact') });
}

export function usePushPlatformArtifactMutation() {
	const queryClient = useQueryClient();
	return useMutation({ mutationFn: pushPlatformArtifact, onSuccess: () => queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }), ...withError('Failed pushing platform artifact') });
}

export function useCleanupPlatformArtifactOriginalMutation() {
	const queryClient = useQueryClient();
	return useMutation({ mutationFn: cleanupPlatformArtifactOriginal, onSuccess: () => queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }), ...withError('Failed cleaning platform artifact original') });
}

export function usePlatformConfigSets({
  projectId,
  scopeType,
  scopeId,
}: {
  projectId?: number;
  scopeType?: PlatformConfigScopeType;
  scopeId?: number;
}) {
  return useQuery({
    queryKey: platformQueryKeys.configSets(projectId, scopeType, scopeId),
    queryFn: () =>
      getConfigSets({
        projectId: projectId as number,
        scopeType,
        scopeId,
      }),
    enabled: !!projectId && !!scopeId,
    ...withError('Failed loading platform config sets'),
  });
}

export function useCreatePlatformConfigSetMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createConfigSet,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform config set'),
  });
}

export function useUpdatePlatformConfigSetMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateConfigSet,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed updating platform config set'),
  });
}

export function useArchivePlatformConfigSetMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: archiveConfigSet,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed archiving platform config set'),
  });
}

export function useReadPlatformConfigSecretMutation() {
  return useMutation({
    mutationFn: readConfigSecret,
    ...withError('Failed reading platform sensitive variable'),
  });
}

export function useValidatePlatformReleaseMutation() {
  return useMutation({
    mutationFn: validateRelease,
    ...withError('Failed validating platform release'),
  });
}

export function useCreatePlatformReleaseMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createRelease,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform release'),
  });
}

export function usePlatformServices(applicationId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.services(applicationId),
    queryFn: () => getServices(applicationId as number),
    enabled: !!applicationId,
    ...withError('Failed loading platform services'),
  });
}

export function usePlatformServiceDeployments(serviceDefinitionId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.deployments(serviceDefinitionId),
    queryFn: () => getServiceDeployments(serviceDefinitionId as number),
    enabled: !!serviceDefinitionId,
    ...withError('Failed loading platform service deployments'),
  });
}

export function usePlatformServiceDeploymentStatus(deploymentId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.deploymentStatus(deploymentId),
    queryFn: () => getServiceDeploymentStatus(deploymentId as number),
    enabled: !!deploymentId,
    ...withError('Failed loading platform service status'),
  });
}

export function usePlatformEffectiveConfig(deploymentId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.effectiveConfig(deploymentId),
    queryFn: () => getEffectiveConfig(deploymentId as number),
    enabled: !!deploymentId,
    ...withError('Failed loading platform effective config'),
  });
}

export function usePlatformServiceDeploymentLogs(
  deploymentId?: number,
  tail = 100
) {
  return useQuery({
    queryKey: platformQueryKeys.deploymentLogs(deploymentId, tail),
    queryFn: () => getServiceDeploymentLogs(deploymentId as number, tail),
    enabled: !!deploymentId,
    ...withError('Failed loading platform service logs'),
  });
}

export function usePlatformArtifacts() {
  return useQuery({
    queryKey: platformQueryKeys.artifacts(),
    queryFn: getArtifacts,
    ...withError('Failed loading platform artifacts'),
  });
}

export function usePlatformReleases() {
  return useQuery({
    queryKey: platformQueryKeys.releases(),
    queryFn: getReleases,
    ...withError('Failed loading platform releases'),
  });
}

/**
 * 发布创建接口只负责落库并返回 Release ID；执行过程由控制面异步推进。
 * 在发布尚未结束时轮询详情，页面才能展示真实状态和每个已完成的安全步骤。
 */
export function usePlatformRelease(releaseId?: number, poll = false) {
  return useQuery({
    queryKey: platformQueryKeys.release(releaseId),
    queryFn: () => getRelease(releaseId as number),
    enabled: !!releaseId,
    refetchInterval: poll ? 1500 : false,
    refetchIntervalInBackground: poll,
    ...withError('Failed loading platform release'),
  });
}

export function usePlatformReleaseRollbackDiff(releaseId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.rollbackDiff(releaseId),
    queryFn: () => getReleaseRollbackDiff(releaseId as number),
    enabled: false,
    ...withError('Failed loading platform release rollback diff'),
  });
}

export function usePlatformAuditLogs(projectId?: number, enabled = true) {
  return useQuery({
    queryKey: platformQueryKeys.auditLogs(projectId),
    queryFn: () => getAuditLogs(projectId as number),
    enabled: !!projectId && enabled,
    ...withError('Failed loading platform audit logs'),
  });
}

export function useResolvePlatformReleaseMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: resolveRelease,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed resolving platform release'),
  });
}

export function useRollbackPlatformReleaseMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: rollbackRelease,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed creating platform rollback release'),
  });
}
