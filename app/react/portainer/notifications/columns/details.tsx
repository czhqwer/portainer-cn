import { translateNotificationText } from '@/portainer/services/notifications';

import { columnHelper } from './helper';

export const details = columnHelper.accessor('details', {
  header: 'Details',
  id: 'details',
  cell: ({ getValue }) => {
    const value = getValue();

    return (
      <div className="whitespace-normal">
        {translateNotificationText(value || '')}
      </div>
    );
  },
});
