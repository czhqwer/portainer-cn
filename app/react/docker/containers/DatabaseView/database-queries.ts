import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { parseAxiosError } from '@/portainer/services/axios/axios';
import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';
import { EnvironmentId } from '@/react/portainer/environments/types';

import { withAgentTargetHeader } from '../../proxy/queries/utils';
import { buildDockerUrl } from '../../queries/utils/buildDockerUrl';

export type DatabaseConnectionType = 'mysql' | 'mariadb' | 'postgres' | 'redis';

export type DatabaseConnection = {
  Id: number;
  EnvironmentId: number;
  ContainerId?: string;
  Scope: 'environment' | 'container';
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
  ContainerId?: string;
};

export type DatabaseQueryResult = {
  Columns: string[];
  Rows: Array<Record<string, string>>;
  Message: string;
  Duration: number;
  RowsAffected?: number;
  StatementType?: string;
  Preview?: boolean;
};

export type DatabaseSchema = {
  Databases: Array<{
    Name: string;
    Tables: Array<{
      Name: string;
      Type?: string;
    }>;
  }>;
  Message?: string;
};

export type DatabaseConnectionTestResult = {
  Message: string;
};

const databaseQueryKeys = {
  list: (environmentId: EnvironmentId) =>
    ['environments', environmentId, 'database-connections'] as const,
  schema: (environmentId: EnvironmentId, connectionId?: number) =>
    [
      'environments',
      environmentId,
      'database-connections',
      connectionId,
      'schema',
    ] as const,
};

export function useDatabaseConnections(environmentId: EnvironmentId) {
  return useQuery({
    queryKey: databaseQueryKeys.list(environmentId),
    queryFn: () => getDatabaseConnections(environmentId),
    ...withError('Unable to retrieve database connections'),
  });
}

export function useCreateDatabaseConnection(environmentId: EnvironmentId) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (payload: DatabaseConnectionPayload) =>
      createDatabaseConnection(environmentId, payload),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: databaseQueryKeys.list(environmentId),
      }),
    ...withError('Unable to create database connection'),
  });
}

export function useUpdateDatabaseConnection(environmentId: EnvironmentId) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: number;
      payload: DatabaseConnectionPayload;
    }) => updateDatabaseConnection(environmentId, id, payload),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: databaseQueryKeys.list(environmentId),
      }),
    ...withError('Unable to update database connection'),
  });
}

export function useDeleteDatabaseConnection(environmentId: EnvironmentId) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: number) => deleteDatabaseConnection(environmentId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: databaseQueryKeys.list(environmentId),
      }),
    ...withError('Unable to delete database connection'),
  });
}

export function useRunDatabaseQuery(environmentId: EnvironmentId) {
  return useMutation({
    mutationFn: ({
      connection,
      query,
      database,
      preview,
      nodeName,
    }: {
      connection: DatabaseConnection;
      query: string;
      database?: string;
      preview?: boolean;
      nodeName?: string;
    }) =>
      runDatabaseQuery(
        environmentId,
        connection,
        query,
        database,
        preview,
        nodeName
      ),
    ...withError('Unable to execute database query'),
  });
}

export function useDatabaseSchema(
  environmentId: EnvironmentId,
  connection?: DatabaseConnection,
  nodeName?: string
) {
  return useQuery({
    queryKey: databaseQueryKeys.schema(environmentId, connection?.Id),
    queryFn: () => getDatabaseSchema(environmentId, connection!, nodeName),
    enabled: !!connection,
    ...withError('Unable to retrieve database schema'),
  });
}

export function useTestDatabaseConnection(environmentId: EnvironmentId) {
  return useMutation({
    mutationFn: ({
      payload,
      nodeName,
    }: {
      payload: DatabaseConnectionPayload;
      nodeName?: string;
    }) => testDatabaseConnection(environmentId, payload, nodeName),
    ...withError('Unable to test database connection'),
  });
}

async function getDatabaseConnections(environmentId: EnvironmentId) {
  try {
    const { data } = await axios.get<DatabaseConnection[]>(
      databaseConnectionsUrl(environmentId)
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to retrieve database connections');
  }
}

async function createDatabaseConnection(
  environmentId: EnvironmentId,
  payload: DatabaseConnectionPayload
) {
  try {
    const { data } = await axios.post<DatabaseConnection>(
      databaseConnectionsUrl(environmentId),
      payload
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to create database connection');
  }
}

async function updateDatabaseConnection(
  environmentId: EnvironmentId,
  id: number,
  payload: DatabaseConnectionPayload
) {
  try {
    const { data } = await axios.put<DatabaseConnection>(
      databaseConnectionUrl(environmentId, id),
      payload
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to update database connection');
  }
}

async function deleteDatabaseConnection(
  environmentId: EnvironmentId,
  id: number
) {
  try {
    await axios.delete(databaseConnectionUrl(environmentId, id));
  } catch (e) {
    throw parseAxiosError(e, 'Unable to delete database connection');
  }
}

async function runDatabaseQuery(
  environmentId: EnvironmentId,
  connection: DatabaseConnection,
  query: string,
  database?: string,
  preview = false,
  nodeName?: string
) {
  try {
    const { data } = await axios.post<DatabaseQueryResult>(
      `${databaseConnectionUrl(environmentId, connection)}/query`,
      { Query: query, Database: database, Preview: preview },
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to execute database query');
  }
}

async function getDatabaseSchema(
  environmentId: EnvironmentId,
  connection: DatabaseConnection,
  nodeName?: string
) {
  try {
    const { data } = await axios.get<DatabaseSchema>(
      `${databaseConnectionUrl(environmentId, connection)}/schema`,
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to retrieve database schema');
  }
}

async function testDatabaseConnection(
  environmentId: EnvironmentId,
  payload: DatabaseConnectionPayload,
  nodeName?: string
) {
  try {
    const { data } = await axios.post<DatabaseConnectionTestResult>(
      databaseConnectionTestUrl(environmentId, payload),
      payload,
      { headers: { ...withAgentTargetHeader(nodeName) } }
    );

    return data;
  } catch (e) {
    throw parseAxiosError(e, 'Unable to test database connection');
  }
}

function databaseConnectionsUrl(environmentId: EnvironmentId) {
  return `/endpoints/${environmentId}/database-connections`;
}

function databaseConnectionTestUrl(
  environmentId: EnvironmentId,
  payload: DatabaseConnectionPayload
) {
  if (payload.ContainerId) {
    return buildDockerUrl(
      environmentId,
      'containers',
      payload.ContainerId,
      'database-connections',
      'test'
    );
  }

  return `${databaseConnectionsUrl(environmentId)}/test`;
}

function databaseConnectionUrl(
  environmentId: EnvironmentId,
  connection: number | DatabaseConnection
) {
  if (typeof connection !== 'number' && connection.ContainerId) {
    return buildDockerUrl(
      environmentId,
      'containers',
      connection.ContainerId,
      'database-connections',
      connection.Id
    );
  }

  const id = typeof connection === 'number' ? connection : connection.Id;
  return `${databaseConnectionsUrl(environmentId)}/${id}`;
}
