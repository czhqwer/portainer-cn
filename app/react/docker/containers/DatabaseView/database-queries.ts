import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { parseAxiosError } from '@/portainer/services/axios/axios';
import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';
import { EnvironmentId } from '@/react/portainer/environments/types';

import { buildDockerProxyUrl } from '../../proxy/queries/buildDockerProxyUrl';
import { withAgentTargetHeader } from '../../proxy/queries/utils';
import { queryKeys as containerQueryKeys } from '../queries/query-keys';
import { ContainerId } from '../types';

export type DatabaseConnectionType =
  | 'mysql'
  | 'mariadb'
  | 'postgres'
  | 'redis';

export type DatabaseConnection = {
  Id: number;
  EnvironmentId: number;
  ContainerId: string;
  CreatedByUserId: number;
  Name: string;
  Type: DatabaseConnectionType;
  Host: string;
  Port: number;
  Database: string;
  Username: string;
  QueryTimeout: number;
  HasPassword: boolean;
  CreatedAt: number;
  UpdatedAt: number;
};

export type DatabaseConnectionPayload = {
  Name: string;
  Type: DatabaseConnectionType;
  Host: string;
  Port: number;
  Database?: string;
  Username?: string;
  Password?: string;
  QueryTimeout: number;
};

export type DatabaseQueryResult = {
  Columns: string[];
  Rows: Array<Record<string, string>>;
  Message: string;
  Duration: number;
};

const databaseQueryKeys = {
  list: (environmentId: EnvironmentId, containerId: ContainerId) =>
    [
      ...containerQueryKeys.container(environmentId, containerId),
      'database-connections',
    ] as const,
};

export function useDatabaseConnections(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  nodeName?: string
) {
  return useQuery({
    queryKey: databaseQueryKeys.list(environmentId, containerId),
    queryFn: () => getDatabaseConnections(environmentId, containerId, nodeName),
    ...withError('Unable to retrieve database connections'),
  });
}

export function useCreateDatabaseConnection(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  nodeName?: string
) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (payload: DatabaseConnectionPayload) =>
      createDatabaseConnection(environmentId, containerId, payload, nodeName),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: databaseQueryKeys.list(environmentId, containerId),
      }),
    ...withError('Unable to create database connection'),
  });
}

export function useUpdateDatabaseConnection(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  nodeName?: string
) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: number;
      payload: DatabaseConnectionPayload;
    }) =>
      updateDatabaseConnection(
        environmentId,
        containerId,
        id,
        payload,
        nodeName
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: databaseQueryKeys.list(environmentId, containerId),
      }),
    ...withError('Unable to update database connection'),
  });
}

export function useDeleteDatabaseConnection(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  nodeName?: string
) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) =>
      deleteDatabaseConnection(environmentId, containerId, id, nodeName),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: databaseQueryKeys.list(environmentId, containerId),
      }),
    ...withError('Unable to delete database connection'),
  });
}

export function useRunDatabaseQuery(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  nodeName?: string
) {
  return useMutation({
    mutationFn: ({ id, query }: { id: number; query: string }) =>
      runDatabaseQuery(environmentId, containerId, id, query, nodeName),
    ...withError('Unable to execute database query'),
  });
}

async function getDatabaseConnections(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  nodeName?: string
) {
  try {
    const { data } = await axios.get<DatabaseConnection[]>(
      databaseConnectionsUrl(environmentId, containerId),
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to retrieve database connections');
  }
}

async function createDatabaseConnection(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  payload: DatabaseConnectionPayload,
  nodeName?: string
) {
  try {
    const { data } = await axios.post<DatabaseConnection>(
      databaseConnectionsUrl(environmentId, containerId),
      payload,
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to create database connection');
  }
}

async function updateDatabaseConnection(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  id: number,
  payload: DatabaseConnectionPayload,
  nodeName?: string
) {
  try {
    const { data } = await axios.put<DatabaseConnection>(
      databaseConnectionUrl(environmentId, containerId, id),
      payload,
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to update database connection');
  }
}

async function deleteDatabaseConnection(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  id: number,
  nodeName?: string
) {
  try {
    await axios.delete(databaseConnectionUrl(environmentId, containerId, id), {
      headers: { ...withAgentTargetHeader(nodeName) },
    });
  } catch (e) {
    throw parseAxiosError(e, 'Unable to delete database connection');
  }
}

async function runDatabaseQuery(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  id: number,
  query: string,
  nodeName?: string
) {
  try {
    const { data } = await axios.post<DatabaseQueryResult>(
      `${databaseConnectionUrl(environmentId, containerId, id)}/query`,
      { Query: query },
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to execute database query');
  }
}

function databaseConnectionsUrl(
  environmentId: EnvironmentId,
  containerId: ContainerId
) {
  return buildDockerProxyUrl(
    environmentId,
    'containers',
    containerId,
    'database-connections'
  );
}

function databaseConnectionUrl(
  environmentId: EnvironmentId,
  containerId: ContainerId,
  id: number
) {
  return buildDockerProxyUrl(
    environmentId,
    'containers',
    containerId,
    'database-connections',
    id
  );
}
