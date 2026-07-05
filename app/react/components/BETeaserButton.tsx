import { FeatureId } from '@/react/portainer/feature-flags/enums';
import { AutomationTestingProps } from '@/types';

interface Props extends AutomationTestingProps {
  featureId: FeatureId;
  heading: string;
  message: string;
  buttonText: string;
  className?: string;
  buttonClassName?: string;
}

export function BETeaserButton(_props: Props) {
  return null;
}
