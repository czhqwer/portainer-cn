import angular from 'angular';

import { ItemView as NetworksItemView } from '@/react/docker/networks/ItemView';
import { r2a } from '@/react-tools/react2angular';
import { withCurrentUser } from '@/react-tools/withCurrentUser';
import { withReactQuery } from '@/react-tools/withReactQuery';
import { withUIRouter } from '@/react-tools/withUIRouter';
import { DashboardView } from '@/react/docker/DashboardView/DashboardView';
import { ListView } from '@/react/docker/events/ListView';
import { DatabaseView } from '@/react/docker/containers/DatabaseView/DatabaseView';

import { containersModule } from './containers';
import { configsModule } from './configs';
import { imagesModule } from './images';
import { stacksModule } from './stacks';

export const viewsModule = angular
  .module('portainer.docker.react.views', [
    containersModule,
    configsModule,
    imagesModule,
    stacksModule,
  ])
  .component(
    'dockerDashboardView',
    r2a(withUIRouter(withCurrentUser(DashboardView)), [])
  )
  .component('eventsListView', r2a(withUIRouter(withCurrentUser(ListView)), []))
  .component(
    'dockerDatabaseView',
    r2a(withUIRouter(withReactQuery(withCurrentUser(DatabaseView))), [])
  )
  .component(
    'networkDetailsView',
    r2a(withUIRouter(withCurrentUser(NetworksItemView)), [])
  ).name;
