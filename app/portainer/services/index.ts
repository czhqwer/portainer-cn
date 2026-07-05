import angular from 'angular';

import { apiServicesModule } from './api';
import { Notifications } from './notifications';
import { HttpRequestHelperAngular } from './http-request.helper';
import { EndpointProvider } from './endpointProvider';
import { AngularToReact } from './angularToReact';
import { LegacyI18n } from './legacy-i18n.service';

export default angular
  .module('portainer.app.services', [apiServicesModule])
  .factory('Notifications', Notifications)
  .factory('EndpointProvider', EndpointProvider)
  .factory('HttpRequestHelper', HttpRequestHelperAngular)
  .factory('AngularToReact', AngularToReact)
  .factory('LegacyI18n', LegacyI18n).name;
