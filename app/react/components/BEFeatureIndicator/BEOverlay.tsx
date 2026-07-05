import { FeatureId } from '@/react/portainer/feature-flags/enums';
import { isLimitedToBE } from '@/react/portainer/feature-flags/feature-flags.service';

export function BEOverlay({
  featureId,
  children,
}: {
  featureId: FeatureId;
  children: React.ReactNode;
  variant?: 'form-section' | 'widget' | 'multi-widget';
}) {
  const isLimited = isLimitedToBE(featureId);
  if (isLimited) {
    return null;
  }

  return <>{children}</>;
}
