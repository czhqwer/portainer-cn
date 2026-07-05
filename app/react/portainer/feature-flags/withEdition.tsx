import { ComponentType } from 'react';

import { hideCommercialFeatures } from './feature-flags.service';

export function withEdition<T>(
  WrappedComponent: ComponentType<T>,
  edition: 'BE' | 'CE'
): ComponentType<T> {
  // Try to create a nice displayName for React Dev Tools.
  const displayName =
    WrappedComponent.displayName || WrappedComponent.name || 'Component';

  function WrapperComponent(props: T & JSX.IntrinsicAttributes) {
    if (hideCommercialFeatures && edition === 'BE') {
      return null;
    }

    if (process.env.PORTAINER_EDITION !== edition) {
      return null;
    }

    return <WrappedComponent {...props} />;
  }

  WrapperComponent.displayName = `with${edition}Edition(${displayName})`;

  return WrapperComponent;
}
