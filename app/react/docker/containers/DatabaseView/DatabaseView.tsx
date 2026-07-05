import { FormEvent, useMemo, useState } from 'react';
import { useCurrentStateAndParams } from '@uirouter/react';
import { useTranslation } from 'react-i18next';
import {
  Database,
  Pencil,
  Play,
  Plus,
  Save,
  Search,
  Trash2,
  X,
} from 'lucide-react';

import { EnvironmentId } from '@/react/portainer/environments/types';

import { PageHeader } from '@@/PageHeader';
import { Icon } from '@@/Icon';
import { Button, ButtonGroup, LoadingButton } from '@@/buttons';
import { FormControl } from '@@/form-components/FormControl';

import {
  DatabaseConnection,
  DatabaseConnectionPayload,
  DatabaseConnectionType,
  useCreateDatabaseConnection,
  useDatabaseConnections,
  useDeleteDatabaseConnection,
  useRunDatabaseQuery,
  useUpdateDatabaseConnection,
} from './database-queries';

type ConnectionFormValues = DatabaseConnectionPayload;

const defaultFormValues: ConnectionFormValues = {
  Name: '',
  Type: 'mysql',
  Host: '127.0.0.1',
  Port: 3306,
  Database: '',
  Username: '',
  Password: '',
  QueryTimeout: 30,
};

const redisCommands = [
  'GET key',
  'SET key value',
  'DEL key',
  'KEYS pattern',
  'HGETALL key',
  'LRANGE key 0 -1',
];

const sqlTemplates = [
  'SELECT * FROM table_name LIMIT 100;',
  'INSERT INTO table_name (column_name) VALUES (value);',
  'UPDATE table_name SET column_name = value WHERE id = 1;',
  'DELETE FROM table_name WHERE id = 1;',
];

const portByType: Record<DatabaseConnectionType, number> = {
  mysql: 3306,
  mariadb: 3306,
  postgres: 5432,
  redis: 6379,
};

