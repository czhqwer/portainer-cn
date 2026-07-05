import { PropsWithChildren, ReactNode } from 'react';
import { Loader2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Icon } from '@@/Icon';

import { type Props as ButtonProps, Button } from './Button';

interface Props extends ButtonProps {
  loadingText: string;
  isLoading: boolean;
}

export function LoadingButton({
  loadingText,
  isLoading,
  disabled,
  type = 'submit',
  children,
  icon,
  ...buttonProps
}: PropsWithChildren<Props>) {
  const { t, i18n } = useTranslation();
  const translatedLoadingText =
    getResourceText('buttons', loadingText, i18n) ||
    t('buttons.' + loadingText, {
      defaultValue: loadingText,
    });

  return (
    <Button
      // eslint-disable-next-line react/jsx-props-no-spreading
      {...buttonProps}
      type={type}
      disabled={disabled || isLoading}
      icon={loadingButtonIcon(isLoading, icon)}
    >
      {isLoading ? translatedLoadingText : children}
    </Button>
  );
}

function getResourceText(
  section: string,
  key: string,
  i18n: {
    language?: string;
    resolvedLanguage?: string;
    languages?: readonly string[];
    getResourceBundle(language: string, namespace: string): unknown;
  }
) {
  const languages = [
    i18n.language,
    i18n.resolvedLanguage,
    ...(i18n.languages || []),
    'en',
  ].filter((language): language is string => !!language);

  for (const language of languages) {
    const bundle = i18n.getResourceBundle(language, 'translation') as
      | Record<string, Record<string, string>>
      | undefined;
    const value = bundle?.[section]?.[key];
    if (typeof value === 'string') {
      return value;
    }
  }

  return undefined;
}

function loadingButtonIcon(isLoading: boolean, defaultIcon: ReactNode) {
  if (!isLoading) {
    return defaultIcon;
  }
  return (
    <span className="flex items-center" role="status" aria-label="loading">
      <Icon icon={Loader2} className="ml-1 animate-spin-slow" />
    </span>
  );
}
