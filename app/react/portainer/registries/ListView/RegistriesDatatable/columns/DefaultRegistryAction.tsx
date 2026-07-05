import { Eye, EyeOff } from 'lucide-react';

import { notifySuccess } from '@/portainer/services/notifications';
import { FeatureId } from '@/react/portainer/feature-flags/enums';
import { isLimitedToBE } from '@/react/portainer/feature-flags/feature-flags.service';
import {
  usePublicSettings,
  useUpdateDefaultRegistrySettingsMutation,
} from '@/react/portainer/settings/queries';

import { Button } from '@@/buttons';

export function DefaultRegistryAction() {
  const settingsQuery = usePublicSettings({
    select: (settings) => settings.DefaultRegistry?.Hide,
  });
  const defaultRegistryMutation = useUpdateDefaultRegistrySettingsMutation();

  if (!settingsQuery.isSuccess) {
    return null;
  }
  const hideDefaultRegistry = settingsQuery.data;

  const isLimited = isLimitedToBE(FeatureId.HIDE_DOCKER_HUB_ANONYMOUS);
  if (isLimited) {
    return null;
  }

  return (
    <>
      {!hideDefaultRegistry ? (
        <div className="vertical-center">
          <Button
            color="danger"
            data-cy="hide-default-registry-button"
            icon={EyeOff}
            onClick={() => handleShowOrHide(true)}
          >
            Hide for all users
          </Button>
        </div>
      ) : (
        <div className="vertical-center">
          <Button
            data-cy="show-default-registry-button"
            icon={Eye}
            onClick={() => handleShowOrHide(false)}
          >
            Show for all users
          </Button>
        </div>
      )}
    </>
  );

  function handleShowOrHide(hideDefaultRegistry: boolean) {
    defaultRegistryMutation.mutate(
      {
        Hide: hideDefaultRegistry,
      },
      {
        onSuccess() {
          notifySuccess(
            'Success',
            'Default registry Settings updated successfully'
          );
        },
      }
    );
  }
}
