import { FileCode } from 'lucide-react';
import { createColumnHelper } from '@tanstack/react-table';

import { RoleTypes } from '@/portainer/rbac/models/role';

import { Datatable } from '@@/datatables';
import { createPersistedStore } from '@@/datatables/types';
import { useTableState } from '@@/datatables/useTableState';

import { isBE } from '../../feature-flags/feature-flags.service';

import { RbacRole } from './types';

const tableKey = 'rbac-roles-table';

const store = createPersistedStore(tableKey);

const columns = getColumns();

export function RbacRolesDatatable({
  dataset,
}: {
  dataset: Array<RbacRole> | undefined;
}) {
  const tableState = useTableState(store, tableKey);
  const filteredDataset = isBE
    ? dataset
    : dataset?.filter((role) => role.Id === RoleTypes.STANDARD);

  return (
    <Datatable
      title="Roles"
      titleIcon={FileCode}
      dataset={filteredDataset || []}
      columns={columns}
      isLoading={!dataset}
      settingsManager={tableState}
      disableSelect
      data-cy="rbac-roles-datatable"
    />
  );
}

function getColumns() {
  const columnHelper = createColumnHelper<RbacRole>();

  return [
    columnHelper.accessor('Name', {
      header: 'Name',
    }),
    columnHelper.accessor('Description', {
      header: 'Description',
    }),
  ];
}
