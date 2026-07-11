import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { useCurrentStateAndParams } from '@uirouter/react';
import { useTranslation } from 'react-i18next';
import { saveAs } from 'file-saver';
import { format as formatSql } from 'sql-formatter';
import {
  ChevronDown,
  ChevronRight,
  Check,
  Copy,
  Database,
  Download,
  Eye,
  FileJson,
  FileSpreadsheet,
  Folder,
  FlaskConical,
  History,
  KeyRound,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Save,
  Square,
  Table2,
  Trash2,
  Wand2,
  X,
} from 'lucide-react';

import { EnvironmentId } from '@/react/portainer/environments/types';
import { useEnvironment } from '@/react/portainer/environments/queries/useEnvironment';
import { useCurrentUser, useIsPureAdmin } from '@/react/hooks/useUser';
import { useContainers } from '@/react/docker/containers/queries/useContainers';
import {
  ContainerListViewModel,
  ContainerStatus,
} from '@/react/docker/containers/types';
import { dispatchCacheRefreshEvent } from '@/portainer/services/http-request.helper';

import { PageHeader } from '@@/PageHeader';
import { Icon } from '@@/Icon';
import { Alert } from '@@/Alert';
import { Button, LoadingButton } from '@@/buttons';
import { FormControl } from '@@/form-components/FormControl';

import {
  DatabaseConnection,
  DatabaseConnectionPayload,
  DatabaseConnectionType,
  DatabaseQueryResult,
  DatabaseSchema,
  DatabaseTableDetails,
  RedisKeyDetails,
  RedisKeySummary,
  useCreateDatabaseConnection,
  useDatabaseConnections,
  useDatabaseSchema,
  useDatabaseTableDetails,
  useDeleteDatabaseConnection,
  useRedisKeyDetails,
  useRedisKeys,
  useRunDatabaseQuery,
  useTestDatabaseConnection,
  useUpdateDatabaseConnection,
} from './database-queries';
import styles from './DatabaseView.module.css';

type ConnectionFormValues = DatabaseConnectionPayload;
type FormMode = 'create' | 'edit';

const defaultFormValues: ConnectionFormValues = {
  Name: '',
  Type: 'mysql',
  Host: '127.0.0.1',
  Port: 3306,
  Database: '',
  Username: '',
  Password: '',
  QueryTimeout: 30,
  ContainerId: '',
};

const maxSelectRows = 200;
const maxRedisTreeKeys = 100;
const defaultSqlComment = '-- SQL 查询结果最多显示 200 行';
const defaultSqlTemplate = defaultSqlComment;
const historyRetentionMs = 7 * 24 * 60 * 60 * 1000;
const redisDatabaseOptions = Array.from({ length: 16 }, (_, index) =>
  String(index)
);

type QueryHistoryItem = {
  id: string;
  query: string;
  executedAt: number;
};

type QueryTab = {
  id: string;
  name: string;
  query: string;
  database: string;
  result?: DatabaseQueryResult;
  error?: DatabaseViewError;
};

type PersistedQueryTabs = {
  activeTabId: string;
  tabs: QueryTab[];
};

type SelectedTable = {
  database: string;
  table: string;
};

type RedisTreeNode = {
  segment: string;
  path: string;
  children: RedisTreeNode[];
  key?: RedisKeySummary;
  keyCount: number;
};

type DatabaseViewError = Error & {
  ErrorCode?: string;
  Details?: string;
};

type TranslateFn = (key: string, options?: Record<string, unknown>) => string;

const portByType: Record<DatabaseConnectionType, number> = {
  mysql: 3306,
  mariadb: 3306,
  postgres: 5432,
  redis: 6379,
};

