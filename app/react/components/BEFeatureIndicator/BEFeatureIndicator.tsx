import { ReactNode } from 'react';

import { FeatureId } from '@/react/portainer/feature-flags/enums';

import { getFeatureDetails } from './utils';

export interface Props {
  featureId: FeatureId;
  showIcon?: boolean;
  className?: string;
  children?: (isLimited: boolean) => ReactNode;
}

export function BEFeatureIndicator({
  featureId,
  children = () => null,
}: Props) {
  const { limitedToBE = false } = getFeatureDetails(featureId);

  if (limitedToBE) {
    return null;
  }

  return <>{children(false)}</>;
}
