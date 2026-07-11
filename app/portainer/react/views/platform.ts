import angular from 'angular';

import {
  PlatformApplicationsView,
  PlatformArtifactsView,
  PlatformConfigView,
  PlatformDeployView,
  PlatformProjectsView,
  PlatformReleasesView,
} from '@/react/portainer/platform/PlatformViews';
import { r2a } from '@/react-tools/react2angular';
import { withCurrentUser } from '@/react-tools/withCurrentUser';
import { withReactQuery } from '@/react-tools/withReactQuery';
import { withUIRouter } from '@/react-tools/withUIRouter';

export const platformViewsModule = angular
  .module('portainer.app.react.views.platform', [])
  .component(
    'platformProjectsView',
    r2a(withUIRouter(withReactQuery(withCurrentUser(PlatformProjectsView))), [])
  )
  .component(
    'platformApplicationsView',
    r2a(
      withUIRouter(withReactQuery(withCurrentUser(PlatformApplicationsView))),
      []
    )
  )
  .component(
    'platformDeployView',
    r2a(withUIRouter(withReactQuery(withCurrentUser(PlatformDeployView))), [])
  )
  .component(
    'platformArtifactsView',
    r2a(
      withUIRouter(withReactQuery(withCurrentUser(PlatformArtifactsView))),
      []
    )
  )
  .component(
    'platformConfigView',
    r2a(withUIRouter(withReactQuery(withCurrentUser(PlatformConfigView))), [])
  )
  .component(
    'platformReleasesView',
    r2a(withUIRouter(withReactQuery(withCurrentUser(PlatformReleasesView))), [])
  ).name;
