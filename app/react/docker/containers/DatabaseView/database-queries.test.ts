import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
}));

vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>();

  return {
    ...actual,
    useQuery: mocks.useQuery,
  };
});

import {
  DatabaseConnection,
  useDatabaseConnections,
  useDatabaseSchema,
  useDatabaseTableDetails,
  useRedisKeys,
} from './database-queries';

const mysqlConnection = {
  Id: 1,
  Type: 'mysql',
} as DatabaseConnection;

const redisConnection = {
  Id: 2,
  Type: 'redis',
} as DatabaseConnection;

describe('database workbench queries', () => {
  beforeEach(() => {
    mocks.useQuery.mockClear();
  });

  it('does not refetch connection, schema, table detail, or Redis key data on window focus', () => {
    useDatabaseConnections(1);
    useDatabaseSchema(1, mysqlConnection);
    useDatabaseTableDetails(1, mysqlConnection, 'app', 'users');
    useRedisKeys(1, redisConnection, '0');

    expect(mocks.useQuery).toHaveBeenCalledTimes(4);
    mocks.useQuery.mock.calls.forEach(([options]) => {
      expect(options.refetchOnWindowFocus).toBe(false);
    });
  });
});
