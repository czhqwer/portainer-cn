import { History, Wifi, WifiOff, X } from 'lucide-react';
import clsx from 'clsx';
import { useTranslation } from 'react-i18next';

import { Environment } from '@/react/portainer/environments/types';
import {
  getDashboardRoute,
  isEdgeAsync as checkEdgeAsync,
} from '@/react/portainer/environments/utils';
import { isBE } from '@/react/portainer/feature-flags/feature-flags.service';

import { Icon } from '@@/Icon';
import { LinkButton } from '@@/LinkButton';
import { Button } from '@@/buttons';

type BrowseStatus = 'snapshot' | 'connected' | 'disconnected';

export function EnvironmentBrowseButtons({
  environment,
  onClickBrowse,
  onClickDisconnect,
  isActive,
}: {
  environment: Environment;
  onClickBrowse(): void;
  onClickDisconnect(): void;
  isActive: boolean;
}) {
  const { t } = useTranslation();
  const isEdgeAsync = checkEdgeAsync(environment);
  const browseStatus = getStatus(isActive, isEdgeAsync);

  const dashboardRoute = getDashboardRoute(environment);
  return (
    <div className="flex h-24 w-full flex-col justify-center gap-2 [&>*]:h-1/3">
      {isBE &&
        (browseStatus !== 'snapshot' ? (
          <LinkButton
            icon={History}
            disabled={!isEdgeAsync}
            to="edge.browse.dashboard"
            params={{
              environmentId: environment.Id,
            }}
            size="medium"
            color="light"
            className="!m-0 w-full !py-0"
            title={
              !isEdgeAsync
                ? t(
                    'legacyText.Browse snapshot is only available for async environments',
                    {
                      defaultValue:
                        'Browse snapshot is only available for async environments',
                    }
                  )
                : ''
            }
            data-cy={`browse-snapshot-link-${environment.Name}`}
          >
            {t('legacyText.Browse snapshot', {
              defaultValue: 'Browse snapshot',
            })}
          </LinkButton>
        ) : (
          <Button
            icon={X}
            data-cy={`close-snapshot-link-${environment.Name}`}
            onClick={onClickDisconnect}
            className="!m-0 w-full !py-0 opacity-60"
            size="medium"
            color="light"
          >
            {t('legacyText.Close snapshot', {
              defaultValue: 'Close snapshot',
            })}
          </Button>
        ))}

      {browseStatus !== 'connected' ? (
        <LinkButton
          title={
            isEdgeAsync
              ? t(
                  'legacyText.Live connection is not available for async environments',
                  {
                    defaultValue:
                      'Live connection is not available for async environments',
                  }
                )
              : ''
          }
          icon={Wifi}
          disabled={isEdgeAsync}
          to={dashboardRoute.to}
          params={dashboardRoute.params}
          size="medium"
          onClick={onClickBrowse}
          color="primary"
          className="!m-0 w-full !py-0"
          data-cy={`live-connect-link-${environment.Name}`}
        >
          {t('legacyText.Live connect', { defaultValue: 'Live connect' })}
        </LinkButton>
      ) : (
        <Button
          data-cy={`disconnect-link-${environment.Name}`}
          icon={WifiOff}
          onClick={onClickDisconnect}
          className="!m-0 w-full !py-0 opacity-60"
          size="medium"
          color="primary"
        >
          {t('legacyText.Disconnect', { defaultValue: 'Disconnect' })}
        </Button>
      )}

      <BrowseStatusTag status={browseStatus} />
    </div>
  );
}

function getStatus(isActive: boolean, isEdgeAsync: boolean) {
  if (!isActive) {
    return 'disconnected';
  }

  if (isEdgeAsync) {
    return 'snapshot';
  }

  return 'connected';
}

function BrowseStatusTag({ status }: { status: BrowseStatus }) {
  switch (status) {
    case 'snapshot':
      return <Snapshot />;
    case 'connected':
      return <Connected />;
    case 'disconnected':
      return <Disconnected />;
    default:
      return null;
  }
}

function Disconnected() {
  const { t } = useTranslation();

  return (
    <div className="flex items-center justify-center gap-2">
      <Icon icon={WifiOff} />
      {t('legacyText.Disconnected', { defaultValue: 'Disconnected' })}
    </div>
  );
}

function Connected() {
  const { t } = useTranslation();

  return (
    <div
      className={clsx(
        'flex items-center justify-center gap-2 rounded-lg',
        'text-green-8 th-dark:text-green-4',
        'bg-green-3 th-dark:bg-green-3/30'
      )}
    >
      <div className="h-2 w-2 rounded-full bg-green-8 th-dark:bg-green-4" />
      {t('legacyText.Connected', { defaultValue: 'Connected' })}
    </div>
  );
}

function Snapshot() {
  const { t } = useTranslation();

  return (
    <div
      className={clsx(
        'flex items-center justify-center gap-2 rounded-lg',
        'text-warning-7 th-dark:text-warning-4',
        'bg-warning-3 th-highcontrast:bg-warning-3/30 th-dark:bg-warning-3/10'
      )}
    >
      <div className="h-2 w-2 rounded-full bg-warning-7" />
      {t('legacyText.Browsing Snapshot', {
        defaultValue: 'Browsing Snapshot',
      })}
    </div>
  );
}