export function DatabaseView() {
  const { t } = useTranslation();
  const { user } = useCurrentUser();
  const {
    state,
    params: { endpointId },
  } = useCurrentStateAndParams();
  const isPureAdmin = useIsPureAdmin();
  const environmentId = Number(endpointId) as EnvironmentId;
  const isDockerEnvironment = (state.name || '').startsWith('docker.');

  const connectionsQuery = useDatabaseConnections(environmentId);
  const containersQuery = useContainers(environmentId, {
    enabled: isDockerEnvironment,
    select: (containers) =>
      containers.filter((container) =>
        [
          ContainerStatus.Starting,
          ContainerStatus.Running,
          ContainerStatus.Healthy,
          ContainerStatus.Unhealthy,
        ].includes(container.Status)
      ),
  });
  const createConnection = useCreateDatabaseConnection(environmentId);
  const updateConnection = useUpdateDatabaseConnection(environmentId);
  const deleteConnection = useDeleteDatabaseConnection(environmentId);
  const runQuery = useRunDatabaseQuery(environmentId);
  const testConnection = useTestDatabaseConnection(environmentId);
  const environmentQuery = useEnvironment(environmentId, (environment) => ({
    url: environment.URL,
  }));

  const connections = connectionsQuery.data || [];
  const [selectedId, setSelectedId] = useState<number>();
  const [editingId, setEditingId] = useState<number>();
  const [formMode, setFormMode] = useState<FormMode>();
  const [formValues, setFormValues] =
    useState<ConnectionFormValues>(defaultFormValues);
  const [queryTabs, setQueryTabs] = useState<QueryTab[]>([]);
  const [activeTabId, setActiveTabId] = useState('');
  const [selectedTable, setSelectedTable] = useState<SelectedTable>();
  const [redisPattern, setRedisPattern] = useState('*');
  const [selectedRedisKey, setSelectedRedisKey] = useState('');
  const [pendingRedisPreviewKey, setPendingRedisPreviewKey] = useState('');
  const [expandedRedisFolders, setExpandedRedisFolders] = useState<
    Record<string, boolean>
  >({});
  const [history, setHistory] = useState<QueryHistoryItem[]>([]);

  const selectedConnection = useMemo(
    () => connections.find((connection) => connection.Id === selectedId),
    [connections, selectedId]
  );

  const activeConnection = selectedConnection || connections[0];
  const activeContainer = containersQuery.data?.find(
    (container) => container.Id === activeConnection?.ContainerId
  );
  const schemaQuery = useDatabaseSchema(
    environmentId,
    activeConnection,
    activeContainer?.NodeName
  );
  const databaseOptions = useMemo(
    () =>
      activeConnection?.Type === 'redis'
        ? redisDatabaseOptions
        : schemaQuery.data?.Databases.map((database) => database.Name) || [],
    [activeConnection?.Type, schemaQuery.data]
  );
  const activeTab =
    queryTabs.find((tab) => tab.id === activeTabId) || queryTabs[0];
  // Redis 只能使用数字 DB index；从 MySQL 切过来时 tab.database 可能仍是业务库名。
  const activeDatabase = useMemo(() => {
    const raw =
      activeTab?.database ||
      activeConnection?.Database ||
      (activeConnection?.Type === 'redis'
        ? '0'
        : preferredDatabaseOption(databaseOptions, activeConnection?.Type)) ||
      '';

    if (activeConnection?.Type === 'redis') {
      return normalizeRedisDatabase(raw);
    }

    return raw;
  }, [
    activeConnection?.Database,
    activeConnection?.Type,
    activeTab?.database,
    databaseOptions,
  ]);
  const historyStorageKey = `portainer.databaseHistory.${user.Id}.${environmentId}.${activeConnection?.Id || 'none'}`;
  const tabsStorageKey = `portainer.databaseTabs.${user.Id}.${environmentId}.${activeConnection?.Id || 'none'}`;
  const preferPublishedContainerPorts = isLocalAgentUrl(
    environmentQuery.data?.url
  );
  const tableDetailsQuery = useDatabaseTableDetails(
    environmentId,
    activeConnection,
    selectedTable?.database,
    selectedTable?.table,
    activeContainer?.NodeName
  );
  const redisKeysQuery = useRedisKeys(
    environmentId,
    activeConnection,
    activeDatabase,
    redisPattern,
    '0',
    activeContainer?.NodeName
  );
  const redisTreeKeys = useMemo(
    () => (redisKeysQuery.data?.Keys || []).slice(0, maxRedisTreeKeys),
    [redisKeysQuery.data?.Keys]
  );
  const redisKeyDetailsQuery = useRedisKeyDetails(
    environmentId,
    activeConnection,
    activeDatabase,
    selectedRedisKey,
    activeContainer?.NodeName
  );
  const abortControllerRef = useRef<AbortController>();

  useEffect(() => {
    if (!activeConnection) {
      setQueryTabs([]);
      setActiveTabId('');
      return;
    }

    const loadedTabs = loadQueryTabs(
      tabsStorageKey,
      activeConnection,
      databaseOptions,
      t
    );
    setQueryTabs(loadedTabs.tabs);
    setActiveTabId(loadedTabs.activeTabId);
  }, [
    activeConnection?.Id,
    activeConnection?.Database,
    databaseOptions,
    tabsStorageKey,
    t,
  ]);

  useEffect(() => {
    const nextHistory = loadQueryHistory(historyStorageKey);
    setHistory(nextHistory);
    saveQueryHistory(historyStorageKey, nextHistory);
  }, [historyStorageKey]);

  useEffect(() => {
    if (!activeConnection || queryTabs.length === 0) {
      return;
    }

    saveQueryTabs(tabsStorageKey, activeTabId, queryTabs);
  }, [activeConnection?.Id, activeTabId, queryTabs, tabsStorageKey]);

  useEffect(() => {
    setSelectedTable(undefined);
    setSelectedRedisKey('');
    setPendingRedisPreviewKey('');
    setExpandedRedisFolders({});
  }, [
    activeConnection?.Id,
    activeConnection?.Type,
    activeConnection?.Database,
  ]);

  // 仅在用户主动点选 Key 时写入结果区；避免详情查询后台刷新覆盖命令执行结果。
  useEffect(() => {
    if (
      activeConnection?.Type !== 'redis' ||
      !pendingRedisPreviewKey ||
      !redisKeyDetailsQuery.data ||
      redisKeyDetailsQuery.data.Name !== pendingRedisPreviewKey ||
      !activeTab
    ) {
      return;
    }

    setQueryTabs((tabs) =>
      tabs.map((tab) =>
        tab.id === activeTab.id
          ? {
              ...tab,
              result: redisDetailsToQueryResult(redisKeyDetailsQuery.data!),
              error: undefined,
            }
          : tab
      )
    );
    setPendingRedisPreviewKey('');
  }, [
    activeConnection?.Type,
    activeTab?.id,
    pendingRedisPreviewKey,
    redisKeyDetailsQuery.data,
  ]);

  function selectConnection(connection: DatabaseConnection) {
    // 先清空旧 tabs，避免切到 Redis 时把 MySQL 的库名写进新连接缓存并触发 SCAN。
    setSelectedId(connection.Id);
    setQueryTabs([]);
    setActiveTabId('');
    setSelectedRedisKey('');
    setPendingRedisPreviewKey('');
    setExpandedRedisFolders({});
  }

  function openCreateForm() {
    setEditingId(undefined);
    setFormMode('create');
    setFormValues(defaultFormValues);
  }

  function openEditForm(connection: DatabaseConnection) {
    setSelectedId(connection.Id);
    setEditingId(connection.Id);
    setFormMode('edit');
    setFormValues({
      Name: connection.Name,
      Type: connection.Type,
      Host: connection.Host,
      Port: connection.Port,
      Database: connection.Database,
      Username: connection.Username,
      Password: '',
      QueryTimeout: connection.QueryTimeout,
      ContainerId: connection.ContainerId || '',
    });
  }

  function closeForm() {
    setEditingId(undefined);
    setFormMode(undefined);
    setFormValues(defaultFormValues);
  }

  async function handleSave(event: FormEvent) {
    event.preventDefault();

    const payload = normalizePayload(formValues, {
      preserveBlankPassword: !!editingId,
    });

    if (editingId) {
      const updated = await updateConnection.mutateAsync({
        id: editingId,
        payload,
      });
      setSelectedId(updated.Id);
    } else {
      const created = await createConnection.mutateAsync(payload);
      setSelectedId(created.Id);
    }

    closeForm();
  }

  async function handleDelete(connection: DatabaseConnection) {
    if (
      !window.confirm(
        t('legacyText.Delete database connection "{{name}}"?', {
          name: connection.Name,
          defaultValue: `Delete database connection "${connection.Name}"?`,
        })
      )
    ) {
      return;
    }

    await deleteConnection.mutateAsync(connection.Id);

    if (selectedId === connection.Id) {
      setSelectedId(undefined);
    }
    if (editingId === connection.Id) {
      closeForm();
    }
  }

  async function handleRunQuery(queryToRun?: string) {
    const statement = prepareQueryForExecution(
      queryToRun || activeTab?.query || ''
    );
    if (!activeConnection || !activeTab || !statement) {
      return;
    }

    // 命令执行优先于 Key 预览，避免旧详情结果盖住新查询的值列。
    setPendingRedisPreviewKey('');
    setSelectedRedisKey('');

    const queryDatabase =
      activeConnection.Type === 'redis'
        ? normalizeRedisDatabase(activeDatabase)
        : activeDatabase;
    const isWriteStatement = isUpdateOrDeleteStatement(statement);
    const unsafeWrite = isUnsafeWriteStatement(statement);
    const abortController = new AbortController();
    abortControllerRef.current = abortController;
    updateActiveTab({ error: undefined });

    try {
      if (isWriteStatement) {
        const preview = await runQuery.mutateAsync({
          connection: activeConnection,
          query: statement,
          database: queryDatabase,
          preview: true,
          nodeName: activeContainer?.NodeName,
          signal: abortController.signal,
        });

        const confirmMessage = unsafeWrite
          ? t(
              'legacyText.This statement has no WHERE clause and will affect {{count}} row(s). Execute it?',
              {
                count: preview.RowsAffected || 0,
                defaultValue: `This statement has no WHERE clause and will affect ${
                  preview.RowsAffected || 0
                } row(s). Execute it?`,
              }
            )
          : t(
              'legacyText.This statement will affect {{count}} row(s). Execute it?',
              {
                count: preview.RowsAffected || 0,
                defaultValue: `This statement will affect ${
                  preview.RowsAffected || 0
                } row(s). Execute it?`,
              }
            );

        const confirmed = window.confirm(confirmMessage);

        if (!confirmed) {
          return;
        }
      }

      const result = await runQuery.mutateAsync({
        connection: activeConnection,
        query: statement,
        database: queryDatabase,
        confirmUnsafeWrite: unsafeWrite,
        nodeName: activeContainer?.NodeName,
        signal: abortController.signal,
      });

      if (result.RequiresConfirmation) {
        updateActiveTab({
          result,
          error: databaseResultError(result),
        });
        return;
      }

      updateActiveTab({ result, error: undefined });
      appendQueryHistory(historyStorageKey, statement, setHistory);
    } catch (error) {
      if (abortController.signal.aborted) {
        updateActiveTab({
          error: databaseCanceledError(),
        });
        return;
      }

      updateActiveTab({
        error: error as DatabaseViewError,
      });
    } finally {
      if (abortControllerRef.current === abortController) {
        abortControllerRef.current = undefined;
      }
    }
  }

  function cancelRunningQuery() {
    abortControllerRef.current?.abort();
  }

  function updateActiveTab(patch: Partial<QueryTab>) {
    if (!activeTab) {
      return;
    }

    setQueryTabs((tabs) =>
      tabs.map((tab) => (tab.id === activeTab.id ? { ...tab, ...patch } : tab))
    );
  }

  function updateActiveTabQuery(query: string) {
    updateActiveTab({ query });
  }

  function updateActiveTabDatabase(database: string) {
    const nextDatabase =
      activeConnection?.Type === 'redis'
        ? normalizeRedisDatabase(database)
        : database;
    updateActiveTab({ database: nextDatabase });
    if (activeConnection?.Type === 'redis') {
      setSelectedRedisKey('');
      setPendingRedisPreviewKey('');
      setExpandedRedisFolders({});
    }
  }

  function addQueryTab() {
    if (!activeConnection) {
      return;
    }

    const nextTab = createDefaultQueryTab(
      activeConnection,
      databaseOptions,
      queryTabs.length + 1,
      t
    );

    setQueryTabs((tabs) => [...tabs, nextTab]);
    setActiveTabId(nextTab.id);
  }

  function closeQueryTab(tabId: string) {
    setQueryTabs((tabs) => {
      if (tabs.length <= 1) {
        return tabs;
      }

      const nextTabs = tabs.filter((tab) => tab.id !== tabId);
      if (activeTabId === tabId) {
        setActiveTabId(nextTabs[0]?.id || '');
      }

      return nextTabs;
    });
  }

  function formatActiveTabQuery(selectionStart: number, selectionEnd: number) {
    if (!activeConnection || !activeTab || activeConnection.Type === 'redis') {
      return;
    }

    const targetSql =
      selectionStart !== selectionEnd
        ? activeTab.query.slice(selectionStart, selectionEnd)
        : activeTab.query;

    try {
      const formatted = formatSql(targetSql, {
        language: activeConnection.Type === 'postgres' ? 'postgresql' : 'mysql',
      });

      if (selectionStart !== selectionEnd) {
        updateActiveTabQuery(
          `${activeTab.query.slice(0, selectionStart)}${formatted}${activeTab.query.slice(selectionEnd)}`
        );
        return;
      }

      updateActiveTabQuery(formatted);
    } catch (error) {
      updateActiveTab({ error: error as DatabaseViewError });
    }
  }

  // 单击表名负责展开/收起结构，双击表名才生成 SELECT，避免误改 SQL 编辑区。
  function selectTable(database: string, table: string) {
    setSelectedTable((current) =>
      current?.database === database && current.table === table
        ? undefined
        : { database, table }
    );
  }

  function insertTableSelect(database: string, table: string) {
    if (!activeConnection || !activeTab) {
      return;
    }

    updateActiveTabQuery(
      appendSqlStatement(
        activeTab.query,
        tableSelectStatement(activeConnection.Type, database, table)
      )
    );
  }

  // 双击 Key 写入对应读命令到编辑区，单击只预览结果，避免误改命令草稿。
  function insertRedisKeyCommand(key: RedisKeySummary) {
    if (!activeTab) {
      return;
    }

    updateActiveTabQuery(
      appendSqlStatement(activeTab.query, redisKeyReadCommand(key))
    );
  }

  function toggleRedisFolder(path: string) {
    setExpandedRedisFolders((expanded) => ({
      ...expanded,
      [path]: !expanded[path],
    }));
  }

  if (!isPureAdmin) {
    return null;
  }

  return (
    <div className={`${styles.root} flex h-full flex-col`}>
      <PageHeader
        breadcrumbs={t('legacyText.Databases', { defaultValue: 'Databases' })}
      />

      <div className="flex min-h-[560px] flex-1 gap-4 overflow-hidden">
        <aside className="flex h-full min-h-0 w-[300px] shrink-0 flex-col overflow-hidden border-r border-gray-5 pr-4">
          <ConnectionSidebar
            connections={connections}
            activeConnection={activeConnection}
            isLoading={connectionsQuery.isLoading}
            schema={schemaQuery.data}
            isSchemaLoading={schemaQuery.isLoading}
            schemaError={schemaQuery.error as DatabaseViewError | undefined}
            databaseFilter={activeDatabase}
            selectedTable={selectedTable}
            tableDetails={tableDetailsQuery.data}
            isTableDetailsLoading={tableDetailsQuery.isLoading}
            redisKeys={redisTreeKeys}
            isRedisKeysLoading={redisKeysQuery.isLoading}
            redisPattern={redisPattern}
            selectedRedisKey={selectedRedisKey}
            expandedRedisFolders={expandedRedisFolders}
            onSelectConnection={selectConnection}
            onCreate={openCreateForm}
            onEdit={openEditForm}
            onDelete={handleDelete}
            onSelectTable={selectTable}
            onInsertTableSelect={insertTableSelect}
            onRedisPatternChange={(pattern) => {
              setRedisPattern(normalizeRedisSearchPattern(pattern));
            }}
            onRedisRefresh={() => redisKeysQuery.refetch()}
            onSelectRedisKey={(key) => {
              setSelectedRedisKey(key);
              setPendingRedisPreviewKey(key);
            }}
            onInsertRedisKeyCommand={insertRedisKeyCommand}
            onToggleRedisFolder={toggleRedisFolder}
          />
        </aside>

        <main className="flex h-full min-w-0 flex-1 flex-col overflow-hidden">
          <QueryWorkspace
            activeConnection={activeConnection}
            databases={databaseOptions}
            tabs={queryTabs}
            activeTabId={activeTabId}
            selectedDatabase={activeDatabase}
            history={history}
            isRunning={runQuery.isLoading}
            onSelectTab={setActiveTabId}
            onAddTab={addQueryTab}
            onCloseTab={closeQueryTab}
            onSelectDatabase={updateActiveTabDatabase}
            onQueryChange={updateActiveTabQuery}
            onFormat={formatActiveTabQuery}
            onRun={handleRunQuery}
            onCancel={cancelRunningQuery}
            onRefresh={() => {
              // 顶部刷新下放到运行按钮旁：刷新连接/库表/Redis Key，避免整页重载打断编辑。
              dispatchCacheRefreshEvent();
              connectionsQuery.refetch();
              schemaQuery.refetch();
              if (activeConnection?.Type === 'redis') {
                redisKeysQuery.refetch();
              }
            }}
          />
        </main>
      </div>

      {formMode && (
        <div className="fixed inset-y-0 right-0 z-50 flex w-[420px] max-w-full flex-col border-l border-gray-5 bg-gray-10 p-5 shadow-2xl">
          <ConnectionForm
            values={formValues}
            isEditing={formMode === 'edit'}
            hasSavedPassword={
              !!editingId &&
              !!connections.find((connection) => connection.Id === editingId)
                ?.HasPassword
            }
            isLoading={createConnection.isLoading || updateConnection.isLoading}
            isTesting={testConnection.isLoading}
            onCancel={closeForm}
            onTest={async (values) => {
              const payload = normalizePayload(
                {
                  ...values,
                  Name: values.Name || 'test',
                },
                {
                  connectionId: editingId,
                  preserveBlankPassword: !!editingId,
                }
              );
              const container = containersQuery.data?.find(
                (container) => container.Id === payload.ContainerId
              );

              await testConnection.mutateAsync({
                payload,
                nodeName: container?.NodeName,
              });
            }}
            onSave={handleSave}
            onChange={setFormValues}
            containers={containersQuery.data || []}
            isContainerTargetVisible={isDockerEnvironment}
            preferPublishedContainerPorts={preferPublishedContainerPorts}
          />
        </div>
      )}
    </div>
  );
}

