import _ from 'lodash';

import i18n from '@/i18n';

import { columnHelper } from './helper';

export const type = columnHelper.accessor('type', {
  header: 'Type',
  id: 'type',
  cell: ({ getValue }) => {
    const value = getValue();

    const capitalized = _.capitalize(value);
    return i18n.t(`legacyText.${capitalized}`, { defaultValue: capitalized });
  },
});
