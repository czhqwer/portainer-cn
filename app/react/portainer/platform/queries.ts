import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';

import {
  PlatformApplication,
  PlatformArtifact,
  PlatformProject,
  PlatformRelease,
  PlatformReleaseResolutionAction,
  PlatformServiceDeployment,
  PlatformServiceDeploymentLogs,
  PlatformServiceDeploymentStatus,
  PlatformServiceDefinition,
} from './types';

export const platformQueryKeys = {
  all: ['platform'] as const,
  projects: () => [...platformQueryKeys.all, 'projects'] as const,
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
};

async function getProjects() {
  const response = await axios.get<PlatformProject[]>('/platform/projects');
  return response.data;
}

async function getApplications(projectId: number) {
  const response = await axios.get<PlatformApplication[]>(
    `/platform/projects/${projectId}/applications`
  );
  return response.data;
}

async function getServices(applicationId: number) {
  const response = await axios.get<PlatformServiceDefinition[]>(
    `/platform/applications/${applicationId}/services`
  );
  return response.data;
}

async function getServiceDeployments(serviceDefinitionId: number) {
  const response = await axios.get<PlatformServiceDeployment[]>(
    `/platform/services/${serviceDefinitionId}/deployments`
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

async function getReleases() {
  const response = await axios.get<PlatformRelease[]>('/platform/releases');
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

export function usePlatformApplications(projectId?: number) {
  return useQuery({
    queryKey: platformQueryKeys.applications(projectId),
    queryFn: () => getApplications(projectId as number),
    enabled: !!projectId,
    ...withError('Failed loading platform applications'),
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

export function useResolvePlatformReleaseMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: resolveRelease,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: platformQueryKeys.all }),
    ...withError('Failed resolving platform release'),
  });
}
