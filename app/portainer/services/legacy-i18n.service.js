import i18n from '@/i18n';

const TRANSLATABLE_ATTRIBUTES = ['placeholder', 'title', 'aria-label', 'uib-tooltip'];
const TEXT_SKIP_TAGS = new Set(['CODE', 'PRE', 'SCRIPT', 'STYLE', 'TEXTAREA']);

/* @ngInject */
export function LegacyI18n($timeout) {
  let observer;

  return {
    start,
  };

  function start() {
    translateSoon();

    i18n.on('initialized', translateSoon);
    i18n.on('languageChanged', translateSoon);
    i18n.on('loaded', translateSoon);

    observer = new MutationObserver((mutations) => {
      if (mutations.some(shouldTranslateMutation)) {
        translateSoon();
      }
    });

    observer.observe(document.body, {
      attributes: true,
      attributeFilter: TRANSLATABLE_ATTRIBUTES,
      characterData: true,
      childList: true,
      subtree: true,
    });

    window.addEventListener('beforeunload', stop);
  }

  function stop() {
    observer?.disconnect();
    i18n.off('initialized', translateSoon);
    i18n.off('languageChanged', translateSoon);
    i18n.off('loaded', translateSoon);
    window.removeEventListener('beforeunload', stop);
  }

  function translateSoon() {
    $timeout(translatePage, 0, false);
  }

  function translatePage() {
    translateTextNodes(document.body);
    translateAttributes(document.body);
  }
}

function shouldTranslateMutation(mutation) {
  return mutation.type === 'childList' || mutation.type === 'characterData' || TRANSLATABLE_ATTRIBUTES.includes(mutation.attributeName);
}

function translateTextNodes(root) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.nodeValue || !node.nodeValue.trim()) {
        return NodeFilter.FILTER_REJECT;
      }

      const parent = node.parentElement;
      if (!parent || shouldSkipTextElement(parent)) {
        return NodeFilter.FILTER_REJECT;
      }

      return NodeFilter.FILTER_ACCEPT;
    },
  });

  let node = walker.nextNode();
  while (node) {
    const nextNode = walker.nextNode();
    translateTextNode(node);
    node = nextNode;
  }
}

function translateTextNode(node) {
  const original = getOriginalText(node);
  const translated = translateLegacyText(original);
  const currentText = node.nodeValue.trim();

  if (currentText !== translated) {
    node.__legacyI18nText = original;
    node.nodeValue = preserveSpacing(node.nodeValue, translated);
  }
}

function translateAttributes(root) {
  root.querySelectorAll(TRANSLATABLE_ATTRIBUTES.map((attr) => `[${attr}]`).join(',')).forEach((element) => {
    if (shouldSkipAttributeElement(element)) {
      return;
    }

    TRANSLATABLE_ATTRIBUTES.forEach((attribute) => {
      const value = element.getAttribute(attribute);
      if (!value || value.includes('{{')) {
        return;
      }

      const original = element.dataset[`legacyI18n${toDatasetKey(attribute)}`] || value;
      const translated = translateLegacyText(original);

      if (translated !== value) {
        element.dataset[`legacyI18n${toDatasetKey(attribute)}`] = original;
        element.setAttribute(attribute, translated);
      }
    });
  });
}

function getOriginalText(node) {
  return node.__legacyI18nText || node.nodeValue.trim();
}

