import { PropsWithChildren, ReactNode } from 'react';
import clsx from 'clsx';
import { useTranslation } from 'react-i18next';

import { Tooltip } from '@@/Tip/Tooltip';
import { InlineLoader } from '@@/InlineLoader';

import { FormError } from '../FormError';

export type Size = 'xsmall' | 'small' | 'medium' | 'large' | 'vertical';

export interface Props {
  inputId?: string;
  dataCy?: string;
  label: ReactNode;
  size?: Size;
  tooltip?: ReactNode;
  setTooltipHtmlMessage?: boolean;
  children?: ReactNode;
  errors?: ReactNode;
  required?: boolean;
  className?: string;
  isLoading?: boolean; // whether to show an inline loader, instead of the children
  loadingText?: ReactNode; // text to show when isLoading is true
}

export function FormControl({
  inputId,
  dataCy,
  label,
  size = 'small',
  tooltip = '',
  children,
  errors,
  className,
  required = false,
  setTooltipHtmlMessage,
  isLoading = false,
  loadingText = 'Loading...',
}: PropsWithChildren<Props>) {
  const { t, i18n } = useTranslation();
  const translatedLabel = translateText(label, 'formLabels', t, i18n);
  const translatedTooltip = translateText(tooltip, 'formLabels', t, i18n);
  const translatedLoadingText = translateText(loadingText, 'common', t, i18n);
  const translatedErrors = translateText(errors, 'formLabels', t, i18n);

  return (
    <div
      className={clsx(
        className,
        'form-group',
        'after:clear-both after:table after:content-[""]' // to fix issues with float
      )}
      data-cy={dataCy}
    >
      <label
        htmlFor={inputId}
        className={clsx(sizeClassLabel(size), 'control-label', 'text-left')}
      >
        {translatedLabel}

        {required && <span className="text-danger">*</span>}

        {tooltip && (
          <Tooltip
            message={translatedTooltip}
            setHtmlMessage={setTooltipHtmlMessage}
          />
        )}
      </label>

      <div className={clsx('flex flex-col', sizeClassChildren(size))}>
        {isLoading && (
          // 34px height to reduce layout shift when loading is complete
          <div className="flex h-[34px] items-center">
            <InlineLoader>{translatedLoadingText}</InlineLoader>
          </div>
        )}
        {!isLoading && children}
        {!!errors && !isLoading && <FormError>{translatedErrors}</FormError>}
      </div>
    </div>
  );
}

function sizeClassLabel(size: Size) {
  switch (size) {
    case 'large':
      return 'col-sm-5 col-lg-4';
    case 'medium':
      return 'col-sm-4 col-lg-3';
    case 'xsmall':
      return 'col-sm-1';
    case 'vertical':
      return '';
    case 'small':
    default:
      return 'col-sm-3 col-lg-2';
  }
}

function sizeClassChildren(size: Size) {
  switch (size) {
    case 'large':
      return 'col-sm-7 col-lg-8';
    case 'medium':
      return 'col-sm-8 col-lg-9';
    case 'xsmall':
      return 'col-sm-11';
    case 'vertical':
      return '';
    case 'small':
    default:
      return 'col-sm-9 col-lg-10';
  }
}

function translateText(
  value: ReactNode,
  namespace: string,
  t: (key: string, options: { defaultValue: string }) => string,
  i18n: {
    language?: string;
    resolvedLanguage?: string;
    languages?: readonly string[];
    getResourceBundle(language: string, namespace: string): unknown;
  }
) {
  return typeof value === 'string'
    ? getResourceText(namespace, value, i18n) ||
        t(namespace + '.' + value, { defaultValue: value })
    : value;
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
