import { translateNotificationText } from '@/portainer/services/notifications';

import { columnHelper } from './helper';

export const title = columnHelper.accessor('title', {
  header: 'Title',
  id: 'title',
  cell: ({ getValue }) => translateNotificationText(getValue() || ''),
});
