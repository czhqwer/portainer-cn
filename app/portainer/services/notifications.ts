import _ from 'lodash';
import toastr from 'toastr';
import sanitize from 'sanitize-html';
import { v4 as uuid } from 'uuid';

import { get as localStorageGet } from '@/react/hooks/useLocalStorage';
import { notificationsStore } from '@/react/portainer/notifications/notifications-store';
import { ToastNotification } from '@/react/portainer/notifications/types';
import i18n from '@/i18n';

const { addNotification } = notificationsStore.getState();

toastr.options = {
  timeOut: 3000,
  closeButton: true,
  progressBar: true,
  tapToDismiss: false,
  escapeHtml: true,
  // custom button, using the lucide icon x.svg inside
  closeHtml: `<button type="button"><svg
  xmlns="http://www.w3.org/2000/svg"
  width="18"
  height="18"
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
>
  <line x1="18" y1="6" x2="6" y2="18" />
  <line x1="6" y1="6" x2="18" y2="18" />
</svg></button>`,
};

export function notifySuccess(title: string, text: string) {
  const translatedTitle = translateNotificationText(title);
  const translatedText = translateNotificationText(text);
  saveNotification(translatedTitle, translatedText, 'success');
  toastr.success(
    sanitize(_.escape(translatedText)),
    sanitize(_.escape(translatedTitle))
  );
}

export function notifyWarning(title: string, text: string) {
  const translatedTitle = translateNotificationText(title);
  const translatedText = translateNotificationText(text);
  saveNotification(translatedTitle, translatedText, 'warning');
  toastr.warning(sanitize(_.escape(translatedText)), sanitize(translatedTitle), {
    timeOut: 6000,
  });
}

export function notifyError(title: string, e?: unknown, fallbackText = '') {
  const msg = translateNotificationText(pickErrorMsg(e) || fallbackText);
  const translatedTitle = translateNotificationText(title);
  saveNotification(translatedTitle, msg, 'error');

  if (!_.get(e, 'resource.password')) {
    // eslint-disable-next-line no-console
    console.error(e);
  }

  if (msg !== 'Invalid JWT token') {
    toastr.error(sanitize(_.escape(msg)), sanitize(translatedTitle), {
      timeOut: 6000,
    });
  }
}

export const success = notifySuccess;
export const error = notifyError;
export const warning = notifyWarning;

/* @ngInject */
export function Notifications() {
  return {
    success: notifySuccess,
    warning: notifyWarning,
    error: notifyError,
  };
}

function pickErrorMsg(e?: unknown) {
  if (!e) {
    return '';
  }

  const props = [
    'err.data.details',
    'err.data.message',
    'data.details',
    'data.message',
    'data.content',
    'data.error',
    'message',
    'err.data[0].message',
    'err.data.err',
    'data.err',
    'msg',
  ];

  let msg = '';

  props.forEach((prop) => {
    const val = _.get(e, prop);
    if (typeof val === 'string') {
      msg = msg || val;
    }
  });

  return msg;
}

function saveNotification(title: string, text: string, type: string) {
  const notif: ToastNotification = {
    id: uuid(),
    title,
    details: text,
    type,
    timeStamp: new Date(),
  };
  const userId = localStorageGet('USER_ID', '');
  if (userId !== '') {
    addNotification(userId, notif);
  }
}

export function translateNotificationText(text: string): string {
  if (!text) {
    return text;
  }

  const stackStatusMatch = text.match(/^Stack (.+) (started|stopped) successfully$/);
  if (stackStatusMatch) {
    return translateInterpolatedNotificationText(
      'Stack {{name}} {{action}} successfully',
      {
        name: stackStatusMatch[1],
        action: translateNotificationText(stackStatusMatch[2]),
      },
      text
    );
  }

  const unableToRemoveStackMatch = text.match(/^Unable to remove stack (.+)$/);
  if (unableToRemoveStackMatch) {
    return translateInterpolatedNotificationText(
      'Unable to remove stack {{name}}',
      {
        name: unableToRemoveStackMatch[1],
      },
      text
    );
  }

  const connectedContainerMatch = text.match(/^Connected container to (.+)$/);
  if (connectedContainerMatch) {
    return translateInterpolatedNotificationText(
      'Connected container to {{network}}',
      {
        network: connectedContainerMatch[1],
      },
      text
    );
  }

  const reclaimedMatch = text.match(/^Reclaimed (.+)$/);
  if (reclaimedMatch) {
    return translateInterpolatedNotificationText(
      'Reclaimed {{size}}',
      {
        size: reclaimedMatch[1],
      },
      text
    );
  }

  const replicaCountMatch = text.match(/^New replica count:\s+(.+)$/);
  if (replicaCountMatch) {
    return translateInterpolatedNotificationText(
      'New replica count: {{count}}',
      {
        count: replicaCountMatch[1],
      },
      text
    );
  }

  const failedRemoveEnvironmentMatch = text.match(/^Failed to remove environment (.+)$/);
  if (failedRemoveEnvironmentMatch) {
    return translateInterpolatedNotificationText(
      'Failed to remove environment {{name}}',
      {
        name: failedRemoveEnvironmentMatch[1],
      },
      text
    );
  }

  const createdNamedResourceMatch = text.match(/^(Tag|Group) (.+) created$/);
  if (createdNamedResourceMatch) {
    return translateInterpolatedNotificationText(
      '{{resource}} {{name}} created',
      {
        resource: translateNotificationText(createdNamedResourceMatch[1]),
        name: createdNamedResourceMatch[2],
      },
      text
    );
  }

  const podDeletedMatch = text.match(/^Pod '(.+)' deleted$/);
  if (podDeletedMatch) {
    return translateInterpolatedNotificationText(
      "Pod '{{name}}' deleted",
      {
        name: podDeletedMatch[1],
      },
      text
    );
  }

  const failedRemoveStackMatch = text.match(/^Failed to remove stack '(.+)'$/);
  if (failedRemoveStackMatch) {
    return translateInterpolatedNotificationText(
      "Failed to remove stack '{{name}}'",
      {
        name: failedRemoveStackMatch[1],
      },
      text
    );
  }

  const exactMatch = getLegacyText(text);
  if (exactMatch) {
    return exactMatch;
  }

  const prefixWithTargetMatch = text.match(/^([^:]+):(.+)$/);
  if (prefixWithTargetMatch) {
    const translatedPrefix = getLegacyText(prefixWithTargetMatch[1]);
    if (translatedPrefix) {
      return `${translatedPrefix}: ${translateNotificationText(prefixWithTargetMatch[2].trim())}`;
    }
  }

  return text;
}

function translateInterpolatedNotificationText(
  key: string,
  values: Record<string, string>,
  defaultValue: string
): string {
  const template = getLegacyText(key);
  if (!template) {
    return defaultValue;
  }

  return Object.entries(values).reduce(
    (text, [name, value]) => text.replace(`{{${name}}}`, value),
    template
  );
}

function getLegacyText(key: string): string | undefined {
  const languages = [
    i18n.language,
    i18n.resolvedLanguage,
    ...(i18n.languages || []),
    'en',
  ].filter(Boolean);

  for (const language of languages) {
    const bundle = i18n.getResourceBundle(language, 'translation');
    const value = bundle?.legacyText?.[key];
    if (typeof value === 'string') {
      return value;
    }
  }

  return undefined;
}
