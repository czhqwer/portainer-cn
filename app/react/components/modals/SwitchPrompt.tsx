import { ReactNode, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { i18n as I18n } from 'i18next';

import { AutomationTestingProps } from '@/types';

import { SwitchField } from '@@/form-components/SwitchField';

import { ModalType, type ButtonOptions } from './types';
import { openModal } from './open-modal';
import { OnSubmit } from './Modal/types';
import { Dialog } from './Dialog';
import { buildCancelButton, buildConfirmButton } from './utils';

function SwitchPrompt({
  onSubmit,
  title,
  confirmButton = buildConfirmButton('OK'),
  switchLabel,
  modalType,
  message,
  defaultValue = false,
  'data-cy': dataCy,
}: {
  onSubmit: OnSubmit<{ value: boolean }>;
  title: string;
  switchLabel: string;
  confirmButton?: ButtonOptions<true>;
  modalType?: ModalType;
  message?: ReactNode;
  defaultValue?: boolean;
} & AutomationTestingProps) {
  const { i18n } = useTranslation();
  const [value, setValue] = useState(defaultValue);
  const translatedMessage =
    typeof message === 'string' ? translateLegacyString(message, i18n) : message;

  return (
    <Dialog
      modalType={modalType}
      title={title}
      message={
        <>
          {translatedMessage && (
            <div className="mb-3">{translatedMessage}</div>
          )}
          <SwitchField
            name="value"
            data-cy={dataCy}
            label={switchLabel}
            checked={value}
            onChange={setValue}
          />
        </>
      }
      onSubmit={(confirm) => onSubmit(confirm ? { value } : undefined)}
      buttons={[buildCancelButton(), confirmButton]}
    />
  );
}

function translateLegacyString(text: string, i18n: I18n) {
  const languages = [
    i18n.language,
    i18n.resolvedLanguage,
    ...(i18n.languages || []),
    'en',
  ].filter(Boolean);

  for (const language of languages) {
    const bundle = i18n.getResourceBundle(language, 'translation');
    const value = bundle?.legacyText?.[text];
    if (typeof value === 'string') {
      return value;
    }
  }

  return text;
}

export async function openSwitchPrompt(
  title: string,
  switchLabel: string,
  {
    confirmButton,
    modalType,
    message,
    defaultValue,
    'data-cy': dataCy,
  }: {
    confirmButton?: ButtonOptions<true>;
    modalType?: ModalType;
    message?: ReactNode;
    defaultValue?: boolean;
  } & AutomationTestingProps = {
    'data-cy': 'switch-prompt',
  }
) {
  return openModal(SwitchPrompt, {
    confirmButton,
    title,
    switchLabel,
    modalType,
    message,
    defaultValue,
    'data-cy': dataCy,
  });
}