export function DatabaseView() {
  const { t } = useTranslation();
  const {
    params: { endpointId, id: containerId, nodeName },
  } = useCurrentStateAndParams();
  const environmentId = Number(endpointId) as EnvironmentId;

  const connectionsQuery = useDatabaseConnections(
    environmentId,
    containerId,
    nodeName
  );
  const createConnection = useCreateDatabaseConnection(
    environmentId,
    containerId,
    nodeName
  );
  const updateConnection = useUpdateDatabaseConnection(
    environmentId,
    containerId,
    nodeName
  );
  const deleteConnection = useDeleteDatabaseConnection(
    environmentId,
    containerId,
    nodeName
  );
  const runQuery = useRunDatabaseQuery(environmentId, containerId, nodeName);

  const connections = connectionsQuery.data || [];
  const [selectedId, setSelectedId] = useState<number>();
  const [editingId, setEditingId] = useState<number>();
  const [formValues, setFormValues] =
    useState<ConnectionFormValues>(defaultFormValues);
  const [query, setQuery] = useState(sqlTemplates[0]);

  const selectedConnection = useMemo(
    () => connections.find((connection) => connection.Id === selectedId),
    [connections, selectedId]
  );

  const activeConnection = selectedConnection || connections[0];
  const queryTemplates =
    activeConnection?.Type === 'redis' ? redisCommands : sqlTemplates;

  function selectConnection(connection: DatabaseConnection) {
    setSelectedId(connection.Id);
    setEditingId(undefined);
    setQuery(connection.Type === 'redis' ? redisCommands[0] : sqlTemplates[0]);
  }

  function editConnection(connection: DatabaseConnection) {
    setSelectedId(connection.Id);
    setEditingId(connection.Id);
    setFormValues({
      Name: connection.Name,
      Type: connection.Type,
      Host: connection.Host,
      Port: connection.Port,
      Database: connection.Database,
      Username: connection.Username,
      Password: '',
      QueryTimeout: connection.QueryTimeout,
    });
  }

  function resetForm() {
    setEditingId(undefined);
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

    resetForm();
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
      resetForm();
    }
  }

  async function handleRunQuery() {
    if (!activeConnection || !query.trim()) {
      return;
    }

    await runQuery.mutateAsync({
      id: activeConnection.Id,
      query,
    });
  }

  return (
    <>
      <PageHeader
        title="Database query"
        breadcrumbs="Containers"
        reload
      />

      <div className="row">
        <div className="col-sm-4 col-lg-3">
          <section className="mb-4">
            <div className="form-section-title">
              <Icon icon={Database} className="lucide space-right" />
              {t('panelTitles.Connections', { defaultValue: 'Connections' })}
            </div>
            <div className="vertical-center mb-2">
              {connectionsQuery.isLoading && (
                <span>
                  {t('common.Loading...', { defaultValue: 'Loading...' })}
                </span>
              )}
              {!connectionsQuery.isLoading && connections.length === 0 && (
                <span className="text-muted">
                  {t('legacyText.No saved connections', {
                    defaultValue: 'No saved connections',
                  })}
                </span>
              )}
            </div>
            <div className="list-group">
              {connections.map((connection) => (
                <button
                  key={connection.Id}
                  type="button"
                  className={`list-group-item list-group-item-action ${
                    activeConnection?.Id === connection.Id ? 'active' : ''
                  }`}
                  onClick={() => selectConnection(connection)}
                >
                  <div className="vertical-center">
                    <span className="font-bold">{connection.Name}</span>
                    <span className="label label-default ml-auto">
                      {connection.Type}
                    </span>
                  </div>
                  <div className="small text-muted">
                    {connection.Host}:{connection.Port}
                    {connection.Database ? ` / ${connection.Database}` : ''}
                  </div>
                </button>
              ))}
            </div>
          </section>

          <ConnectionForm
            values={formValues}
            isEditing={!!editingId}
            isLoading={createConnection.isLoading || updateConnection.isLoading}
            onCancel={resetForm}
            onSave={handleSave}
            onChange={setFormValues}
          />
        </div>

        <div className="col-sm-8 col-lg-9">
          <section>
            <div className="form-section-title">
              <Icon icon={Search} className="lucide space-right" />
              {t('panelTitles.Query', { defaultValue: 'Query' })}
            </div>

            <div className="mb-3">
              <ButtonGroup>
                {queryTemplates.map((template) => (
                  <Button
                    key={template}
                    type="button"
                    color="default"
                    size="small"
                    data-cy={`database-query-template-${template
                      .split(' ')[0]
                      .toLowerCase()}`}
                    onClick={() => setQuery(template)}
                  >
                    {template.split(' ')[0]}
                  </Button>
                ))}
              </ButtonGroup>
            </div>

            <FormControl label="Connection">
              <select
                className="form-control"
                value={activeConnection?.Id || ''}
                onChange={(event) => {
                  const next = connections.find(
                    (connection) =>
                      connection.Id === Number(event.target.value)
                  );
                  if (next) {
                    selectConnection(next);
                  }
                }}
                disabled={connections.length === 0}
              >
                {connections.map((connection) => (
                  <option key={connection.Id} value={connection.Id}>
                    {connection.Name}
                  </option>
                ))}
              </select>
            </FormControl>

            <FormControl
              label={activeConnection?.Type === 'redis' ? 'Command' : 'SQL'}
            >
              <textarea
                className="form-control vertical-resize"
                rows={8}
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={
                  activeConnection?.Type === 'redis'
                    ? 'GET key'
                    : 'SELECT * FROM table_name LIMIT 100;'
                }
              />
            </FormControl>

            <div className="form-actions">
              <LoadingButton
                type="button"
                color="primary"
                onClick={handleRunQuery}
                disabled={!activeConnection || !query.trim()}
                isLoading={runQuery.isLoading}
                loadingText="Running..."
                data-cy="database-run-query-button"
              >
                <Icon icon={Play} className="lucide space-right" />
                {t('buttons.Run', { defaultValue: 'Run' })}
              </LoadingButton>
              {activeConnection && (
                <ButtonGroup>
                  <Button
                    type="button"
                    color="default"
                    data-cy="database-edit-connection-button"
                    onClick={() => editConnection(activeConnection)}
                  >
                    <Icon icon={Pencil} className="lucide space-right" />
                    {t('buttons.Edit', { defaultValue: 'Edit' })}
                  </Button>
                  <Button
                    type="button"
                    color="dangerlight"
                    data-cy="database-delete-connection-button"
                    onClick={() => handleDelete(activeConnection)}
                  >
                    <Icon icon={Trash2} className="lucide space-right" />
                    {t('buttons.Delete', { defaultValue: 'Delete' })}
                  </Button>
                </ButtonGroup>
              )}
            </div>

            <QueryResult result={runQuery.data} />
          </section>
        </div>
      </div>
    </>
  );
}

