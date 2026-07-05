import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { useCurrentStateAndParams } from '@uirouter/react';
import { useTranslation } from 'react-i18next';
import {
  ChevronDown,
  ChevronRight,
  Check,
  Copy,
  Database,
  Eye,
  FolderTree,
  FlaskConical,
  History,
  ListTree,
  Pencil,
  Play,
  Plus,
  Save,
  Table2,
  Trash2,
  X,
} from 'lucide-react';

import { EnvironmentId } from '@/react/portainer/environments/types';
import { useCurrentUser, useIsPureAdmin } from '@/react/hooks/useUser';
import { useContainers } from '@/react/docker/containers/queries/useContainers';
import {
  ContainerListViewModel,
  ContainerStatus,
} from '@/react/docker/containers/types';

import { PageHeader } from '@@/PageHeader';
import { Icon } from '@@/Icon';
import { Button, LoadingButton } from '@@/buttons';
import { FormControl } from '@@/form-components/FormControl';

import {
  DatabaseConnection,
  DatabaseConnectionPayload,
  DatabaseConnectionType,
  DatabaseQueryResult,
  DatabaseSchema,
  useCreateDatabaseConnection,
  useDatabaseConnections,
  useDatabaseSchema,
  useDeleteDatabaseConnection,
  useRunDatabaseQuery,
  useTestDatabaseConnection,
  useUpdateDatabaseConnection,
} from './database-queries';

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
const defaultSqlComment = '-- SQL 查询结果最多显示 200 行';
const defaultSqlTemplate = defaultSqlComment;
const historyRetentionMs = 7 * 24 * 60 * 60 * 1000;