function translateLegacyText(text) {
  const normalizedText = normalizeWhitespace(text);
  const i18nExpressionMatch = normalizedText.match(/^t\('(.+)'\)$/);
  if (i18nExpressionMatch) {
    return i18n.t(i18nExpressionMatch[1], { defaultValue: normalizedText });
  }

  const quotedTextMatch = normalizedText.match(/^['"](.+)['"]$/);
  if (quotedTextMatch) {
    return translateLegacyText(quotedTextMatch[1]);
  }

  const exactMatch = getResourceText('legacyText', normalizedText) || normalizedText;
  if (exactMatch !== normalizedText) {
    return exactMatch;
  }

  return translateDynamicLegacyText(normalizedText);
}

function getResourceText(section, key) {
  const languages = [
    i18n.language,
    i18n.resolvedLanguage,
    ...(i18n.languages || []),
    'en',
  ].filter(Boolean);

  for (const language of languages) {
    const bundle = i18n.getResourceBundle(language, 'translation');
    const value = bundle?.[section]?.[key];
    if (typeof value === 'string') {
      return value;
    }
  }

  return undefined;
}

function translateDynamicLegacyText(text) {
  const statusCountMatch = text.match(/^(\d+)\s+(running|stopped|healthy|unhealthy)$/i);
  if (statusCountMatch) {
    return i18n.t('legacyText.{{count}} {{status}}', {
      count: statusCountMatch[1],
      status: i18n.t(`legacyText.${statusCountMatch[2].toLowerCase()}`, {
        defaultValue: statusCountMatch[2],
      }),
      defaultValue: text,
    });
  }

  const showingMatch = text.match(/^Showing\s+(.+)\s+of\s+(.+)$/i);
  if (showingMatch) {
    return i18n.t('legacyText.Showing {{range}} of {{total}}', {
      range: showingMatch[1],
      total: showingMatch[2],
      defaultValue: text,
    });
  }

  const groupMatch = text.match(/^Group:\s+(.+)$/i);
  if (groupMatch) {
    return i18n.t('legacyText.Group: {{group}}', {
      group: i18n.t(`legacyText.${groupMatch[1]}`, {
        defaultValue: groupMatch[1],
      }),
      defaultValue: text,
    });
  }

  const exitedMatch = text.match(/^exited\s+-\s+code\s+(\d+)$/i);
  if (exitedMatch) {
    return i18n.t('legacyText.exited - code {{code}}', {
      code: exitedMatch[1],
      defaultValue: text,
    });
  }

  const durationMatch = text.match(/^(\d+)\s+(second|seconds|minute|minutes|hour|hours|day|days|week|weeks|month|months|year|years)$/i);
  if (durationMatch) {
    return translateLegacyDuration(durationMatch[0]);
  }

  const durationWithForMatch = text.match(/^for\s+(.+)$/i);
  if (durationWithForMatch) {
    return i18n.t('legacyText.for {{duration}}', {
      duration: translateLegacyDuration(durationWithForMatch[1]),
      defaultValue: text,
    });
  }

  const stoppedWithExitCodeMatch = text.match(/^Stopped(?:\s+for)?\s+(.+)\s+with\s+exit\s+code\s+(\d+)$/i);
  if (stoppedWithExitCodeMatch) {
    return i18n.t('legacyText.Stopped {{duration}} with exit code {{code}}', {
      duration: translateLegacyDuration(stoppedWithExitCodeMatch[1]),
      code: stoppedWithExitCodeMatch[2],
      defaultValue: text,
    });
  }

  const withExitCodeMatch = text.match(/^with\s+exit\s+code\s+(\d+)$/i);
  if (withExitCodeMatch) {
    return i18n.t('legacyText.with exit code {{code}}', {
      code: withExitCodeMatch[1],
      defaultValue: text,
    });
  }

  const containerWaitingMatch = text.match(/^Container is waiting:\s+(.+)$/i);
  if (containerWaitingMatch) {
    return i18n.t('legacyText.Container is waiting: {{reason}}', {
      reason: containerWaitingMatch[1],
      defaultValue: text,
    });
  }

  const containerExitedMatch = text.match(/^Container exited with code\s+(\d+)(?:\s+\(restarted\s+(\d+)\s+times?\))?\.\s*(.*)$/i);
  if (containerExitedMatch) {
    const key = containerExitedMatch[2]
      ? 'legacyText.Container exited with code {{code}} (restarted {{count}} times). {{details}}'
      : 'legacyText.Container exited with code {{code}}. {{details}}';
    return i18n.t(key, {
      code: containerExitedMatch[1],
      count: Number(containerExitedMatch[2] || 0),
      details: translateLegacyText(containerExitedMatch[3]),
      defaultValue: text,
    });
  }

  const containerTerminatedMatch = text.match(/^Container terminated:\s+(.+?)(?:\s+\(restarted\s+(\d+)\s+times?\))?\.\s*(.*)$/i);
  if (containerTerminatedMatch) {
    const key = containerTerminatedMatch[2]
      ? 'legacyText.Container terminated: {{reason}} (restarted {{count}} times). {{details}}'
      : 'legacyText.Container terminated: {{reason}}. {{details}}';
    return i18n.t(key, {
      reason: containerTerminatedMatch[1],
      count: Number(containerTerminatedMatch[2] || 0),
      details: translateLegacyText(containerTerminatedMatch[3]),
      defaultValue: text,
    });
  }

  const crashLoopWithDetailsMatch = text.match(/^Container keeps crashing after startup\. Check application logs and startup configuration\.\s+Details:\s+'(.+)'$/i);
  if (crashLoopWithDetailsMatch) {
    return i18n.t('legacyText.Container keeps crashing after startup. Check application logs and startup configuration. Details: {{details}}', {
      details: crashLoopWithDetailsMatch[1],
      defaultValue: text,
    });
  }

  const statusWithRestartMatch = text.match(/^(.+)\s+\(Restarted\s+(\d+)\s+times?\)$/i);
  if (statusWithRestartMatch) {
    const count = Number(statusWithRestartMatch[2]);
    return `${translateLegacyText(statusWithRestartMatch[1])} (${i18n.t('legacyText.Restarted {{count}} times', {
      count,
      defaultValue: statusWithRestartMatch[0],
    })})`;
  }

  const deletePodMatch = text.match(/^Delete pod\s+(.+)$/i);
  if (deletePodMatch) {
    return `${i18n.t('legacyText.Delete pod', { defaultValue: 'Delete pod' })} ${deletePodMatch[1]}`;
  }

  const publicAccessMatch = text.match(/^I want any user with access to this\s+(.+)\s+to be able to manage this\s+(.+)$/i);
  if (publicAccessMatch) {
    return i18n.t('legacyText.I want any user with access to this {{resource}} to be able to manage this {{target}}', {
      resource: translateLegacyText(publicAccessMatch[1]),
      target: translateLegacyText(publicAccessMatch[2]),
      defaultValue: text,
    });
  }

  const restrictToAdminsMatch = text.match(/^I want to restrict the management of this\s+(.+)\s+to administrators only$/i);
  if (restrictToAdminsMatch) {
    return i18n.t('legacyText.I want to restrict the management of this {{resource}} to administrators only', {
      resource: translateLegacyText(restrictToAdminsMatch[1]),
      defaultValue: text,
    });
  }

  const restrictToUsersTeamsMatch = text.match(/^I want to restrict the management of this\s+(.+)\s+to a set of users and\/or teams$/i);
  if (restrictToUsersTeamsMatch) {
    return i18n.t('legacyText.I want to restrict the management of this {{resource}} to a set of users and/or teams', {
      resource: translateLegacyText(restrictToUsersTeamsMatch[1]),
      defaultValue: text,
    });
  }

  const restrictToSelfMatch = text.match(/^I want to restrict this\s+(.+)\s+to be manageable by myself only$/i);
  if (restrictToSelfMatch) {
    return i18n.t('legacyText.I want to restrict this {{resource}} to be manageable by myself only', {
      resource: translateLegacyText(restrictToSelfMatch[1]),
      defaultValue: text,
    });
  }

  const agoMatch = text.match(/^(.+)\s+ago$/i);
  if (agoMatch) {
    return i18n.t('legacyText.{{duration}} ago', {
      duration: translateLegacyDuration(agoMatch[1]),
      defaultValue: text,
    });
  }

  const prefixWithDetailsMatch = text.match(/^([^:]+):\s+(.+)$/);
  if (prefixWithDetailsMatch) {
    const translatedPrefix = getResourceText('legacyText', prefixWithDetailsMatch[1]);
    if (translatedPrefix) {
      return i18n.t('legacyText.{{prefix}}: {{details}}', {
        prefix: translatedPrefix,
        details: translateLegacyText(prefixWithDetailsMatch[2]),
        defaultValue: text,
      });
    }
  }

  return text;
}

function translateLegacyDuration(duration) {
  const durationMatch = duration.match(/^(\d+)\s+(sec|second|seconds|min|minute|minutes|hour|hours|day|days|week|weeks|month|months|year|years)$/i);
  if (!durationMatch) {
    return duration;
  }

  const unit = durationMatch[2].toLowerCase();
  const unitKey = unit.endsWith('s') ? unit.slice(0, -1) : unit;
  const normalizedUnitKey = {
    sec: 'second',
    min: 'minute',
  }[unitKey] || unitKey;

  return i18n.t(`legacyText.duration.${normalizedUnitKey}`, {
    count: Number(durationMatch[1]),
    defaultValue: duration,
  });
}

function normalizeWhitespace(text) {
  return text.replace(/\s+/g, ' ').trim();
}

function preserveSpacing(currentValue, translated) {
  const leading = currentValue.match(/^\s*/)?.[0] || '';
  const trailing = currentValue.match(/\s*$/)?.[0] || '';
  return `${leading}${translated}${trailing}`;
}

function shouldSkipTextElement(element) {
  return TEXT_SKIP_TAGS.has(element.tagName) || element.closest('[data-legacy-i18n-skip], code, pre, script, style, textarea');
}

function shouldSkipAttributeElement(element) {
  return element.closest('[data-legacy-i18n-skip], code, pre, script, style');
}

function toDatasetKey(attribute) {
  return attribute
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join('');
}
