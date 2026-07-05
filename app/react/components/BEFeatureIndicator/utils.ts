import { FeatureId } from '@/react/portainer/feature-flags/enums';
import { isLimitedToBE } from '@/react/portainer/feature-flags/feature-flags.service';

export function getFeatureDetails(featureId?: FeatureId) {
  if (!featureId) {
    return {};
  }

  const limitedToBE = isLimitedToBE(featureId);

  return { url: '', limitedToBE };
}
