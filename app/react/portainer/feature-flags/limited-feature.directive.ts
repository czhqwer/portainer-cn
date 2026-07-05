import { IAttributes, IDirective, IScope } from 'angular';

import { FeatureState } from '@/react/portainer/feature-flags/enums';

import { selectShow } from './feature-flags.service';

/* @ngInject */
export function limitedFeatureDirective(): IDirective {
  return {
    restrict: 'A',
    link,
  };

  function link(scope: IScope, elem: JQLite, attrs: IAttributes) {
    const { limitedFeatureDir: featureId } = attrs;

    if (!featureId) {
      return;
    }

    const state = selectShow(featureId);

    if (state === FeatureState.HIDDEN) {
      elem.hide();
      return;
    }

    if (state === FeatureState.VISIBLE) {
      return;
    }

    elem.hide();
  }
}
