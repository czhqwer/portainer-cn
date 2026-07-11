import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { parseAxiosError } from '@/portainer/services/axios/axios';
import axios from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';
import { EnvironmentId } from '@/react/portainer/environments/types';

import { withAgentTargetHeader } from '../../proxy/queries/utils';

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
  Id?: number;
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
  RequiresConfirmation?: boolean;
  UnsafeWrite?: boolean;
  ErrorCode?: string;
  RedisKey?: string;
  RedisType?: string;
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

export type DatabaseTableDetails = {
  Database: string;
  Table: string;
  Comment?: string;
  Columns: Array<{
    Name: string;
    Type: string;
    Nullable: boolean;
    PrimaryKey: boolean;
    Default?: string;
    Extra?: string;
    Comment?: string;
  }>;
  Indexes: Array<{
    Name: string;
    Columns: string[];
    Unique: boolean;
    Primary: boolean;
  }>;
};

export type RedisKeySummary = {
  Name: string;
  Type: string;
  TTL: string;
};

export type RedisKeyScanResponse = {
  Cursor: string;
  Keys: RedisKeySummary[];
};

export type RedisKeyDetails = {
  Name: string;
  Type: string;
  TTL: string;
  Value?: string;
  Rows: Array<Record<string, string>>;
};

export type DatabaseRequestError = Error & {
  ErrorCode?: string;
  Details?: string;
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
  tableDetails: (
    environmentId: EnvironmentId,
    connectionId?: number,
    database?: string,
    table?: string
  ) =>
    [
      'environments',
      environmentId,
      'database-connections',
      connectionId,
      'table-details',
      database,
      table,
    ] as const,
  redisKeys: (
    environmentId: EnvironmentId,
    connectionId?: number,
    database?: string,
    pattern?: string,
    cursor?: string
  ) =>
    [
      'environments',
      environmentId,
      'database-connections',
      connectionId,
      'redis-keys',
      database,
      pattern,
      cursor,
    ] as const,
  redisKeyDetails: (
    environmentId: EnvironmentId,
    connectionId?: number,
    database?: string,
    key?: string
  ) =>
    [
      'environments',
      environmentId,
      'database-connections',
      connectionId,
      'redis-key-details',
      database,
      key,
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
      confirmUnsafeWrite,
      nodeName,
      signal,
    }: {
      connection: DatabaseConnection;
      query: string;
      database?: string;
      preview?: boolean;
      confirmUnsafeWrite?: boolean;
      nodeName?: string;
      signal?: AbortSignal;
    }) =>
      runDatabaseQuery(
        environmentId,
        connection,
        query,
        database,
        preview,
        confirmUnsafeWrite,
        nodeName,
        signal
      ),
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

export function useDatabaseTableDetails(
  environmentId: EnvironmentId,
  connection?: DatabaseConnection,
  database?: string,
  table?: string,
  nodeName?: string
) {
  return useQuery({
    queryKey: databaseQueryKeys.tableDetails(
      environmentId,
      connection?.Id,
      database,
      table
    ),
    queryFn: () =>
      getDatabaseTableDetails(
        environmentId,
        connection!,
        database!,
        table!,
        nodeName
      ),
    enabled:
      !!connection && !!database && !!table && connection.Type !== 'redis',
    ...withError('Unable to retrieve table details'),
  });
}

export function useRedisKeys(
  environmentId: EnvironmentId,
  connection?: DatabaseConnection,
  database?: string,
  pattern = '*',
  cursor = '0',
  nodeName?: string
) {
  return useQuery({
    queryKey: databaseQueryKeys.redisKeys(
      environmentId,
      connection?.Id,
      database,
      pattern,
      cursor
    ),
    queryFn: () =>
      getRedisKeys(
        environmentId,
        connection!,
        database,
        pattern,
        cursor,
        nodeName
      ),
    enabled:
      !!connection &&
      connection.Type === 'redis' &&
      /^\d+$/.test((database || '0').trim()),
    ...withError('Unable to scan Redis keys'),
  });
}