function ConnectionSidebar({
  connections,
  activeConnection,
  isLoading,
  schema,
  isSchemaLoading,
  schemaError,
  databaseFilter,
  selectedTable,
  tableDetails,
  isTableDetailsLoading,
  redisKeys,
  isRedisKeysLoading,
  redisPattern,
  selectedRedisKey,
  expandedRedisFolders,
  onSelectConnection,
  onCreate,
  onEdit,
  onDelete,
  onSelectTable,
  onInsertTableSelect,
  onRedisPatternChange,
  onRedisRefresh,
  onSelectRedisKey,
  onInsertRedisKeyCommand,
  onToggleRedisFolder,
}: {
  connections: DatabaseConnection[];
  activeConnection?: DatabaseConnection;
  isLoading: boolean;
  schema?: DatabaseSchema;
  isSchemaLoading: boolean;
  schemaError?: DatabaseViewError;
  databaseFilter?: string;
  selectedTable?: SelectedTable;
  tableDetails?: DatabaseTableDetails;
  isTableDetailsLoading: boolean;
  redisKeys: RedisKeySummary[];
  isRedisKeysLoading: boolean;
  redisPattern: string;
  selectedRedisKey: string;
  expandedRedisFolders: Record<string, boolean>;
  onSelectConnection: (connection: DatabaseConnection) => void;
  onCreate: () => void;
  onEdit: (connection: DatabaseConnection) => void;
  onDelete: (connection: DatabaseConnection) => void;
  onSelectTable: (database: string, table: string) => void;
  onInsertTableSelect: (database: string, table: string) => void;
  onRedisPatternChange: (pattern: string) => void;
  onRedisRefresh: () => void;
  onSelectRedisKey: (key: string) => void;
  onInsertRedisKeyCommand: (key: RedisKeySummary) => void;
  onToggleRedisFolder: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [isConnectionPickerOpen, setIsConnectionPickerOpen] = useState(false);
  const [connectionSearch, setConnectionSearch] = useState('');
  const connectionPickerRef = useRef<HTMLDivElement>(null);
  const isRedisConnection = activeConnection?.Type === 'redis';
  const filteredConnections = useMemo(
    () =>
      connections.filter((connection) =>
        isConnectionMatched(connection, connectionSearch)
      ),
    [connections, connectionSearch]
  );

  // 连接选择器是临时弹层：点击外部或按 Escape 时收起，避免遮挡库表树继续操作。
  useEffect(() => {
    if (!isConnectionPickerOpen) {
      return undefined;
    }

    function closeOnOutsideClick(event: MouseEvent) {
      if (
        event.target instanceof Node &&
        !connectionPickerRef.current?.contains(event.target)
      ) {
        setIsConnectionPickerOpen(false);
      }
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setIsConnectionPickerOpen(false);
      }
    }

    document.addEventListener('mousedown', closeOnOutsideClick);
    document.addEventListener('keydown', closeOnEscape);

    return () => {
      document.removeEventListener('mousedown', closeOnOutsideClick);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [isConnectionPickerOpen]);

  function toggleConnectionPicker() {
    if (connections.length === 0) {
      return;
    }

    if (!isConnectionPickerOpen) {
      setConnectionSearch('');
    }
    setIsConnectionPickerOpen((isOpen) => !isOpen);
  }

  function chooseConnection(connection: DatabaseConnection) {
    onSelectConnection(connection);
    setConnectionSearch('');
    setIsConnectionPickerOpen(false);
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div ref={connectionPickerRef} className="relative shrink-0 pb-2">
        <div className="flex items-center gap-2 pb-2">
          <Icon icon={Database} className="lucide" />
          <span className="font-semibold">
            {t('panelTitles.Connections', { defaultValue: 'Connections' })}
          </span>
          <Button
            type="button"
            color="default"
            size="small"
            title={t('buttonTitles.Select database connection', {
              defaultValue: 'Select database connection',
            })}
            disabled={connections.length === 0}
            onClick={toggleConnectionPicker}
            data-cy="database-select-connection-button"
          >
            <Icon
              icon={ChevronDown}
              className={`lucide transition-transform ${
                isConnectionPickerOpen ? 'rotate-180' : ''
              }`}
            />
          </Button>
          <Button
            type="button"
            color="default"
            size="small"
            className="ml-auto"
            title={t('buttonTitles.Add database connection', {
              defaultValue: 'Add database connection',
            })}
            onClick={onCreate}
            data-cy="database-add-connection-button"
          >
            <Icon icon={Plus} className="lucide" />
          </Button>
        </div>

        <div
          role="button"
          tabIndex={connections.length > 0 ? 0 : -1}
          aria-label={t('buttonTitles.Select database connection', {
            defaultValue: 'Select database connection',
          })}
          className={`flex w-full items-center gap-2 rounded border px-2 py-2 text-left transition ${
            connections.length > 0
              ? 'cursor-pointer border-blue-8 bg-blue-8 text-white hover:bg-blue-9'
              : 'border-gray-5 bg-white text-gray-7 th-dark:bg-gray-iron-11 th-dark:text-gray-4'
          }`}
          onClick={toggleConnectionPicker}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              toggleConnectionPicker();
            }
          }}
        >
          {activeConnection ? (
            <>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-semibold">
                    {activeConnection.Name}
                  </span>
                  <span className="label label-default shrink-0">
                    {activeConnection.Type}
                  </span>
                </div>
                <div className="mt-0.5 truncate text-[11px] opacity-80">
                  {connectionSourceLabel(activeConnection, t)}
                  {' - '}
                  {connectionTargetLabel(activeConnection)}
                </div>
              </div>
              <Icon icon={ChevronDown} className="lucide shrink-0 opacity-80" />
            </>
          ) : (
            <span className="text-sm">
              {isLoading
                ? t('common.Loading...', { defaultValue: 'Loading...' })
                : t('legacyText.No saved connections', {
                    defaultValue: 'No saved connections',
                  })}
            </span>
          )}
        </div>

        {isConnectionPickerOpen && (
          <div className="absolute left-0 right-0 top-full z-20 mt-1 overflow-hidden rounded border border-gray-5 bg-white shadow-xl th-dark:bg-gray-iron-11">
            <div className="border-b border-gray-5 p-2">
              <input
                className="form-control h-8 w-full"
                value={connectionSearch}
                onChange={(event) => setConnectionSearch(event.target.value)}
                placeholder={t('placeholders.Search database connections', {
                  defaultValue: 'Search connections',
                })}
                aria-label={t('placeholders.Search database connections', {
                  defaultValue: 'Search connections',
                })}
              />
            </div>

            <div className="max-h-[320px] overflow-y-auto p-1 [color-scheme:light] th-dark:[color-scheme:dark]">
              {isLoading && (
                <span className="text-muted block px-2 py-1">
                  {t('common.Loading...', { defaultValue: 'Loading...' })}
                </span>
              )}
              {!isLoading && connections.length === 0 && (
                <span className="text-muted block px-2 py-1">
                  {t('legacyText.No saved connections', {
                    defaultValue: 'No saved connections',
                  })}
                </span>
              )}
              {!isLoading &&
                connections.length > 0 &&
                filteredConnections.length === 0 && (
                  <span className="text-muted block px-2 py-1">
                    {t('legacyText.No matching connections', {
                      defaultValue: 'No matching connections',
                    })}
                  </span>
                )}
              {filteredConnections.map((connection) => {
                const isActive = activeConnection?.Id === connection.Id;

                return (
                  <div
                    key={connection.Id}
                    role="button"
                    tabIndex={0}
                    className={`group mb-1 flex w-full items-center gap-2 rounded border px-2 py-2 text-left transition ${
                      isActive
                        ? 'border-blue-8 bg-blue-2 text-blue-10 th-dark:bg-gray-iron-10 th-dark:text-gray-1'
                        : 'border-transparent text-gray-10 hover:border-gray-5 hover:bg-gray-2 th-dark:text-gray-2 th-dark:hover:bg-gray-iron-10'
                    }`}
                    onClick={() => chooseConnection(connection)}
                    onKeyDown={(event) => {
                      if (event.target !== event.currentTarget) {
                        return;
                      }
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        chooseConnection(connection);
                      }
                    }}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="truncate text-sm font-semibold">
                          {connection.Name}
                        </span>
                        <span className="label label-default shrink-0">
                          {connection.Type}
                        </span>
                      </div>
                      <div className="mt-0.5 truncate text-[11px] opacity-80">
                        {connectionSourceLabel(connection, t)}
                        {' - '}
                        {connectionTargetLabel(connection)}
                      </div>
                    </div>

                    {isActive && (
                      <Icon icon={Check} className="lucide shrink-0" />
                    )}
                    <div className="flex shrink-0 gap-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
                      <Button
                        type="button"
                        color="default"
                        size="small"
                        title={t('buttonTitles.Edit', {
                          defaultValue: 'Edit',
                        })}
                        onClick={(event) => {
                          event.stopPropagation();
                          setIsConnectionPickerOpen(false);
                          onEdit(connection);
                        }}
                        data-cy="database-edit-connection-button"
                      >
                        <Icon icon={Pencil} className="lucide" />
                      </Button>
                      <Button
                        type="button"
                        color="dangerlight"
                        size="small"
                        title={t('buttons.Remove', {
                          defaultValue: 'Remove',
                        })}
                        onClick={(event) => {
                          event.stopPropagation();
                          setIsConnectionPickerOpen(false);
                          onDelete(connection);
                        }}
                        data-cy="database-delete-connection-button"
                      >
                        <Icon icon={Trash2} className="lucide" />
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden border-t border-gray-5 pt-1.5">
        {isRedisConnection ? (
          <RedisKeyTree
            keys={redisKeys}
            isLoading={isRedisKeysLoading}
            pattern={redisPattern}
            selectedKey={selectedRedisKey}
            expandedFolders={expandedRedisFolders}
            onPatternChange={onRedisPatternChange}
            onRefresh={onRedisRefresh}
            onSelectKey={onSelectRedisKey}
            onInsertKeyCommand={onInsertRedisKeyCommand}
            onToggleFolder={onToggleRedisFolder}
          />
        ) : (
          <SchemaTree
            schema={schema}
            isLoading={isSchemaLoading}
            error={schemaError}
            databaseFilter={databaseFilter}
            selectedTable={selectedTable}
            tableDetails={tableDetails}
            isTableDetailsLoading={isTableDetailsLoading}
            onSelectTable={onSelectTable}
            onInsertTableSelect={onInsertTableSelect}
          />
        )}
      </div>
    </div>
  );
}

function connectionSourceLabel(connection: DatabaseConnection, t: TranslateFn) {
  return connection.ContainerId
    ? t('legacyText.Container', {
        defaultValue: 'Container',
      })
    : t('legacyText.Custom address', {
        defaultValue: 'Custom address',
      });
}

function connectionTargetLabel(connection: DatabaseConnection) {
  const database = connection.Database ? ` / ${connection.Database}` : '';
  return `${connection.Host}:${connection.Port}${database}`;
}

// 连接数量变多后，搜索需要同时覆盖名称、类型和连接目标；多关键字用空格分隔并全部命中。
function isConnectionMatched(connection: DatabaseConnection, search: string) {
  const keywords = search.trim().toLowerCase().split(/\s+/).filter(Boolean);

  if (keywords.length === 0) {
    return true;
  }

  const searchableText = [
    connection.Name,
    connection.Type,
    connection.Host,
    String(connection.Port),
    connection.Database,
    connection.ContainerId,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();

  return keywords.every((keyword) => searchableText.includes(keyword));
}

function SchemaTree({
  schema,
  isLoading,
  error,
  databaseFilter,
  selectedTable,
  tableDetails,
  isTableDetailsLoading,
  onSelectTable,
  onInsertTableSelect,
}: {
  schema?: DatabaseSchema;
  isLoading: boolean;
  error?: DatabaseViewError;
  databaseFilter?: string;
  selectedTable?: SelectedTable;
  tableDetails?: DatabaseTableDetails;
  isTableDetailsLoading: boolean;
  onSelectTable: (database: string, table: string) => void;
  onInsertTableSelect: (database: string, table: string) => void;
}) {
  const { t } = useTranslation();
  const [searchDraft, setSearchDraft] = useState('');
  const [tableSearch, setTableSearch] = useState('');

  useEffect(() => {
    setSearchDraft('');
    setTableSearch('');
  }, [databaseFilter, schema?.Databases?.length]);

  // 当前库已由右上角下拉确定，侧栏只扁平展示表名，不再套一层库名节点。
  const filteredTables = useMemo(() => {
    const source = databaseFilter
      ? schema?.Databases.filter((database) => database.Name === databaseFilter)
      : schema?.Databases;
    if (!source) {
      return [];
    }

    const keyword = tableSearch.trim().toLowerCase();
    const tables: Array<{ database: string; name: string }> = [];
    source.forEach((database) => {
      database.Tables.forEach((table) => {
        if (!keyword || table.Name.toLowerCase().includes(keyword)) {
          tables.push({ database: database.Name, name: table.Name });
        }
      });
    });
    return tables;
  }, [databaseFilter, schema?.Databases, tableSearch]);

  function submitSearch() {
    setTableSearch(searchDraft.trim());
  }

  if (isLoading) {
    return (
      <span className="text-muted">
        {t('legacyText.Loading database tree...', {
          defaultValue: 'Loading database tree...',
        })}
      </span>
    );
  }

  if (error) {
    return <span className="text-warning">{databaseErrorLabel(error, t)}</span>;
  }

  if (schema?.Message) {
    return (
      <span className="text-muted">
        {t(`legacyText.${schema.Message}`, { defaultValue: schema.Message })}
      </span>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="mb-1.5 flex items-center gap-2">
        <input
          className="form-control h-8 min-w-0 flex-1"
          value={searchDraft}
          onChange={(event) => setSearchDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              submitSearch();
            }
          }}
          placeholder={t('placeholders.Search tables', {
            defaultValue: 'Search tables (fuzzy)',
          })}
          aria-label={t('placeholders.Search tables', {
            defaultValue: 'Search tables (fuzzy)',
          })}
        />
        <Button
          type="button"
          color="default"
          size="small"
          onClick={submitSearch}
          data-cy="database-schema-search-button"
        >
          {t('buttons.Search', { defaultValue: 'Search' })}
        </Button>
      </div>

      {filteredTables.length === 0 ? (
        <span className="text-muted px-1.5 py-1">
          {tableSearch
            ? t('legacyText.No matching tables', {
                defaultValue: 'No matching tables',
              })
            : t('legacyText.No tables', { defaultValue: 'No tables' })}
        </span>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto pr-1 text-sm [color-scheme:light] th-dark:[color-scheme:dark]">
          {filteredTables.map((table) => {
            const isSelected =
              selectedTable?.database === table.database &&
              selectedTable?.table === table.name;

            return (
              <div key={`${table.database}.${table.name}`}>
                <button
                  type="button"
                  className={`flex w-full items-center gap-1.5 rounded border-0 px-1.5 py-1 text-left text-xs transition-colors ${
                    isSelected
                      ? 'bg-blue-2 text-blue-9 th-dark:bg-gray-iron-10 th-dark:text-gray-1'
                      : 'bg-transparent text-gray-9 hover:bg-gray-2 hover:text-gray-10 th-dark:text-gray-4 th-dark:hover:bg-gray-iron-10 th-dark:hover:text-gray-1'
                  }`}
                  onClick={() => onSelectTable(table.database, table.name)}
                  onDoubleClick={() =>
                    onInsertTableSelect(table.database, table.name)
                  }
                >
                  <Icon icon={Table2} className="lucide" />
                  <span className="truncate" data-legacy-i18n-skip="true">
                    {table.name}
                  </span>
                </button>
                {isSelected && (
                  <InlineTableStructure
                    details={tableDetails}
                    isLoading={isTableDetailsLoading}
                    tableName={table.name}
                  />
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function QueryWorkspace({
  activeConnection,
  databases,
  tabs,
  activeTabId,
  selectedDatabase,
  history,
  isRunning,
  onSelectTab,
  onAddTab,
  onCloseTab,
  onSelectDatabase,
  onQueryChange,
  onFormat,
  onRun,
  onCancel,
  onRefresh,
}: {
  activeConnection?: DatabaseConnection;
  databases: string[];
  tabs: QueryTab[];
  activeTabId: string;
  selectedDatabase: string;
  history: QueryHistoryItem[];
  isRunning: boolean;
  onSelectTab: (tabId: string) => void;
  onAddTab: () => void;
  onCloseTab: (tabId: string) => void;
  onSelectDatabase: (database: string) => void;
  onQueryChange: (query: string) => void;
  onFormat: (selectionStart: number, selectionEnd: number) => void;
  onRun: (queryToRun?: string) => void;
  onCancel: () => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  const workspaceRef = useRef<HTMLDivElement>(null);
  const editorPanelRef = useRef<HTMLDivElement>(null);
  const resizeStartRef = useRef<{ y: number; height: number }>();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [editorScrollTop, setEditorScrollTop] = useState(0);
  const [editorHeight, setEditorHeight] = useState(280);
  const [isResizingEditor, setIsResizingEditor] = useState(false);
  const [isHistoryOpen, setIsHistoryOpen] = useState(false);
  const activeTab = tabs.find((tab) => tab.id === activeTabId) || tabs[0];
  const query = activeTab?.query || '';
  const isRedisConnection = activeConnection?.Type === 'redis';
  const lineNumbers = useMemo(
    () => Array.from({ length: Math.max(query.split('\n').length, 1) }),
    [query]
  );

  useEffect(() => {
    if (!isResizingEditor) {
      return undefined;
    }

    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'row-resize';
    document.body.style.userSelect = 'none';

    function handleMouseMove(event: MouseEvent) {
      const start = resizeStartRef.current;
      if (!start) {
        return;
      }

      const workspaceHeight = workspaceRef.current?.clientHeight || 720;
      const maxHeight = Math.max(220, workspaceHeight - 260);
      const nextHeight = start.height + event.clientY - start.y;
      setEditorHeight(Math.min(maxHeight, Math.max(160, nextHeight)));
    }

    function handleMouseUp() {
      resizeStartRef.current = undefined;
      setIsResizingEditor(false);
    }

    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);

    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
    };
  }, [isResizingEditor]);

  // 拖动分隔条时只调整 SQL 编辑区高度，结果区保持 flex 占用剩余空间。
  function startEditorResize(event: {
    preventDefault(): void;
    clientY: number;
  }) {
    event.preventDefault();
    resizeStartRef.current = {
      y: event.clientY,
      height:
        editorPanelRef.current?.getBoundingClientRect().height || editorHeight,
    };
    setIsResizingEditor(true);
  }

  function formatSelectedOrAllQuery() {
    const textarea = textareaRef.current;
    if (!textarea) {
      onFormat(0, query.length);
      return;
    }

    onFormat(textarea.selectionStart, textarea.selectionEnd);
  }

  function selectedOrAllQuery() {
    const textarea = textareaRef.current;
    if (!textarea) {
      return query;
    }

    const selected = query.slice(
      textarea.selectionStart,
      textarea.selectionEnd
    );
    return selected.trim() ? selected : query;
  }

  return (
    <>
      <div
        ref={workspaceRef}
        className="flex min-h-0 flex-1 flex-col overflow-hidden"
      >
        <div className="mb-2 flex shrink-0 items-center gap-1 overflow-x-auto border-b border-gray-5 pb-2">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className={`flex max-w-[180px] shrink-0 items-center gap-2 rounded-t border border-b-0 px-3 py-1.5 text-sm ${
                tab.id === activeTab?.id
                  ? 'border-blue-8 bg-white text-blue-9 th-dark:bg-gray-iron-11 th-dark:text-white'
                  : 'border-gray-5 bg-gray-1 text-gray-8 hover:bg-gray-2 th-dark:bg-gray-iron-10 th-dark:text-gray-3 th-dark:hover:bg-gray-iron-9'
              }`}
              onClick={() => onSelectTab(tab.id)}
            >
              <span className="truncate">{tab.name}</span>
              {tabs.length > 1 && (
                <span
                  role="button"
                  tabIndex={0}
                  className="rounded p-0.5 hover:bg-black/10 th-dark:hover:bg-white/10"
                  onClick={(event) => {
                    event.stopPropagation();
                    onCloseTab(tab.id);
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      event.stopPropagation();
                      onCloseTab(tab.id);
                    }
                  }}
                >
                  <Icon icon={X} className="lucide" />
                </span>
              )}
            </button>
          ))}
          <Button
            type="button"
            color="default"
            size="small"
            onClick={onAddTab}
            data-cy="database-add-query-tab-button"
          >
            <Icon icon={Plus} className="lucide" />
          </Button>
        </div>

        <div className="flex shrink-0 items-center gap-3 pb-3">
          <button
            type="button"
            className="flex items-center border-0 bg-transparent p-0 font-semibold"
            onClick={() => setIsHistoryOpen(true)}
            data-cy="database-query-history-button"
          >
            <Icon icon={History} className="lucide space-right" />
            {t('panelTitles.Execution history', {
              defaultValue: 'Execution history',
            })}
          </button>
          <select
            className="form-control ml-auto max-w-[280px]"
            value={selectedDatabase}
            onChange={(event) => onSelectDatabase(event.target.value)}
            disabled={databases.length === 0}
            aria-label={
              isRedisConnection
                ? t('legacyText.Redis database', {
                    defaultValue: 'Redis database',
                  })
                : t('legacyText.Database', {
                    defaultValue: 'Database',
                  })
            }
          >
            {!selectedDatabase && (
              <option value="">
                {t('placeholders.Select...', { defaultValue: 'Select...' })}
              </option>
            )}
            {databases.map((database) => (
              <option key={database} value={database}>
                {database}
              </option>
            ))}
          </select>
          <Button
            type="button"
            color="default"
            disabled={!query.trim() || isRedisConnection}
            onClick={formatSelectedOrAllQuery}
            data-cy="database-format-query-button"
          >
            <Icon icon={Wand2} className="lucide space-right" />
            {t('buttons.Format', { defaultValue: 'Format' })}
          </Button>
          {isRunning ? (
            <Button
              type="button"
              color="dangerlight"
              onClick={onCancel}
              data-cy="database-cancel-query-button"
            >
              <Icon icon={Square} className="lucide space-right" />
              {t('buttons.Cancel', { defaultValue: 'Cancel' })}
            </Button>
          ) : (
            <Button
              type="button"
              color="primary"
              onClick={() => onRun(selectedOrAllQuery())}
              disabled={!activeConnection || !query.trim()}
              data-cy="database-run-query-button"
            >
              <Icon icon={Play} className="lucide space-right" />
              {t('buttons.Run', { defaultValue: 'Run' })}
            </Button>
          )}
          <Button
            type="button"
            color="default"
            onClick={onRefresh}
            title={t('pageTitles.Refresh page', {
              defaultValue: 'Refresh page',
            })}
            data-cy="database-refresh-workspace-button"
          >
            <Icon icon={RefreshCw} className="lucide" />
          </Button>
        </div>

        <div
          ref={editorPanelRef}
          className="flex min-h-[160px] shrink-0 overflow-hidden rounded border border-gray-5 bg-white text-gray-10 th-dark:bg-gray-iron-11 th-dark:text-white"
          style={{ height: editorHeight }}
        >
          <div className="w-12 shrink-0 overflow-hidden border-r border-gray-5 bg-gray-2 text-right font-mono text-xs leading-5 text-gray-6 th-dark:bg-gray-iron-10 th-dark:text-gray-5">
            <div
              className="px-2 py-2"
              style={{ transform: `translateY(-${editorScrollTop}px)` }}
            >
              {lineNumbers.map((_, index) => (
                <div key={index}>{index + 1}</div>
              ))}
            </div>
          </div>
          <textarea
            ref={textareaRef}
            className="min-h-full flex-1 resize-none border-0 bg-transparent p-2 font-mono leading-5 text-gray-10 outline-none placeholder:text-gray-6 th-dark:text-white th-dark:placeholder:text-gray-6"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            onScroll={(event) =>
              setEditorScrollTop(event.currentTarget.scrollTop)
            }
            onKeyDown={(event) => {
              if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
                event.preventDefault();
                onRun(selectedOrAllQuery());
              }
            }}
            placeholder={isRedisConnection ? 'PING' : defaultSqlTemplate}
            aria-label={isRedisConnection ? 'Command' : 'SQL'}
          />
        </div>

        <div
          role="separator"
          aria-orientation="horizontal"
          className="my-1 flex h-4 shrink-0 cursor-row-resize items-center justify-center"
          title={t('buttonTitles.Drag to resize editor and results', {
            defaultValue: 'Drag to resize editor and results',
          })}
          onMouseDown={startEditorResize}
        >
          <div
            className={`h-1 w-16 rounded-full transition ${
              isResizingEditor
                ? 'bg-blue-7'
                : 'bg-gray-5 hover:bg-gray-6 th-dark:bg-gray-iron-8 th-dark:hover:bg-gray-iron-7'
            }`}
          />
        </div>

        {activeTab?.error && (
          <Alert color="error" className="mt-3 shrink-0 py-2">
            {databaseErrorLabel(activeTab.error, t)}
          </Alert>
        )}

        <QueryResult
          key={`${activeTab?.id || 'empty'}-${activeTab?.result?.Duration ?? 'none'}-${activeTab?.result?.Message || ''}-${activeTab?.result?.Rows?.[0]?.Value || ''}-${activeTab?.result?.Rows?.length || 0}`}
          result={activeTab?.result}
        />
      </div>

      {isHistoryOpen && (
        <QueryHistoryModal
          history={history}
          onClose={() => setIsHistoryOpen(false)}
        />
      )}
    </>
  );
}

// 表结构直接嵌在左侧库表树中；字段行只展示字段和类型，索引统一在底部区域查看。
function InlineTableStructure({
  details,
  isLoading,
  tableName,
}: {
  details?: DatabaseTableDetails;
  isLoading: boolean;
  tableName: string;
}) {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <div className="text-muted mb-2 ml-5 rounded border border-gray-4 bg-gray-1 px-2 py-2 text-xs th-dark:border-gray-iron-8 th-dark:bg-gray-iron-11">
        {t('legacyText.Loading table structure...', {
          defaultValue: 'Loading table structure...',
        })}
      </div>
    );
  }

  if (!details) {
    return null;
  }

  return (
    <div className="mb-2 ml-5 rounded border border-gray-4 bg-gray-1 px-2 py-2 text-xs text-gray-10 th-dark:border-gray-iron-8 th-dark:bg-gray-iron-11 th-dark:text-gray-3">
      <div
        className="truncate font-mono font-semibold"
        title={tableName}
        data-legacy-i18n-skip="true"
      >
        {tableName}
      </div>
      <div className="my-1 border-t border-gray-4 th-dark:border-gray-iron-8" />
      <div className="space-y-1" data-legacy-i18n-skip="true">
        {details.Columns.map((column) => {
          const flags = [
            column.Nullable ? '' : 'NOT NULL',
            column.Default ? `DEFAULT ${column.Default}` : '',
            column.Extra || '',
          ].filter(Boolean);
          const tooltip = [
            `${column.Name} ${column.Type}`,
            flags.join(' '),
            column.Comment || '',
          ]
            .filter(Boolean)
            .join('\n');

          return (
            <div
              key={column.Name}
              className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-2"
              title={tooltip}
            >
              <span className="truncate font-mono">{column.Name}</span>
              <span className="truncate font-mono text-gray-8 th-dark:text-gray-5">
                {column.Type}
              </span>
            </div>
          );
        })}
      </div>
      {details.Indexes.length > 0 && (
        <div className="mt-3 border-t border-gray-4 pt-2 th-dark:border-gray-iron-8">
          <div className="mb-1 font-semibold">
            {t('panelTitles.Indexes', { defaultValue: 'Indexes' })}
          </div>
          <div className="space-y-1" data-legacy-i18n-skip="true">
            {details.Indexes.map((index) => {
              const indexType = index.Primary
                ? 'PK'
                : index.Unique
                  ? 'UNIQUE'
                  : '';
              const tooltip = [index.Name, index.Columns.join(', '), indexType]
                .filter(Boolean)
                .join('\n');

              return (
                <div
                  key={index.Name}
                  className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_48px] gap-2"
                  title={tooltip}
                >
                  <span className="truncate font-mono">{index.Name}</span>
                  <span className="truncate font-mono text-gray-8 th-dark:text-gray-5">
                    {index.Columns.join(', ')}
                  </span>
                  <span className="text-right font-mono text-gray-7 th-dark:text-gray-6">
                    {indexType}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

// Redis Key 树放在库表树位置：按 `:` 分层展开，只展示 Key 结构不展示 value。
function RedisKeyTree({
  keys,
  isLoading,
  pattern,
  selectedKey,
  expandedFolders,
  onPatternChange,
  onRefresh,
  onSelectKey,
  onInsertKeyCommand,
  onToggleFolder,
}: {
  keys: RedisKeySummary[];
  isLoading: boolean;
  pattern: string;
  selectedKey: string;
  expandedFolders: Record<string, boolean>;
  onPatternChange: (pattern: string) => void;
  onRefresh: () => void;
  onSelectKey: (key: string) => void;
  onInsertKeyCommand: (key: RedisKeySummary) => void;
  onToggleFolder: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [patternDraft, setPatternDraft] = useState(
    displayRedisSearchPattern(pattern)
  );
  const tree = useMemo(() => buildRedisKeyTree(keys), [keys]);

  useEffect(() => {
    setPatternDraft(displayRedisSearchPattern(pattern));
  }, [pattern]);

  function submitPattern() {
    onPatternChange(patternDraft);
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="mb-1.5 flex items-center gap-2">
        <input
          className="form-control h-8 min-w-0 flex-1"
          value={patternDraft}
          onChange={(event) => setPatternDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              submitPattern();
            }
          }}
          placeholder={t('placeholders.Search Redis keys', {
            defaultValue: 'Search keys (fuzzy)',
          })}
          aria-label={t('placeholders.Search Redis keys', {
            defaultValue: 'Search keys (fuzzy)',
          })}
        />
        <Button
          type="button"
          color="default"
          size="small"
          onClick={submitPattern}
          data-cy="database-redis-search-button"
        >
          {t('buttons.Search', { defaultValue: 'Search' })}
        </Button>
        <Button
          type="button"
          color="default"
          size="small"
          onClick={onRefresh}
          data-cy="database-redis-refresh-button"
        >
          <Icon icon={RefreshCw} className="lucide" />
        </Button>
      </div>

      <div className="text-muted mb-2 px-0.5 text-[11px] leading-4">
        {t('legacyText.Redis tree shows at most {{count}} keys', {
          count: maxRedisTreeKeys,
          defaultValue: `Showing at most ${maxRedisTreeKeys} keys`,
        })}
      </div>

      <div className="min-h-0 flex-1 overflow-auto pr-1 text-sm [color-scheme:light] th-dark:[color-scheme:dark]">
        {isLoading && keys.length === 0 && (
          <div className="text-muted px-1.5 py-1">
            {t('common.Loading...', { defaultValue: 'Loading...' })}
          </div>
        )}
        {!isLoading && keys.length === 0 && (
          <div className="text-muted px-1.5 py-1">
            {t('legacyText.No keys found', { defaultValue: 'No keys found' })}
          </div>
        )}
        {tree.children.map((node) => (
          <RedisTreeNodeView
            key={node.path}
            node={node}
            depth={0}
            selectedKey={selectedKey}
            expandedFolders={expandedFolders}
            onToggleFolder={onToggleFolder}
            onSelectKey={onSelectKey}
            onInsertKeyCommand={onInsertKeyCommand}
          />
        ))}
      </div>
    </div>
  );
}

function RedisTreeNodeView({
  node,
  depth,
  selectedKey,
  expandedFolders,
  onToggleFolder,
  onSelectKey,
  onInsertKeyCommand,
}: {
  node: RedisTreeNode;
  depth: number;
  selectedKey: string;
  expandedFolders: Record<string, boolean>;
  onToggleFolder: (path: string) => void;
  onSelectKey: (key: string) => void;
  onInsertKeyCommand: (key: RedisKeySummary) => void;
}) {
  const hasChildren = node.children.length > 0;
  const expanded = expandedFolders[node.path] ?? false;
  const paddingLeft = 6 + depth * 14;

  if (hasChildren) {
    return (
      <div>
        <button
          type="button"
          className="flex w-full items-center gap-1 rounded border-0 bg-transparent py-1 text-left text-gray-10 hover:bg-gray-2 th-dark:text-gray-2 th-dark:hover:bg-gray-iron-10"
          style={{ paddingLeft }}
          onClick={() => onToggleFolder(node.path)}
        >
          <Icon
            icon={expanded ? ChevronDown : ChevronRight}
            className="lucide shrink-0"
          />
          <Icon icon={Folder} className="lucide shrink-0" />
          <span className="min-w-0 flex-1 truncate">{node.segment}</span>
          <span className="text-muted shrink-0 text-[11px]">
            ({node.keyCount})
          </span>
        </button>
        {node.key && (
          <RedisKeyLeafButton
            keySummary={node.key}
            depth={depth + 1}
            selectedKey={selectedKey}
            label={node.segment}
            onSelectKey={onSelectKey}
            onInsertKeyCommand={onInsertKeyCommand}
          />
        )}
        {expanded &&
          node.children.map((child) => (
            <RedisTreeNodeView
              key={child.path}
              node={child}
              depth={depth + 1}
              selectedKey={selectedKey}
              expandedFolders={expandedFolders}
              onToggleFolder={onToggleFolder}
              onSelectKey={onSelectKey}
              onInsertKeyCommand={onInsertKeyCommand}
            />
          ))}
      </div>
    );
  }

  if (!node.key) {
    return null;
  }

  return (
    <RedisKeyLeafButton
      keySummary={node.key}
      depth={depth}
      selectedKey={selectedKey}
      label={node.segment}
      onSelectKey={onSelectKey}
      onInsertKeyCommand={onInsertKeyCommand}
    />
  );
}

function RedisKeyLeafButton({
  keySummary,
  depth,
  selectedKey,
  label,
  onSelectKey,
  onInsertKeyCommand,
}: {
  keySummary: RedisKeySummary;
  depth: number;
  selectedKey: string;
  label?: string;
  onSelectKey: (key: string) => void;
  onInsertKeyCommand: (key: RedisKeySummary) => void;
}) {
  const isSelected = selectedKey === keySummary.Name;
  const paddingLeft = 6 + depth * 14;

  return (
    <button
      type="button"
      className={`flex w-full items-center gap-1.5 rounded border-0 py-1 text-left text-xs transition-colors ${
        isSelected
          ? 'bg-blue-2 text-blue-9 th-dark:bg-gray-iron-10 th-dark:text-gray-1'
          : 'bg-transparent text-gray-9 hover:bg-gray-2 hover:text-gray-10 th-dark:text-gray-4 th-dark:hover:bg-gray-iron-10 th-dark:hover:text-gray-1'
      }`}
      style={{ paddingLeft }}
      title={keySummary.Name}
      onClick={() => onSelectKey(keySummary.Name)}
      onDoubleClick={() => onInsertKeyCommand(keySummary)}
    >
      <Icon icon={KeyRound} className="lucide shrink-0" />
      <span className="min-w-0 flex-1 truncate font-mono">
        {label || keySummary.Name}
      </span>
      <span className="label label-default shrink-0">{keySummary.Type}</span>
    </button>
  );
}

function ConnectionForm({
  values,
  isEditing,
  hasSavedPassword,
  isLoading,
  isTesting,
  containers,
  isContainerTargetVisible,
  preferPublishedContainerPorts,
  onCancel,
  onTest,
  onSave,
  onChange,
}: {
  values: ConnectionFormValues;
  isEditing: boolean;
  hasSavedPassword: boolean;
  isLoading: boolean;
  isTesting: boolean;
  containers: ContainerListViewModel[];
  isContainerTargetVisible: boolean;
  preferPublishedContainerPorts: boolean;
  onCancel: () => void;
  onTest: (values: ConnectionFormValues) => Promise<void>;
  onSave: (event: FormEvent) => void;
  onChange: (values: ConnectionFormValues) => void;
}) {
  const { t } = useTranslation();
  const selectedContainer = containers.find(
    (container) => container.Id === values.ContainerId
  );
  const showContainerIpWarning = !!values.ContainerId && !selectedContainer?.IP;
  const [testStatus, setTestStatus] = useState<'success' | 'error'>();
  const [testError, setTestError] = useState<DatabaseViewError>();

  function updateValue<T extends keyof ConnectionFormValues>(
    key: T,
    value: ConnectionFormValues[T]
  ) {
    onChange({ ...values, [key]: value });
  }

  function updateType(type: DatabaseConnectionType) {
    if (values.ContainerId) {
      onChange(
        applyContainerSuggestion(
          {
            ...values,
            Type: type,
            Database: type === 'redis' ? '' : values.Database,
          },
          selectedContainer,
          type,
          preferPublishedContainerPorts
        )
      );
      return;
    }

    onChange({
      ...values,
      Type: type,
      Port: portByType[type],
      Database: type === 'redis' ? '' : values.Database,
    });
  }

  function updateTarget(target: 'custom' | 'container') {
    if (target === 'custom') {
      onChange({ ...values, ContainerId: '' });
      return;
    }

    const container = containers[0];
    onChange(
      applyContainerSuggestion(
        values,
        container,
        values.Type,
        preferPublishedContainerPorts
      )
    );
  }

  function updateContainer(containerId: string) {
    const container = containers.find(
      (container) => container.Id === containerId
    );
    onChange(
      applyContainerSuggestion(
        { ...values, ContainerId: containerId },
        container,
        values.Type,
        preferPublishedContainerPorts
      )
    );
  }

  return (
    <form onSubmit={onSave} className="flex min-h-0 flex-1 flex-col">
      <div className="mb-4 flex items-center gap-2">
        <Icon icon={isEditing ? Pencil : Plus} className="lucide" />
        <span className="text-lg font-semibold">
          {isEditing
            ? t('panelTitles.Edit connection', {
                defaultValue: 'Edit connection',
              })
            : t('panelTitles.Add connection', {
                defaultValue: 'Add connection',
              })}
        </span>
        <Button
          type="button"
          color="default"
          size="small"
          className="ml-auto"
          onClick={onCancel}
          data-cy="database-close-form-button"
        >
          <Icon icon={X} className="lucide" />
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-auto pr-1">
        {isContainerTargetVisible && (
          <>
            <FormControl
              label={t('legacyText.Target', { defaultValue: 'Target' })}
              inputId="database-connection-target"
            >
              <select
                id="database-connection-target"
                className="form-control"
                value={values.ContainerId ? 'container' : 'custom'}
                onChange={(event) =>
                  updateTarget(event.target.value as 'custom' | 'container')
                }
              >
                <option value="container" disabled={containers.length === 0}>
                  {t('legacyText.Container', { defaultValue: 'Container' })}
                </option>
                <option value="custom">
                  {t('legacyText.Custom address', {
                    defaultValue: 'Custom address',
                  })}
                </option>
              </select>
            </FormControl>

            {values.ContainerId !== undefined && values.ContainerId !== '' && (
              <FormControl
                label={t('legacyText.Container', {
                  defaultValue: 'Container',
                })}
                inputId="database-connection-container"
              >
                <select
                  id="database-connection-container"
                  className="form-control"
                  value={values.ContainerId}
                  onChange={(event) => updateContainer(event.target.value)}
                  required
                >
                  {containers.map((container) => (
                    <option key={container.Id} value={container.Id}>
                      {container.Names?.[0]?.replace(/^\//, '') || container.Id}
                    </option>
                  ))}
                </select>
              </FormControl>
            )}
          </>
        )}

        <FormControl
          label={t('legacyText.Name', { defaultValue: 'Name' })}
          inputId="database-connection-name"
        >
          <input
            id="database-connection-name"
            className="form-control"
            value={values.Name}
            onChange={(event) => updateValue('Name', event.target.value)}
            required
          />
        </FormControl>

        <FormControl
          label={t('legacyText.Type', { defaultValue: 'Type' })}
          inputId="database-connection-type"
        >
          <select
            id="database-connection-type"
            className="form-control"
            value={values.Type}
            onChange={(event) =>
              updateType(event.target.value as DatabaseConnectionType)
            }
          >
            <option value="mysql">MySQL</option>
            <option value="mariadb">MariaDB</option>
            <option value="postgres">PostgreSQL</option>
            <option value="redis">Redis</option>
          </select>
        </FormControl>

        <FormControl
          label={t('legacyText.Host', { defaultValue: 'Host' })}
          inputId="database-connection-host"
        >
          <input
            id="database-connection-host"
            className="form-control"
            value={values.Host}
            onChange={(event) => updateValue('Host', event.target.value)}
            required
          />
        </FormControl>

        {showContainerIpWarning && (
          <div className="small text-warning mb-3">
            {t(
              'legacyText.Container IP is unavailable, using localhost fallback.',
              {
                defaultValue:
                  'Container IP is unavailable, using localhost fallback.',
              }
            )}
          </div>
        )}

        <FormControl
          label={t('legacyText.Port', { defaultValue: 'Port' })}
          inputId="database-connection-port"
        >
          <input
            id="database-connection-port"
            className="form-control"
            type="number"
            min={1}
            max={65535}
            value={values.Port}
            onChange={(event) =>
              updateValue('Port', Number(event.target.value))
            }
            required
          />
        </FormControl>

        {values.Type !== 'redis' && (
          <FormControl
            label={t('legacyText.Database', { defaultValue: 'Database' })}
            inputId="database-connection-database"
          >
            <input
              id="database-connection-database"
              className="form-control"
              value={values.Database}
              onChange={(event) => updateValue('Database', event.target.value)}
            />
          </FormControl>
        )}

        <FormControl
          label={t('legacyText.Username', { defaultValue: 'Username' })}
          inputId="database-connection-username"
        >
          <input
            id="database-connection-username"
            className="form-control"
            value={values.Username}
            onChange={(event) => updateValue('Username', event.target.value)}
          />
        </FormControl>

        <FormControl
          label={t('legacyText.Password', { defaultValue: 'Password' })}
          inputId="database-connection-password"
        >
          <input
            id="database-connection-password"
            className="form-control"
            type="password"
            value={values.Password}
            onChange={(event) => updateValue('Password', event.target.value)}
            placeholder={
              isEditing
                ? t('placeholders.Leave blank to keep current password', {
                    defaultValue: 'Leave blank to keep current password',
                  })
                : ''
            }
          />
        </FormControl>

        {isEditing && (
          <div
            className={`small mb-3 ${
              hasSavedPassword ? 'text-success' : 'text-warning'
            }`}
          >
            {hasSavedPassword
              ? t('legacyText.Saved password exists. Leave blank to keep it.', {
                  defaultValue:
                    'Saved password exists. Leave blank to keep it.',
                })
              : t('legacyText.No password is saved for this connection.', {
                  defaultValue: 'No password is saved for this connection.',
                })}
          </div>
        )}

        <FormControl
          label={t('legacyText.Timeout', { defaultValue: 'Timeout' })}
          inputId="database-connection-timeout"
        >
          <input
            id="database-connection-timeout"
            className="form-control"
            type="number"
            min={5}
            max={120}
            value={values.QueryTimeout}
            onChange={(event) =>
              updateValue('QueryTimeout', Number(event.target.value))
            }
            required
          />
        </FormControl>
      </div>

      {testStatus && (
        <Alert
          color={testStatus === 'success' ? 'success' : 'error'}
          className="mt-4 py-2"
        >
          {testStatus === 'success'
            ? t('legacyText.Connection successful', {
                defaultValue: 'Connection successful',
              })
            : databaseErrorLabel(
                testError ||
                  Object.assign(new Error('Connection failed'), {
                    ErrorCode: 'connection_failed',
                  }),
                t
              )}
        </Alert>
      )}

      <div className="mt-4 flex flex-col gap-2 border-0 border-t border-solid border-gray-7 pt-4 th-dark:border-gray-8 sm:flex-row sm:items-center sm:justify-between">
        <LoadingButton
          type="button"
          color="default"
          className="w-full justify-center sm:w-auto"
          isLoading={isTesting}
          loadingText={t('buttons.Testing...', { defaultValue: 'Testing...' })}
          data-cy="database-test-connection-button"
          onClick={async () => {
            setTestStatus(undefined);
            setTestError(undefined);
            try {
              await onTest(values);
              setTestStatus('success');
            } catch (error) {
              setTestError(error as DatabaseViewError);
              setTestStatus('error');
            }
          }}
        >
          <Icon icon={FlaskConical} className="lucide space-right" />
          {t('buttons.Test Connection', { defaultValue: 'Test Connection' })}
        </LoadingButton>
        <div className="flex gap-2 sm:ml-auto">
          <LoadingButton
            type="submit"
            color="primary"
            className="flex-1 justify-center sm:flex-none"
            isLoading={isLoading}
            loadingText={t('buttons.Saving...', { defaultValue: 'Saving...' })}
            data-cy="database-save-connection-button"
          >
            <Icon icon={Save} className="lucide space-right" />
            {t('buttons.Save', { defaultValue: 'Save' })}
          </LoadingButton>
          <Button
            type="button"
            color="default"
            className="flex-1 justify-center sm:flex-none"
            data-cy="database-cancel-edit-button"
            onClick={onCancel}
          >
            {t('buttons.Cancel', { defaultValue: 'Cancel' })}
          </Button>
        </div>
      </div>
    </form>
  );
}

function QueryHistoryModal({
  history,
  onClose,
}: {
  history: QueryHistoryItem[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [pageSize, setPageSize] = useState(20);
  const [page, setPage] = useState(1);
  const [copiedHistoryId, setCopiedHistoryId] = useState('');
  const filteredHistory = useMemo(
    () =>
      history.filter((item) =>
        item.query.toLowerCase().includes(search.trim().toLowerCase())
      ),
    [history, search]
  );
  const totalPages = Math.max(1, Math.ceil(filteredHistory.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageItems = filteredHistory.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize
  );

  useEffect(() => {
    setPage(1);
  }, [search, pageSize]);

  async function copyHistoryItem(item: QueryHistoryItem) {
    await navigator.clipboard?.writeText(item.query);
    setCopiedHistoryId(item.id);
    window.setTimeout(() => setCopiedHistoryId(''), 1500);
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div className="flex max-h-[80vh] w-[820px] max-w-full flex-col rounded border border-gray-5 bg-white text-gray-10 shadow-2xl th-dark:bg-gray-10 th-dark:text-white">
        <div className="flex items-center gap-2 border-b border-gray-5 px-4 py-3">
          <Icon icon={History} className="lucide" />
          <span className="font-semibold">
            {t('panelTitles.Execution history', {
              defaultValue: 'Execution history',
            })}
          </span>
          <Button
            type="button"
            color="default"
            size="small"
            className="ml-auto"
            onClick={onClose}
            data-cy="database-history-close-button"
          >
            <Icon icon={X} className="lucide" />
          </Button>
        </div>
        <div className="flex items-center gap-3 border-b border-gray-5 px-4 py-3">
          <input
            className="form-control max-w-[420px]"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('placeholders.Search SQL', {
              defaultValue: 'Search SQL',
            })}
          />
          <select
            className="form-control ml-auto w-[90px]"
            value={pageSize}
            onChange={(event) => setPageSize(Number(event.target.value))}
          >
            {[20, 50, 100].map((size) => (
              <option key={size} value={size}>
                {size}
              </option>
            ))}
          </select>
        </div>
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="table-hover mb-0 table">
            <thead className="sticky top-0 z-10 bg-gray-2 text-gray-10 th-dark:bg-gray-iron-10 th-dark:text-white">
              <tr>
                <th>
                  {t('tableHeaders.Execution time', {
                    defaultValue: 'Execution time',
                  })}
                </th>
                <th>SQL</th>
                <th className="w-16 text-right">
                  {t('tableHeaders.Actions', { defaultValue: 'Actions' })}
                </th>
              </tr>
            </thead>
            <tbody>
              {pageItems.length === 0 && (
                <tr>
                  <td colSpan={3}>
                    {t('legacyText.No execution history', {
                      defaultValue: 'No execution history',
                    })}
                  </td>
                </tr>
              )}
              {pageItems.map((item) => (
                <tr key={item.id}>
                  <td className="whitespace-nowrap">
                    {new Date(item.executedAt).toLocaleString()}
                  </td>
                  <td>
                    <pre className="m-0 whitespace-pre-wrap break-words bg-transparent p-0 font-mono text-xs">
                      {item.query}
                    </pre>
                  </td>
                  <td className="text-right">
                    <Button
                      type="button"
                      color="default"
                      size="small"
                      title={t('buttons.Copy', { defaultValue: 'Copy' })}
                      onClick={() => copyHistoryItem(item)}
                      data-cy="database-history-copy-button"
                    >
                      <Icon
                        icon={copiedHistoryId === item.id ? Check : Copy}
                        className="lucide"
                      />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-gray-5 px-4 py-3">
          <span className="text-muted text-xs">
            {currentPage} / {totalPages}
          </span>
          <Button
            type="button"
            color="default"
            size="small"
            disabled={currentPage <= 1}
            onClick={() => setPage((page) => Math.max(1, page - 1))}
            data-cy="database-history-previous-button"
          >
            {t('buttons.Previous', { defaultValue: 'Previous' })}
          </Button>
          <Button
            type="button"
            color="default"
            size="small"
            disabled={currentPage >= totalPages}
            onClick={() => setPage((page) => Math.min(totalPages, page + 1))}
            data-cy="database-history-next-button"
          >
            {t('buttons.Next', { defaultValue: 'Next' })}
          </Button>
        </div>
      </div>
    </div>
  );
}

function QueryResult({ result }: { result?: DatabaseQueryResult }) {
  const { t } = useTranslation();
  const [detail, setDetail] = useState<{
    column: string;
    value: string;
  }>();
  const [copied, setCopied] = useState(false);
  const [copiedMarkdown, setCopiedMarkdown] = useState(false);
  const formattedDetailJson = useMemo(
    () => formatJsonText(detail?.value),
    [detail?.value]
  );

  useEffect(() => {
    setCopied(false);
  }, [detail?.column, detail?.value]);

  async function copyDetail() {
    if (!detail) {
      return;
    }

    await navigator.clipboard?.writeText(detail.value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }

  function formatDetailJson() {
    if (!detail || !formattedDetailJson) {
      return;
    }

    setDetail({
      ...detail,
      value: formattedDetailJson,
    });
  }

  async function copyMarkdown() {
    if (!result) {
      return;
    }

    await navigator.clipboard?.writeText(markdownTable(result));
    setCopiedMarkdown(true);
    window.setTimeout(() => setCopiedMarkdown(false), 1500);
  }

  function exportResult(format: 'csv' | 'json' | 'markdown') {
    if (!result) {
      return;
    }

    const content =
      format === 'csv'
        ? csvTable(result)
        : format === 'json'
          ? jsonTable(result)
          : markdownTable(result);
    const mime =
      format === 'json'
        ? 'application/json;charset=utf-8'
        : 'text/plain;charset=utf-8';
    const extension = format === 'markdown' ? 'md' : format;

    saveAs(
      new Blob([content], { type: mime }),
      `database-result-${formatTimestampForFilename(new Date())}.${extension}`
    );
  }

  if (!result) {
    return (
      <div className="text-muted mt-4 min-h-0 flex-1 rounded border border-gray-5 p-4">
        {t('legacyText.Run a query to see results.', {
          defaultValue: 'Run a query to see results.',
        })}
      </div>
    );
  }

  return (
    <div className="mt-4 flex min-h-0 flex-1 flex-col overflow-hidden rounded border border-gray-5">
      <div className="flex items-center gap-2 border-b border-gray-5 px-3 py-2">
        <span className="font-semibold">
          {t('panelTitles.Results', { defaultValue: 'Results' })}
        </span>
        <span className="text-muted">{result.Message}</span>
        {result.RedisKey && (
          <span
            className="max-w-[240px] truncate font-mono text-xs"
            title={result.RedisKey}
          >
            {result.RedisKey}
          </span>
        )}
        {result.RedisType && (
          <span className="label label-default shrink-0">
            {result.RedisType}
          </span>
        )}
        <span className="ml-auto rounded bg-gray-2 px-2 py-1 text-xs font-medium text-gray-8 th-dark:bg-gray-iron-10 th-dark:text-gray-3">
          {result.Duration.toFixed(3)}s
        </span>
        <Button
          type="button"
          color="default"
          size="small"
          onClick={() => exportResult('csv')}
          data-cy="database-export-csv-button"
        >
          <Icon icon={FileSpreadsheet} className="lucide space-right" />
          CSV
        </Button>
        <Button
          type="button"
          color="default"
          size="small"
          onClick={() => exportResult('json')}
          data-cy="database-export-json-button"
        >
          <Icon icon={FileJson} className="lucide space-right" />
          JSON
        </Button>
        <Button
          type="button"
          color="default"
          size="small"
          onClick={() => exportResult('markdown')}
          data-cy="database-export-markdown-button"
        >
          <Icon icon={Download} className="lucide space-right" />
          Markdown
        </Button>
        <Button
          type="button"
          color="default"
          size="small"
          onClick={copyMarkdown}
          data-cy="database-copy-markdown-button"
        >
          <Icon
            icon={copiedMarkdown ? Check : Copy}
            className="lucide space-right"
          />
          {copiedMarkdown
            ? t('buttons.Copied', { defaultValue: 'Copied' })
            : t('buttons.Copy as Markdown table', {
                defaultValue: 'Copy as Markdown table',
              })}
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="table-hover nowrap-cells mb-0 table min-w-max">
          <thead
            className="sticky top-0 z-10 bg-gray-2 text-gray-10 th-dark:bg-blue-11 th-dark:text-white"
            data-legacy-i18n-skip="true"
          >
            <tr>
              {result.Columns.map((column) => (
                <th key={column} className="min-w-[140px]">
                  {resultColumnLabel(column, t)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody data-legacy-i18n-skip="true">
            {result.Rows.length === 0 && (
              <tr>
                <td colSpan={Math.max(result.Columns.length, 1)}>
                  {t('legacyText.No rows', { defaultValue: 'No rows' })}
                </td>
              </tr>
            )}
            {result.Rows.map((row, rowIndex) => (
              <tr key={rowIndex}>
                {result.Columns.map((column) => (
                  <td key={column} className="max-w-[260px]">
                    <div className="group flex items-center gap-2">
                      <span
                        className="min-w-0 flex-1 truncate"
                        title={row[column]}
                      >
                        {row[column]}
                      </span>
                      <button
                        type="button"
                        className="ml-auto shrink-0 rounded border-0 bg-transparent p-0 opacity-0 transition-opacity group-hover:opacity-100"
                        title={t('buttonTitles.View details', {
                          defaultValue: 'View details',
                        })}
                        onClick={() =>
                          setDetail({
                            column,
                            value: row[column],
                          })
                        }
                      >
                        <Icon icon={Eye} className="lucide" />
                      </button>
                    </div>
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {detail && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="flex max-h-[80vh] w-[640px] max-w-full flex-col rounded border border-gray-5 bg-gray-10 shadow-2xl">
            <div className="flex items-center gap-2 border-b border-gray-5 px-4 py-3">
              <span className="font-semibold">
                {t('panelTitles.Cell details', {
                  defaultValue: 'Cell details',
                })}
              </span>
              <span
                className="text-muted truncate text-sm"
                data-legacy-i18n-skip="true"
              >
                {detail.column}
              </span>
            </div>
            <pre
              className="m-0 min-h-[180px] overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-sm"
              data-legacy-i18n-skip="true"
            >
              {detail.value}
            </pre>
            <div className="flex justify-end gap-2 border-t border-gray-5 px-4 py-3">
              <Button
                type="button"
                color="default"
                disabled={!formattedDetailJson}
                title={
                  formattedDetailJson
                    ? t('buttons.Format', { defaultValue: 'Format' })
                    : t('legacyText.Cell value is not valid JSON', {
                        defaultValue: 'Cell value is not valid JSON',
                      })
                }
                data-cy="database-cell-detail-format-json-button"
                onClick={formatDetailJson}
              >
                <Icon icon={Wand2} className="lucide space-right" />
                {t('buttons.Format', { defaultValue: 'Format' })}
              </Button>
              <Button
                type="button"
                color="default"
                data-cy="database-cell-detail-copy-button"
                onClick={copyDetail}
              >
                <Icon
                  icon={copied ? Check : Copy}
                  className="lucide space-right"
                />
                {copied
                  ? t('buttons.Copied', { defaultValue: 'Copied' })
                  : t('buttons.Copy', { defaultValue: 'Copy' })}
              </Button>
              <Button
                type="button"
                color="default"
                data-cy="database-cell-detail-close-button"
                onClick={() => setDetail(undefined)}
              >
                <Icon icon={X} className="lucide space-right" />
                {t('buttons.Close', { defaultValue: 'Close' })}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function normalizePayload(
  values: ConnectionFormValues,
  options: {
    connectionId?: number;
    preserveBlankPassword?: boolean;
  } = {}
): DatabaseConnectionPayload {
  const { Password, ...valuesWithoutPassword } = values;
  const password = Password?.trim();
  // 编辑连接时空密码表示保留当前密码；这里省略 Password 字段，
  // 测试连接再带上 Id，让后端能安全复用已保存的密码。
  const passwordPayload =
    options.preserveBlankPassword && !password ? {} : { Password: password };

  return {
    ...valuesWithoutPassword,
    ...(options.connectionId ? { Id: options.connectionId } : {}),
    Name: values.Name.trim(),
    Host: values.Host.trim(),
    Database: values.Database?.trim(),
    Username: values.Username?.trim(),
    ...passwordPayload,
    ContainerId: values.ContainerId || '',
    QueryTimeout: Math.min(120, Math.max(5, Number(values.QueryTimeout) || 30)),
  };
}

// 数据库下拉默认优先选择业务库，跳过 MySQL/PostgreSQL 自带系统库；
// 这样刷新页面后不会误把 information_schema 作为当前查询上下文。
function preferredDatabaseOption(
  databases: string[],
  connectionType?: DatabaseConnectionType
) {
  if (!databases.length) {
    return '';
  }

  return (
    databases.find((database) => !isSystemDatabase(database, connectionType)) ||
    databases[0]
  );
}

function isSystemDatabase(
  database: string,
  connectionType?: DatabaseConnectionType
) {
  const normalized = database.toLowerCase();
  const mysqlSystemDatabases = new Set([
    'information_schema',
    'mysql',
    'performance_schema',
    'sys',
  ]);
  const postgresSystemDatabases = new Set(['information_schema', 'pg_catalog']);

  if (connectionType === 'postgres') {
    return (
      normalized.startsWith('pg_') || postgresSystemDatabases.has(normalized)
    );
  }

  return mysqlSystemDatabases.has(normalized);
}

function applyContainerSuggestion(
  values: ConnectionFormValues,
  container: ContainerListViewModel | undefined,
  type: DatabaseConnectionType,
  preferPublishedPorts: boolean
): ConnectionFormValues {
  const containerName = container?.Names?.[0]?.replace(/^\//, '') || '';
  const suggestion = suggestedConnectionTarget(
    container,
    type,
    preferPublishedPorts
  );

  return {
    ...values,
    ContainerId: container?.Id || values.ContainerId || '',
    Name: containerName || values.Name,
    Host: suggestion.host,
    Port: suggestion.port,
  };
}

function suggestedConnectionTarget(
  container: ContainerListViewModel | undefined,
  type: DatabaseConnectionType,
  preferPublishedPorts: boolean
) {
  const defaultPort = portByType[type];
  const publishedPort = publishedPortForType(container, type);

  if (preferPublishedPorts && publishedPort) {
    return {
      host: normalizePublishedHost(publishedPort.host),
      port: publishedPort.public,
    };
  }

  return {
    host: container?.IP || '127.0.0.1',
    port:
      container?.ExposedPorts?.find((port) => port.private === defaultPort)
        ?.private ||
      container?.Ports?.find((port) => port.private === defaultPort)?.private ||
      container?.ExposedPorts?.[0]?.private ||
      defaultPort,
  };
}

function publishedPortForType(
  container: ContainerListViewModel | undefined,
  type: DatabaseConnectionType
) {
  const defaultPort = portByType[type];
  return (
    container?.Ports?.find((port) => port.private === defaultPort) ||
    container?.Ports?.[0]
  );
}

function normalizePublishedHost(host?: string) {
  if (!host || host === '0.0.0.0' || host === '::' || host === '[::]') {
    return '127.0.0.1';
  }

  return host;
}

function isLocalAgentUrl(url = '') {
  const normalized = url.replace(/^[a-z]+:\/\//i, '').toLowerCase();
  return (
    normalized.startsWith('localhost:') ||
    normalized.startsWith('127.0.0.1:') ||
    normalized.startsWith('[::1]:') ||
    normalized.startsWith('host.docker.internal:')
  );
}

function tableSelectStatement(
  type: DatabaseConnectionType,
  _database: string,
  table: string
) {
  if (type === 'postgres') {
    return `SELECT * FROM "${table.replaceAll('"', '""')}" LIMIT 100;`;
  }

  return `SELECT * FROM \`${table.replaceAll('`', '``')}\` LIMIT 100;`;
}

function appendSqlStatement(current: string, statement: string) {
  const trimmed = current.trimEnd();
  if (!trimmed) {
    return statement;
  }

  return `${trimmed}\n${statement}`;
}

function prepareQueryForExecution(query: string) {
  const statement = stripSqlCommentLines(query).trim();
  if (!statement) {
    return '';
  }

  if (!isSelectStatement(statement)) {
    return statement;
  }

  return capSelectLimit(statement, maxSelectRows);
}

function stripSqlCommentLines(query: string) {
  return query
    .split('\n')
    .filter((line) => !line.trimStart().startsWith('--'))
    .join('\n');
}

function isSelectStatement(query: string) {
  return statementType(query) === 'select';
}

function isUpdateOrDeleteStatement(query: string) {
  const type = statementType(query);
  return type === 'update' || type === 'delete';
}

// 写操作缺少 WHERE 时先在前端提高确认等级，后端仍会做同样校验。
function isUnsafeWriteStatement(query: string) {
  return (
    isUpdateOrDeleteStatement(query) &&
    !containsSqlKeywordOutsideLiterals(query, 'where')
  );
}

function containsSqlKeywordOutsideLiterals(query: string, keyword: string) {
  return new RegExp(`\\b${keyword}\\b`, 'i').test(
    stripSqlLiteralsAndComments(query)
  );
}

function stripSqlLiteralsAndComments(query: string) {
  let result = '';
  let quote: "'" | '"' | '`' | undefined;
  let lineComment = false;
  let blockComment = false;

  for (let index = 0; index < query.length; index += 1) {
    const char = query[index];
    const next = query[index + 1];

    if (lineComment) {
      if (char === '\n') {
        lineComment = false;
        result += char;
      }
      continue;
    }

    if (blockComment) {
      if (char === '*' && next === '/') {
        blockComment = false;
        index += 1;
      }
      continue;
    }

    if (quote) {
      result += ' ';
      if (char === '\\') {
        index += 1;
        continue;
      }
      if (char === quote) {
        if (next === quote) {
          index += 1;
          continue;
        }
        quote = undefined;
      }
      continue;
    }

    if (char === '-' && next === '-') {
      lineComment = true;
      index += 1;
      continue;
    }

    if (char === '/' && next === '*') {
      blockComment = true;
      index += 1;
      continue;
    }

    if (char === "'" || char === '"' || char === '`') {
      quote = char;
      result += ' ';
      continue;
    }

    result += char;
  }

  return result;
}

function statementType(query: string) {
  return stripSqlCommentLines(query)
    .trimStart()
    .split(/\s+/, 1)[0]
    ?.toLowerCase();
}

function capSelectLimit(query: string, maxRows: number) {
  const semicolon = query.trimEnd().endsWith(';') ? ';' : '';
  const withoutSemicolon = semicolon
    ? query.trimEnd().slice(0, -1)
    : query.trimEnd();
  const limitPattern = /\blimit\s+(\d+)(\s*,\s*\d+)?\s*$/i;
  const match = withoutSemicolon.match(limitPattern);

  if (!match) {
    return `${withoutSemicolon} LIMIT ${maxRows}${semicolon}`;
  }

  if (match[2]) {
    const offsetLimit = Number(match[2].replace(/[,\s]/g, ''));
    if (offsetLimit <= maxRows) {
      return query;
    }

    return `${withoutSemicolon.replace(limitPattern, `LIMIT ${match[1]}, ${maxRows}`)}${semicolon}`;
  }

  const limit = Number(match[1]);
  if (limit <= maxRows) {
    return query;
  }

  return `${withoutSemicolon.replace(limitPattern, `LIMIT ${maxRows}`)}${semicolon}`;
}

function loadQueryTabs(
  storageKey: string,
  connection: DatabaseConnection,
  databases: string[],
  t: TranslateFn
): PersistedQueryTabs {
  const fallback = createDefaultQueryTab(connection, databases, 1, t);

  try {
    const stored = window.localStorage.getItem(storageKey);
    if (!stored) {
      return { activeTabId: fallback.id, tabs: [fallback] };
    }

    const parsed = JSON.parse(stored) as PersistedQueryTabs;
    const tabs = sanitizeQueryTabs(parsed.tabs, connection, databases, t);
    const activeTabId = tabs.some((tab) => tab.id === parsed.activeTabId)
      ? parsed.activeTabId
      : tabs[0].id;

    return { activeTabId, tabs };
  } catch {
    return { activeTabId: fallback.id, tabs: [fallback] };
  }
}

function saveQueryTabs(
  storageKey: string,
  activeTabId: string,
  tabs: QueryTab[]
) {
  const payload: PersistedQueryTabs = {
    activeTabId,
    tabs: tabs.map(({ id, name, query, database }) => ({
      id,
      name,
      query,
      database,
    })),
  };

  window.localStorage.setItem(storageKey, JSON.stringify(payload));
}

function sanitizeQueryTabs(
  tabs: QueryTab[] | undefined,
  connection: DatabaseConnection,
  databases: string[],
  t: TranslateFn
) {
  const validTabs = (tabs || [])
    .filter((tab) => tab.id && tab.name)
    .map((tab, index) => ({
      id: tab.id,
      name: tab.name || queryTabName(index + 1, t),
      query: defaultQueryDraft(connection, tab.query),
      database:
        connection.Type === 'redis'
          ? normalizeRedisDatabase(tab.database || connection.Database || '0')
          : tab.database ||
            connection.Database ||
            preferredDatabaseOption(databases, connection.Type),
    }));

  return validTabs.length
    ? validTabs
    : [createDefaultQueryTab(connection, databases, 1, t)];
}

// 新建查询标签只持久化草稿和当前库，避免把结果数据写入浏览器存储。
function createDefaultQueryTab(
  connection: DatabaseConnection,
  databases: string[],
  index: number,
  t: TranslateFn
): QueryTab {
  return {
    id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
    name: queryTabName(index, t),
    query: defaultQueryDraft(connection),
    database:
      connection.Type === 'redis'
        ? normalizeRedisDatabase(connection.Database)
        : connection.Database ||
          preferredDatabaseOption(databases, connection.Type),
  };
}

// Redis 编辑区不应残留 SQL 行数提示；历史草稿若仍是该注释则清空。
function defaultQueryDraft(connection: DatabaseConnection, query?: string) {
  if (connection.Type === 'redis') {
    const trimmed = (query || '').trim();
    if (
      !trimmed ||
      trimmed === defaultSqlComment ||
      trimmed.startsWith('-- SQL')
    ) {
      return '';
    }
    return query || '';
  }

  return query || defaultSqlTemplate;
}

// Redis DB 只能是数字 index；切库瞬间残留的 MySQL 库名直接回落为 0。
function normalizeRedisDatabase(database?: string) {
  const trimmed = (database || '').trim();
  if (/^\d+$/.test(trimmed)) {
    return trimmed;
  }
  return '0';
}

// 搜索框输入普通关键字时自动包成 *keyword*，实现模糊匹配；已含通配符则原样提交。
function normalizeRedisSearchPattern(input: string) {
  const trimmed = input.trim();
  if (!trimmed) {
    return '*';
  }
  if (/[*?\[]/.test(trimmed)) {
    return trimmed;
  }
  return `*${trimmed}*`;
}

function displayRedisSearchPattern(pattern: string) {
  if (pattern === '*') {
    return '';
  }
  const fuzzyMatch = pattern.match(/^\*([^*?[\]]+)\*$/);
  if (fuzzyMatch) {
    return fuzzyMatch[1];
  }
  return pattern;
}

function resultColumnLabel(column: string, t: TranslateFn) {
  if (column === 'TTL') {
    return t('tableHeaders.TTL', { defaultValue: 'TTL' });
  }
  if (column === 'Key') {
    return t('tableHeaders.Key', { defaultValue: 'Key' });
  }
  if (column === 'Type') {
    return t('tableHeaders.Type', { defaultValue: 'Type' });
  }
  if (column === 'Value') {
    return t('tableHeaders.Value', { defaultValue: 'Value' });
  }
  if (column === 'Index') {
    return t('tableHeaders.Index', { defaultValue: 'Index' });
  }
  if (column === 'Field') {
    return t('tableHeaders.Field', { defaultValue: 'Field' });
  }

  return column;
}

// 把 Redis Key 详情转成结果：Key/Type 放表头元数据，list/set 行内用 Index。
function redisDetailsToQueryResult(
  details: RedisKeyDetails
): DatabaseQueryResult {
  const meta = {
    RedisKey: details.Name,
    RedisType: details.Type,
  };

  if (typeof details.Value === 'string') {
    return {
      Columns: ['Value', 'TTL'],
      Rows: [
        {
          Value: details.Value,
          TTL: details.TTL,
        },
      ],
      Message: '1 row(s)',
      Duration: 0,
      ...meta,
    };
  }

  if (details.Rows.length === 0) {
    return {
      Columns: ['Value', 'TTL'],
      Rows: [
        {
          Value: '',
          TTL: details.TTL,
        },
      ],
      Message: '1 row(s)',
      Duration: 0,
      ...meta,
    };
  }

  if (details.Type === 'hash') {
    return {
      Columns: ['Field', 'Value', 'TTL'],
      Rows: details.Rows.map((row) => ({
        Field: row.Field || '',
        Value: row.Value ?? '',
        TTL: details.TTL,
      })),
      Message: `${details.Rows.length} row(s)`,
      Duration: 0,
      ...meta,
    };
  }

  if (
    details.Type === 'list' ||
    details.Type === 'set' ||
    details.Type === 'zset'
  ) {
    return {
      Columns: ['Index', 'Value', 'TTL'],
      Rows: details.Rows.map((row, index) => ({
        Index: row.Index ?? String(index),
        Value: redisDetailRowValue(row),
        TTL: details.TTL,
      })),
      Message: `${details.Rows.length} row(s)`,
      Duration: 0,
      ...meta,
    };
  }

  return {
    Columns: ['Value', 'TTL'],
    Rows: details.Rows.map((row) => ({
      Value: redisDetailRowValue(row),
      TTL: details.TTL,
    })),
    Message: `${details.Rows.length} row(s)`,
    Duration: 0,
    ...meta,
  };
}

function redisDetailRowValue(row: Record<string, string>) {
  if (row.Score !== undefined) {
    return `${row.Value ?? ''} (${row.Score})`;
  }
  return row.Value ?? '';
}

function redisKeyReadCommand(key: RedisKeySummary) {
  const quoted = quoteRedisArg(key.Name);
  switch (key.Type) {
    case 'hash':
      return `HGETALL ${quoted}`;
    case 'list':
      return `LRANGE ${quoted} 0 -1`;
    case 'set':
      return `SMEMBERS ${quoted}`;
    case 'zset':
      return `ZRANGE ${quoted} 0 -1 WITHSCORES`;
    default:
      return `GET ${quoted}`;
  }
}

function quoteRedisArg(value: string) {
  if (/^[\w.:-]+$/.test(value)) {
    return value;
  }

  return `"${value.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`;
}

// 按 `:` 把 Redis Key 拆成可展开的命名空间树，文件夹节点只承担导航职责。
function buildRedisKeyTree(keys: RedisKeySummary[]): RedisTreeNode {
  const root: RedisTreeNode = {
    segment: '',
    path: '',
    children: [],
    keyCount: 0,
  };

  keys.forEach((key) => {
    const parts = key.Name.split(':').filter((part) => part.length > 0);
    if (parts.length === 0) {
      return;
    }

    let node = root;
    let path = '';
    parts.forEach((part, index) => {
      path = path ? `${path}:${part}` : part;
      let child = node.children.find((item) => item.segment === part);
      if (!child) {
        child = {
          segment: part,
          path,
          children: [],
          keyCount: 0,
        };
        node.children.push(child);
      }
      node = child;
      if (index === parts.length - 1) {
        node.key = key;
      }
    });
  });

  sortRedisTree(root);
  assignRedisKeyCounts(root);
  return root;
}

function sortRedisTree(node: RedisTreeNode) {
  node.children.sort((left, right) => {
    const leftIsFolder = left.children.length > 0;
    const rightIsFolder = right.children.length > 0;
    if (leftIsFolder !== rightIsFolder) {
      return leftIsFolder ? -1 : 1;
    }
    return left.segment.localeCompare(right.segment);
  });
  node.children.forEach(sortRedisTree);
}

function assignRedisKeyCounts(node: RedisTreeNode): number {
  let count = node.key ? 1 : 0;
  node.children.forEach((child) => {
    count += assignRedisKeyCounts(child);
  });
  node.keyCount = count;
  return count;
}

function queryTabName(index: number, t: TranslateFn) {
  return t('legacyText.Query {{number}}', {
    number: index,
    defaultValue: `Query ${index}`,
  });
}

function databaseResultError(result: DatabaseQueryResult): DatabaseViewError {
  return Object.assign(new Error(result.Message || 'Database query failed'), {
    ErrorCode: result.ErrorCode,
    Details: result.Message,
  });
}

function databaseCanceledError(): DatabaseViewError {
  return Object.assign(new Error('Query canceled'), {
    ErrorCode: 'request_canceled',
  });
}

function databaseErrorLabel(error: DatabaseViewError, t: TranslateFn) {
  const code = error.ErrorCode || 'unknown_database_error';
  const messageMap: Record<string, string> = {
    network_unreachable: t('legacyText.Network unreachable', {
      defaultValue:
        'Network unreachable. Check whether Portainer can access the host and port.',
    }),
    authentication_failed: t('legacyText.Database authentication failed', {
      defaultValue: 'Database authentication failed. Check username/password.',
    }),
    database_not_found: t('legacyText.Database does not exist', {
      defaultValue: 'Database does not exist.',
    }),
    object_not_found: t('legacyText.Database object does not exist', {
      defaultValue: 'Database object does not exist.',
    }),
    permission_denied: t('legacyText.Database permission denied', {
      defaultValue: 'Database permission denied.',
    }),
    timeout: t('legacyText.Database request timed out', {
      defaultValue: 'Database request timed out.',
    }),
    request_canceled: t('legacyText.Query canceled by user', {
      defaultValue: 'Query canceled.',
    }),
    agent_unreachable: t('legacyText.Agent is unreachable', {
      defaultValue:
        'Agent is unreachable. Check the endpoint URL and Agent availability.',
    }),
    sql_error: t('legacyText.SQL execution failed', {
      defaultValue: 'SQL execution failed.',
    }),
    unsafe_write_requires_confirmation: t(
      'legacyText.Unsafe write requires confirmation',
      {
        defaultValue:
          'This UPDATE/DELETE has no WHERE clause and requires confirmation.',
      }
    ),
    connection_failed: t('legacyText.Connection failed', {
      defaultValue: 'Connection failed',
    }),
    unknown_database_error: t('legacyText.Database operation failed', {
      defaultValue: 'Database operation failed.',
    }),
  };
  const message = messageMap[code] || messageMap.unknown_database_error;
  const details = error.Details || error.message;

  return details && details !== message ? `${message} ${details}` : message;
}

function loadQueryHistory(storageKey: string) {
  try {
    const rawHistory = window.localStorage.getItem(storageKey);
    const history = rawHistory
      ? (JSON.parse(rawHistory) as QueryHistoryItem[])
      : [];
    const cutoff = Date.now() - historyRetentionMs;
    return history.filter((item) => item.executedAt >= cutoff);
  } catch {
    return [];
  }
}

function saveQueryHistory(storageKey: string, history: QueryHistoryItem[]) {
  window.localStorage.setItem(storageKey, JSON.stringify(history));
}

function appendQueryHistory(
  storageKey: string,
  query: string,
  setHistory: (
    updater: (history: QueryHistoryItem[]) => QueryHistoryItem[]
  ) => void
) {
  setHistory((history) => {
    const nextHistory = [
      {
        id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
        query,
        executedAt: Date.now(),
      },
      ...history,
    ].filter((item) => item.executedAt >= Date.now() - historyRetentionMs);

    saveQueryHistory(storageKey, nextHistory);
    return nextHistory;
  });
}

function markdownTable(result: DatabaseQueryResult) {
  const columns = result.Columns;
  const header = `| ${columns.map(escapeMarkdownCell).join(' |')} |`;
  const separator = `| ${columns.map(() => '---').join(' |')} |`;
  const rows = result.Rows.map(
    (row) =>
      `| ${columns.map((column) => escapeMarkdownCell(row[column])).join(' |')} |`
  );

  return [header, separator, ...rows].join('\n');
}

function csvTable(result: DatabaseQueryResult) {
  const columns = result.Columns;
  const header = columns.map(escapeCsvCell).join(',');
  const rows = result.Rows.map((row) =>
    columns.map((column) => escapeCsvCell(row[column])).join(',')
  );

  return [header, ...rows].join('\n');
}

function jsonTable(result: DatabaseQueryResult) {
  return JSON.stringify(result.Rows, null, 2);
}

function formatJsonText(value?: string) {
  const trimmed = value?.trim();
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) {
    return undefined;
  }

  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return undefined;
  }
}

function escapeCsvCell(value?: string) {
  const normalized = String(value ?? '');
  if (!/[",\r\n]/.test(normalized)) {
    return normalized;
  }

  return `"${normalized.replaceAll('"', '""')}"`;
}

function formatTimestampForFilename(date: Date) {
  return date.toISOString().replace(/[:.]/g, '-');
}

function escapeMarkdownCell(value?: string) {
  return String(value ?? '')
    .replaceAll('\\', '\\\\')
    .replaceAll('|', '\\|')
    .replace(/\r?\n/g, '<br>');
}
