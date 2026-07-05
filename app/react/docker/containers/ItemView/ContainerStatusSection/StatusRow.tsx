import { HeartPulseIcon } from 'lucide-react';
import { formatDistanceToNow, parseISO } from 'date-fns';
import { zhCN } from 'date-fns/locale';
import { useTranslation } from 'react-i18next';

import { ContainerDetailsViewModel } from '@/docker/models/containerDetails';

import { Icon } from '@@/Icon';

export function StatusRow({
  container,
}: {
  container: ContainerDetailsViewModel;
}) {
  const { i18n, t } = useTranslation();
  const isRunning = container.State?.Running || false;
  const isCreated = container.State?.Status === 'created';
  const activityTime = calculateActivityTime(container, i18n.language);
  const stateText = t(`legacyText.${getStateText(container.State)}`, {
    defaultValue: getStateText(container.State),
  });

  return (
    <div className="flex items-center gap-2">
      <Icon
        icon={HeartPulseIcon}
        mode={getIconColor(container.State)}
        className="lucide mr-1"
      />
      {stateText}{' '}
      {t('legacyText.for {{duration}}', {
        duration: activityTime,
        defaultValue: `for ${activityTime}`,
      })}
      {!isRunning && !isCreated && (
        <span>
          {' '}
          {t('legacyText.with exit code {{code}}', {
            code: container.State?.ExitCode,
            defaultValue: `with exit code ${container.State?.ExitCode}`,
          })}
        </span>
      )}
    </div>
  );
}

function getIconColor(
  state: ContainerDetailsViewModel['State']
): 'success' | 'danger' | undefined {
  if (!state) {
    return undefined;
  }

  if (state.Running) {
    return 'success';
  }

  if (state.Status !== 'created') {
    return 'danger';
  }

  return undefined;
}

function getStateText(state: ContainerDetailsViewModel['State']): string {
  if (state === undefined) {
    return '';
  }

  if (state.Dead) {
    return 'Dead';
  }

  if ('Ghost' in state && state.Ghost && state.Running) {
    return 'Ghost';
  }

  if (state.Running && state.Paused) {
    return 'Running (Paused)';
  }

  if (state.Running) {
    return 'Running';
  }

  if (state.Status === 'created') {
    return 'Created';
  }

  return 'Stopped';
}

function calculateActivityTime(
  container: ContainerDetailsViewModel,
  language: string
): string {
  if (!container.State) {
    return '';
  }

  if (container.State.Running && container.State.StartedAt) {
    return formatActivityDistance(container.State.StartedAt, language);
  }

  if (container.State.Status === 'created' && container.Created) {
    return formatActivityDistance(container.Created, language);
  }

  if (container.State.FinishedAt) {
    return formatActivityDistance(container.State.FinishedAt, language);
  }

  return '';
}

function formatActivityDistance(date: string, language: string): string {
  return formatDistanceToNow(parseISO(date), {
    locale: language === 'zh-CN' ? zhCN : undefined,
  });
}