export function useRedisKeyDetails(
  environmentId: EnvironmentId,
  connection?: DatabaseConnection,
  database?: string,
  key?: string,
  nodeName?: string
) {
  return useQuery({
    queryKey: databaseQueryKeys.redisKeyDetails(
      environmentId,
      connection?.Id,
      database,
      key
    ),
    queryFn: () =>
      getRedisKeyDetails(environmentId, connection!, database, key!, nodeName),
    enabled:
      !!connection &&
      connection.Type === 'redis' &&
      !!key &&
      /^\d+$/.test((database || '0').trim()),
    ...withError('Unable to retrieve Redis key details'),
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
    throw parseDatabaseError(e, 'Unable to retrieve database connections');
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
    throw parseDatabaseError(e, 'Unable to create database connection');
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
    throw parseDatabaseError(e, 'Unable to update database connection');
  }
}

async function deleteDatabaseConnection(
  environmentId: EnvironmentId,
  id: number
) {
  try {
    await axios.delete(databaseConnectionUrl(environmentId, id));
  } catch (e) {
    throw parseDatabaseError(e, 'Unable to delete database connection');
  }
}

async function runDatabaseQuery(
  environmentId: EnvironmentId,
  connection: DatabaseConnection,
  query: string,
  database?: string,
  preview = false,
  confirmUnsafeWrite = false,
  nodeName?: string,
  signal?: AbortSignal
) {
  try {
    const { data } = await axios.post<DatabaseQueryResult>(
      `${databaseConnectionUrl(environmentId, connection)}/query`,
      {
        Query: query,
        Database: database,
        Preview: preview,
        ConfirmUnsafeWrite: confirmUnsafeWrite,
      },
      { headers: { ...withAgentTargetHeader(nodeName) }, signal }
    );

    return data;
  } catch (e) {
    throw parseDatabaseError(e, 'Unable to execute database query');
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
    throw parseDatabaseError(e, 'Unable to retrieve database schema');
  }
}

async function getDatabaseTableDetails(
  environmentId: EnvironmentId,
  connection: DatabaseConnection,
  database: string,
  table: string,
  nodeName?: string
) {
  try {
    const { data } = await axios.get<DatabaseTableDetails>(
      `${databaseConnectionUrl(environmentId, connection)}/table-details`,
      {
        params: { database, table },
        headers: { ...withAgentTargetHeader(nodeName) },
      }
    );

    return data;
  } catch (e) {
    throw parseDatabaseError(e, 'Unable to retrieve table details');
  }
}

async function getRedisKeys(
  environmentId: EnvironmentId,
  connection: DatabaseConnection,
  database?: string,
  pattern = '*',
  cursor = '0',
  nodeName?: string
) {
  try {
    const { data } = await axios.get<RedisKeyScanResponse>(
      `${databaseConnectionUrl(environmentId, connection)}/redis-keys`,
      {
        params: { database, pattern, cursor, count: 100 },
        headers: { ...withAgentTargetHeader(nodeName) },
      }
    );

    return data;
  } catch (e) {
    throw parseDatabaseError(e, 'Unable to scan Redis keys');
  }
}

async function getRedisKeyDetails(
  environmentId: EnvironmentId,
  connection: DatabaseConnection,
  database?: string,
  key?: string,
  nodeName?: string
) {
  try {
    const { data } = await axios.get<RedisKeyDetails>(
      `${databaseConnectionUrl(environmentId, connection)}/redis-key-details`,
      {
        params: { database, key },
        headers: { ...withAgentTargetHeader(nodeName) },
      }
    );

    return data;
  } catch (e) {
    throw parseDatabaseError(e, 'Unable to retrieve Redis key details');
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
    throw parseDatabaseError(e, 'Unable to test database connection');
  }
}

function parseDatabaseError(err: unknown, fallbackMessage: string) {
  const parsedError = parseAxiosError(
    err,
    fallbackMessage
  ) as DatabaseRequestError;
  const responseData =
    typeof err === 'object' && err && 'response' in err
      ? (err.response as { data?: Record<string, unknown> } | undefined)?.data
      : undefined;

  if (responseData) {
    if (typeof responseData.ErrorCode === 'string') {
      parsedError.ErrorCode = responseData.ErrorCode;
    }
    if (typeof responseData.Details === 'string') {
      parsedError.Details = responseData.Details;
    }
    if (typeof responseData.Message === 'string') {
      parsedError.message = responseData.Message;
    }
  }

  return parsedError;
}

function databaseConnectionsUrl(environmentId: EnvironmentId) {
  return `/endpoints/${environmentId}/database-connections`;
}

function databaseConnectionTestUrl(
  environmentId: EnvironmentId,
  _payload: DatabaseConnectionPayload
) {
  return `${databaseConnectionsUrl(environmentId)}/test`;
}

function databaseConnectionUrl(
  environmentId: EnvironmentId,
  connection: number | DatabaseConnection
) {
  const id = typeof connection === 'number' ? connection : connection.Id;
  return `${databaseConnectionsUrl(environmentId)}/${id}`;
}
