import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';

import {
  CreateImageReferenceArtifactPayload,
  CreatePlatformApplicationPayload,
  CreatePlatformEnvironmentPayload,
  CreatePlatformProjectPayload,
  CreatePlatformReleasePayload,
  CreatePlatformServiceDefinitionPayload,
  CreatePlatformServiceDeploymentPayload,
  PlatformEnvironment,
  PlatformApplication,
  PlatformAuditLog,
  PlatformArtifact,
  PlatformProject,
  PlatformRelease,
  PlatformReleaseCreateResponse,
  PlatformReleaseResolutionAction,
  PlatformReleaseValidateResponse,
  PlatformServiceDeployment,
  PlatformServiceDeploymentLogs,
  PlatformServiceDeploymentStatus,
  PlatformServiceDefinition,
  UpdatePlatformServiceDeploymentPayload,
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
  releases: () => [...platformQueryKeys.all, 'releases'] as const,
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

async function createImageReferenceArtifact(
  payload: CreateImageReferenceArtifactPayload
) {
  const response = await axios.post<PlatformArtifact>(
    '/platform/artifacts/image-reference',
    payload
  );
  return response.data;
}

async function getReleases() {
  const response = await axios.get<PlatformRelease[]>('/platform/releases');
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
