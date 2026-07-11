import { useQuery } from '@tanstack/react-query';

import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';

import {
  PlatformApplication,
  PlatformArtifact,
  PlatformProject,
  PlatformRelease,
  PlatformServiceDefinition,
} from './types';

export const platformQueryKeys = {
  all: ['platform'] as const,
  projects: () => [...platformQueryKeys.all, 'projects'] as const,
  applications: (projectId?: number) =>
    [...platformQueryKeys.all, 'applications', projectId] as const,
  services: (applicationId?: number) =>
    [...platformQueryKeys.all, 'services', applicationId] as const,
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

async function getArtifacts() {
  const response = await axios.get<PlatformArtifact[]>('/platform/artifacts');
  return response.data;
}

async function getReleases() {
  const response = await axios.get<PlatformRelease[]>('/platform/releases');
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