type QueryHistoryItem = {
  id: string;
  query: string;
  executedAt: number;
};

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

  const connections = connectionsQuery.data || [];
  const [selectedId, setSelectedId] = useState<number>();
  const [editingId, setEditingId] = useState<number>();
  const [formMode, setFormMode] = useState<FormMode>();
  const [formValues, setFormValues] =
    useState<ConnectionFormValues>(defaultFormValues);
  const [query, setQuery] = useState(defaultSqlTemplate);
  const [selectedDatabase, setSelectedDatabase] = useState('');
  const [history, setHistory] = useState<QueryHistoryItem[]>([]);
  const [expandedDatabases, setExpandedDatabases] = useState<
    Record<string, boolean>
  >({});

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
    () => schemaQuery.data?.Databases.map((database) => database.Name) || [],
    [schemaQuery.data]
  );
  const activeDatabase =
    selectedDatabase || activeConnection?.Database || databaseOptions[0] || '';
  const historyStorageKey = `portainer.databaseHistory.${user.Id}.${environmentId}.${activeConnection?.Id || 'none'}`;

  useEffect(() => {
    if (!activeConnection) {
      setSelectedDatabase('');
      return;
    }

    const preferredDatabase =
      activeConnection.Database &&
      databaseOptions.includes(activeConnection.Database)
        ? activeConnection.Database
        : databaseOptions[0] || activeConnection.Database || '';

    setSelectedDatabase(preferredDatabase);
  }, [activeConnection?.Id, activeConnection?.Database, databaseOptions]);

  useEffect(() => {
    const nextHistory = loadQueryHistory(historyStorageKey);
    setHistory(nextHistory);
    saveQueryHistory(historyStorageKey, nextHistory);
  }, [historyStorageKey]);

  function selectConnection(connection: DatabaseConnection) {
    setSelectedId(connection.Id);
    setQuery(connection.Type === 'redis' ? 'PING' : defaultSqlTemplate);
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

    const payload = normalizePayload(formValues);

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
    const statement = prepareQueryForExecution(queryToRun || query);
    if (!activeConnection || !statement) {
      return;
    }

    const isWriteStatement = isUpdateOrDeleteStatement(statement);

    if (isWriteStatement) {
      const preview = await runQuery.mutateAsync({
        connection: activeConnection,
        query: statement,
        database: activeDatabase,
        preview: true,
        nodeName: activeContainer?.NodeName,
      });

      const confirmed = window.confirm(
        t(
          'legacyText.This statement will affect {{count}} row(s). Execute it?',
          {
            count: preview.RowsAffected || 0,
            defaultValue: `This statement will affect ${preview.RowsAffected || 0} row(s). Execute it?`,
          }
        )
      );

      if (!confirmed) {
        return;
      }
    }

    await runQuery.mutateAsync({
      connection: activeConnection,
      query: statement,
      database: activeDatabase,
      nodeName: activeContainer?.NodeName,
    });
    appendQueryHistory(historyStorageKey, statement, setHistory);
  }

  function toggleDatabase(name: string) {
    setExpandedDatabases((expanded) => ({
      ...expanded,
      [name]: !expanded[name],
    }));
  }

  function selectTable(database: string, table: string) {
    if (!activeConnection) {
      return;
    }

    setQuery((current) =>
      appendSqlStatement(
        current,
        tableSelectStatement(activeConnection.Type, database, table)
      )
    );
  }

  if (!isPureAdmin) {
    return null;
  }

  return (
    <>
      <PageHeader
        title={t('legacyText.Database query', {
          defaultValue: 'Database query',
        })}
        breadcrumbs={t('legacyText.Databases', { defaultValue: 'Databases' })}
        reload
      />

      <div className="flex h-[calc(100vh-170px)] max-h-[calc(100vh-170px)] min-h-[560px] gap-4 overflow-hidden">
        <aside className="flex h-full w-[300px] shrink-0 flex-col overflow-hidden border-r border-gray-5 pr-4">
          <ConnectionSidebar
            connections={connections}
            activeConnection={activeConnection}
            isLoading={connectionsQuery.isLoading}
            schema={schemaQuery.data}
            isSchemaLoading={schemaQuery.isLoading}
            databaseFilter={activeDatabase}
            expandedDatabases={expandedDatabases}
            onSelectConnection={selectConnection}
            onCreate={openCreateForm}
            onEdit={openEditForm}
            onDelete={handleDelete}
            onToggleDatabase={toggleDatabase}
            onSelectTable={selectTable}
          />
        </aside>

        <main className="flex h-full min-w-0 flex-1 flex-col overflow-hidden">
          <QueryWorkspace
            activeConnection={activeConnection}
            databases={databaseOptions}
            selectedDatabase={activeDatabase}
            query={query}
            result={runQuery.data}
            history={history}
            isRunning={runQuery.isLoading}
            onSelectDatabase={setSelectedDatabase}
            onQueryChange={setQuery}
            onRun={handleRunQuery}
          />
        </main>
      </div>

      {formMode && (
        <div className="fixed inset-y-0 right-0 z-50 flex w-[420px] max-w-full flex-col border-l border-gray-5 bg-gray-10 p-5 shadow-2xl">
          <ConnectionForm
            values={formValues}
            isEditing={formMode === 'edit'}
            isLoading={createConnection.isLoading || updateConnection.isLoading}
            isTesting={testConnection.isLoading}
            onCancel={closeForm}
            onTest={async (values) => {
              const payload = normalizePayload({
                ...values,
                Name: values.Name || 'test',
              });
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
          />
        </div>
      )}
    </>
  );
}

function ConnectionSidebar({
  connections,
  activeConnection,
  isLoading,
  schema,
  isSchemaLoading,
  databaseFilter,
  expandedDatabases,
  onSelectConnection,
  onCreate,
  onEdit,
  onDelete,
  onToggleDatabase,
  onSelectTable,
}: {
  connections: DatabaseConnection[];
  activeConnection?: DatabaseConnection;
  isLoading: boolean;
  schema?: DatabaseSchema;
  isSchemaLoading: boolean;
  databaseFilter?: string;
  expandedDatabases: Record<string, boolean>;
  onSelectConnection: (connection: DatabaseConnection) => void;
  onCreate: () => void;
  onEdit: (connection: DatabaseConnection) => void;
  onDelete: (connection: DatabaseConnection) => void;
  onToggleDatabase: (name: string) => void;
  onSelectTable: (database: string, table: string) => void;
}) {
  const { t } = useTranslation();
  const [isConnectionListOpen, setIsConnectionListOpen] = useState(true);

  return (
    <>
      <div className="flex items-center gap-2 pb-3">
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
          onClick={() => setIsConnectionListOpen((isOpen) => !isOpen)}
          data-cy="database-select-connection-button"
        >
          <Icon icon={ListTree} className="lucide" />
        </Button>
        <Button
          type="button"
          color="default"
          size="small"
          className="ml-auto"
          onClick={onCreate}
          data-cy="database-add-connection-button"
        >
          <Icon icon={Plus} className="lucide" />
        </Button>
      </div>

      {isConnectionListOpen && (
        <div className="max-h-[240px] overflow-auto pb-4">
          {isLoading && (
            <span className="text-muted">
              {t('common.Loading...', { defaultValue: 'Loading...' })}
            </span>
          )}
          {!isLoading && connections.length === 0 && (
            <span className="text-muted">
              {t('legacyText.No saved connections', {
                defaultValue: 'No saved connections',
              })}
            </span>
          )}
          <div className="space-y-2">
            {connections.map((connection) => (
              <div
                key={connection.Id}
                role="button"
                tabIndex={0}
                className={`w-full rounded border p-3 text-left transition ${
                  activeConnection?.Id === connection.Id
                    ? 'border-blue-8 bg-blue-8 text-white'
                    : 'border-gray-5 bg-white text-gray-10 hover:bg-gray-2 th-dark:bg-gray-iron-11 th-dark:text-white th-dark:hover:bg-gray-iron-10'
                }`}
                onClick={() => onSelectConnection(connection)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    onSelectConnection(connection);
                  }
                }}
              >
                <div className="flex items-center gap-2">
                  <span className="truncate font-semibold">
                    {connection.Name}
                  </span>
                  <span className="label label-default ml-auto">
                    {connection.Type}
                  </span>
                </div>
                <div className="mt-1 truncate text-xs opacity-80">
                  {connection.ContainerId
                    ? t('legacyText.Container', {
                        defaultValue: 'Container',
                      })
                    : t('legacyText.Custom address', {
                        defaultValue: 'Custom address',
                      })}
                  {' - '}
                  {connection.Host}:{connection.Port}
                  {connection.Database ? ` / ${connection.Database}` : ''}
                </div>
                {activeConnection?.Id === connection.Id && (
                  <div className="mt-3 flex justify-end gap-2">
                    <Button
                      type="button"
                      color="default"
                      size="small"
                      onClick={(event) => {
                        event.stopPropagation();
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
                      onClick={(event) => {
                        event.stopPropagation();
                        onDelete(connection);
                      }}
                      data-cy="database-delete-connection-button"
                    >
                      <Icon icon={Trash2} className="lucide" />
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="flex items-center gap-2 border-t border-gray-5 py-3">
        <Icon icon={FolderTree} className="lucide" />
        <span className="font-semibold">
          {t('panelTitles.Database tree', {
            defaultValue: 'Database tree',
          })}
        </span>
      </div>

      <SchemaTree
        schema={schema}
        isLoading={isSchemaLoading}
        databaseFilter={databaseFilter}
        expandedDatabases={expandedDatabases}
        onToggleDatabase={onToggleDatabase}
        onSelectTable={onSelectTable}
      />
    </>
  );
}

function SchemaTree({
  schema,
  isLoading,
  databaseFilter,
  expandedDatabases,
  onToggleDatabase,
  onSelectTable,
}: {
  schema?: DatabaseSchema;
  isLoading: boolean;
  databaseFilter?: string;
  expandedDatabases: Record<string, boolean>;
  onToggleDatabase: (name: string) => void;
  onSelectTable: (database: string, table: string) => void;
}) {
  const { t } = useTranslation();

  if (isLoading) {
    return (
      <span className="text-muted">
        {t('legacyText.Loading database tree...', {
          defaultValue: 'Loading database tree...',
        })}
      </span>
    );
  }

  if (schema?.Message) {
    return (
      <span className="text-muted">
        {t(`legacyText.${schema.Message}`, { defaultValue: schema.Message })}
      </span>
    );
  }

  const databases = databaseFilter
    ? schema?.Databases.filter((database) => database.Name === databaseFilter)
    : schema?.Databases;

  if (!databases || databases.length === 0) {
    return (
      <span className="text-muted">
        {t('legacyText.No database tree available', {
          defaultValue: 'No database tree available',
        })}
      </span>
    );
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto pr-1 text-sm [color-scheme:light] th-dark:[color-scheme:dark]">
      {databases.map((database) => {
        const expanded = expandedDatabases[database.Name] ?? false;
        return (
          <div key={database.Name}>
            <button
              type="button"
              className="flex w-full items-center gap-1 rounded px-1.5 py-1 text-left text-gray-10 hover:bg-gray-2 th-dark:text-white th-dark:hover:bg-gray-iron-9"
              onClick={() => onToggleDatabase(database.Name)}
            >
              <Icon
                icon={expanded ? ChevronDown : ChevronRight}
                className="lucide"
              />
              <span className="truncate">{database.Name}</span>
            </button>
            {expanded && (
              <div className="ml-5">
                {database.Tables.length === 0 && (
                  <div className="text-muted px-2 py-1 text-xs">
                    {t('legacyText.No tables', { defaultValue: 'No tables' })}
                  </div>
                )}
                {database.Tables.map((table) => (
                  <button
                    key={`${database.Name}.${table.Name}`}
                    type="button"
                    className="flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-left text-xs text-gray-9 hover:bg-blue-2 hover:text-blue-9 th-dark:text-gray-2 th-dark:hover:bg-gray-iron-9 th-dark:hover:text-white"
                    onClick={() => onSelectTable(database.Name, table.Name)}
                  >
                    <Icon icon={Table2} className="lucide" />
                    <span className="truncate">{table.Name}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function QueryWorkspace({
  activeConnection,
  databases,
  selectedDatabase,
  query,
  result,
  history,
  isRunning,
  onSelectDatabase,
  onQueryChange,
  onRun,
}: {
  activeConnection?: DatabaseConnection;
  databases: string[];
  selectedDatabase: string;
  query: string;
  result?: DatabaseQueryResult;
  history: QueryHistoryItem[];
  isRunning: boolean;
  onSelectDatabase: (database: string) => void;
  onQueryChange: (query: string) => void;
  onRun: (queryToRun?: string) => void;
}) {
  const { t } = useTranslation();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [editorScrollTop, setEditorScrollTop] = useState(0);
  const [isHistoryOpen, setIsHistoryOpen] = useState(false);
  const lineNumbers = useMemo(
    () => Array.from({ length: Math.max(query.split('\n').length, 1) }),
    [query]
  );

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
      <div className="flex items-center gap-3 pb-3">
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
          disabled={
            databases.length === 0 || activeConnection?.Type === 'redis'
          }
          aria-label={t('legacyText.Database', {
            defaultValue: 'Database',
          })}
        >
          {databases.map((database) => (
            <option key={database} value={database}>
              {database}
            </option>
          ))}
        </select>
        <LoadingButton
          type="button"
          color="primary"
          onClick={() => onRun(selectedOrAllQuery())}
          disabled={!activeConnection || !query.trim()}
          isLoading={isRunning}
          loadingText={t('buttons.Running...', {
            defaultValue: 'Running...',
          })}
          data-cy="database-run-query-button"
        >
          <Icon icon={Play} className="lucide space-right" />
          {t('buttons.Run', { defaultValue: 'Run' })}
        </LoadingButton>
      </div>

      <div className="flex min-h-[260px] flex-1 overflow-hidden rounded border border-gray-5 bg-white text-gray-10 th-dark:bg-gray-iron-11 th-dark:text-white">
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
          placeholder={
            activeConnection?.Type === 'redis' ? 'PING' : defaultSqlTemplate
          }
          aria-label={activeConnection?.Type === 'redis' ? 'Command' : 'SQL'}
        />
      </div>

      <QueryResult result={result} />

      {isHistoryOpen && (
        <QueryHistoryModal
          history={history}
          onClose={() => setIsHistoryOpen(false)}
        />
      )}
    </>
  );
}

function ConnectionForm({
  values,
  isEditing,
  isLoading,
  isTesting,
  containers,
  isContainerTargetVisible,
  onCancel,
  onTest,
  onSave,
  onChange,
}: {
  values: ConnectionFormValues;
  isEditing: boolean;
  isLoading: boolean;
  isTesting: boolean;
  containers: ContainerListViewModel[];
  isContainerTargetVisible: boolean;
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

  function updateValue<T extends keyof ConnectionFormValues>(
    key: T,
    value: ConnectionFormValues[T]
  ) {
    onChange({ ...values, [key]: value });
  }

  function updateType(type: DatabaseConnectionType) {
    onChange({
      ...values,
      Type: type,
      Port: values.ContainerId
        ? suggestedPortForType(selectedContainer, type)
        : portByType[type],
      Database: type === 'redis' ? '' : values.Database,
    });
  }

  function updateTarget(target: 'custom' | 'container') {
    if (target === 'custom') {
      onChange({ ...values, ContainerId: '' });
      return;
    }

    const container = containers[0];
    onChange(applyContainerSuggestion(values, container, values.Type));
  }

  function updateContainer(containerId: string) {
    const container = containers.find(
      (container) => container.Id === containerId
    );
    onChange(
      applyContainerSuggestion(
        { ...values, ContainerId: containerId },
        container,
        values.Type
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

      <div className="form-actions mt-4">
        <LoadingButton
          type="button"
          color="default"
          isLoading={isTesting}
          loadingText={t('buttons.Testing...', { defaultValue: 'Testing...' })}
          data-cy="database-test-connection-button"
          onClick={async () => {
            setTestStatus(undefined);
            try {
              await onTest(values);
              setTestStatus('success');
            } catch {
              setTestStatus('error');
            }
          }}
        >
          <Icon icon={FlaskConical} className="lucide space-right" />
          {t('buttons.Test Connection', { defaultValue: 'Test Connection' })}
        </LoadingButton>
        <LoadingButton
          type="submit"
          color="primary"
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
          data-cy="database-cancel-edit-button"
          onClick={onCancel}
        >
          {t('buttons.Cancel', { defaultValue: 'Cancel' })}
        </Button>
      </div>
      {testStatus && (
        <div
          className={`small mt-2 ${
            testStatus === 'success' ? 'text-success' : 'text-danger'
          }`}
        >
          {testStatus === 'success'
            ? t('legacyText.Connection successful', {
                defaultValue: 'Connection successful',
              })
            : t('legacyText.Connection failed', {
                defaultValue: 'Connection failed',
              })}
        </div>
      )}
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

  async function copyDetail() {
    if (!detail) {
      return;
    }

    await navigator.clipboard?.writeText(detail.value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  }

  async function copyMarkdown() {
    if (!result) {
      return;
    }

    await navigator.clipboard?.writeText(markdownTable(result));
    setCopiedMarkdown(true);
    window.setTimeout(() => setCopiedMarkdown(false), 1500);
  }

  if (!result) {
    return (
      <div className="text-muted mt-4 min-h-[220px] rounded border border-gray-5 p-4">
        {t('legacyText.Run a query to see results.', {
          defaultValue: 'Run a query to see results.',
        })}
      </div>
    );
  }

  return (
    <div className="mt-4 min-h-0 flex-1 overflow-hidden rounded border border-gray-5">
      <div className="flex items-center gap-2 border-b border-gray-5 px-3 py-2">
        <span className="font-semibold">
          {t('panelTitles.Results', { defaultValue: 'Results' })}
        </span>
        <span className="text-muted">{result.Message}</span>
        <Button
          type="button"
          color="default"
          size="small"
          className="ml-auto"
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
        <span className="small text-muted">{result.Duration.toFixed(3)}s</span>
      </div>
      <div className="h-full overflow-auto">
        <table className="table-hover nowrap-cells mb-0 table">
          <thead className="sticky top-0 z-10 bg-gray-2 text-gray-10 th-dark:bg-blue-11 th-dark:text-white">
            <tr>
              {result.Columns.map((column) => (
                <th key={column}>{column}</th>
              ))}
            </tr>
          </thead>
          <tbody>
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
                      <span className="min-w-0 flex-1 truncate">
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
              <span className="text-muted truncate text-sm">
                {detail.column}
              </span>
              <Button
                type="button"
                color="default"
                size="small"
                className="ml-auto"
                data-cy="database-cell-detail-close-button"
                onClick={() => setDetail(undefined)}
              >
                <Icon icon={X} className="lucide" />
              </Button>
            </div>
            <pre className="m-0 min-h-[180px] overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-sm">
              {detail.value}
            </pre>
            <div className="flex justify-end gap-2 border-t border-gray-5 px-4 py-3">
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
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function normalizePayload(
  values: ConnectionFormValues
): DatabaseConnectionPayload {
  return {
    ...values,
    Name: values.Name.trim(),
    Host: values.Host.trim(),
    Database: values.Database?.trim(),
    Username: values.Username?.trim(),
    Password: values.Password?.trim(),
    ContainerId: values.ContainerId || '',
    QueryTimeout: Math.min(120, Math.max(5, Number(values.QueryTimeout) || 30)),
  };
}

function applyContainerSuggestion(
  values: ConnectionFormValues,
  container: ContainerListViewModel | undefined,
  type: DatabaseConnectionType
): ConnectionFormValues {
  const containerName = container?.Names?.[0]?.replace(/^\//, '') || '';

  return {
    ...values,
    ContainerId: container?.Id || values.ContainerId || '',
    Name: values.Name || containerName,
    Host: container?.IP || '127.0.0.1',
    Port: suggestedPortForType(container, type),
  };
}

function suggestedPortForType(
  container: ContainerListViewModel | undefined,
  type: DatabaseConnectionType
) {
  const defaultPort = portByType[type];
  return (
    container?.ExposedPorts?.find((port) => port.private === defaultPort)
      ?.private ||
    container?.Ports?.find((port) => port.private === defaultPort)?.private ||
    container?.ExposedPorts?.[0]?.private ||
    defaultPort
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

function escapeMarkdownCell(value?: string) {
  return String(value ?? '')
    .replaceAll('\\', '\\\\')
    .replaceAll('|', '\\|')
    .replace(/\r?\n/g, '<br>');
}