function ConnectionForm({
  values,
  isEditing,
  isLoading,
  onCancel,
  onSave,
  onChange,
}: {
  values: ConnectionFormValues;
  isEditing: boolean;
  isLoading: boolean;
  onCancel: () => void;
  onSave: (event: FormEvent) => void;
  onChange: (values: ConnectionFormValues) => void;
}) {
  const { t } = useTranslation();

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
      Port: portByType[type],
      Database: type === 'redis' ? '' : values.Database,
    });
  }

  return (
    <form onSubmit={onSave}>
      <div className="form-section-title">
        <Icon icon={isEditing ? Pencil : Plus} className="lucide space-right" />
        {isEditing
          ? t('panelTitles.Edit connection', {
              defaultValue: 'Edit connection',
            })
          : t('panelTitles.Add connection', {
              defaultValue: 'Add connection',
            })}
      </div>

      <FormControl label="Name" inputId="database-connection-name">
        <input
          id="database-connection-name"
          className="form-control"
          value={values.Name}
          onChange={(event) => updateValue('Name', event.target.value)}
          required
        />
      </FormControl>

      <FormControl label="Type" inputId="database-connection-type">
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

      <FormControl label="Host" inputId="database-connection-host">
        <input
          id="database-connection-host"
          className="form-control"
          value={values.Host}
          onChange={(event) => updateValue('Host', event.target.value)}
          required
        />
      </FormControl>

      <FormControl label="Port" inputId="database-connection-port">
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
        <FormControl label="Database" inputId="database-connection-database">
          <input
            id="database-connection-database"
            className="form-control"
            value={values.Database}
            onChange={(event) => updateValue('Database', event.target.value)}
          />
        </FormControl>
      )}

      <FormControl label="Username" inputId="database-connection-username">
        <input
          id="database-connection-username"
          className="form-control"
          value={values.Username}
          onChange={(event) => updateValue('Username', event.target.value)}
        />
      </FormControl>

      <FormControl label="Password" inputId="database-connection-password">
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

      <FormControl label="Timeout" inputId="database-connection-timeout">
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

      <div className="form-actions">
        <LoadingButton
          type="submit"
          color="primary"
          isLoading={isLoading}
          loadingText="Saving..."
          data-cy="database-save-connection-button"
        >
          <Icon icon={Save} className="lucide space-right" />
          {t('buttons.Save', { defaultValue: 'Save' })}
        </LoadingButton>
        {isEditing && (
          <Button
            type="button"
            color="default"
            data-cy="database-cancel-edit-button"
            onClick={onCancel}
          >
            <Icon icon={X} className="lucide space-right" />
            {t('buttons.Cancel', { defaultValue: 'Cancel' })}
          </Button>
        )}
      </div>
    </form>
  );
}

function QueryResult({
  result,
}: {
  result?: {
    Columns: string[];
    Rows: Array<Record<string, string>>;
    Message: string;
    Duration: number;
  };
}) {
  const { t } = useTranslation();

  if (!result) {
    return null;
  }

  return (
    <div className="table-responsive mt-4">
      <div className="vertical-center mb-2">
        <span>{result.Message}</span>
        <span className="small text-muted ml-auto">
          {result.Duration.toFixed(3)}s
        </span>
      </div>
      <table className="table table-hover nowrap-cells">
        <thead>
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
                <td key={column}>{row[column]}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
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
    QueryTimeout: Math.min(120, Math.max(5, Number(values.QueryTimeout) || 30)),
  };
}
