import { ReactNode, useEffect, useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import type { i18n as I18n } from 'i18next';

import { Button } from '@@/buttons';

import { ButtonOptions, ModalType } from './types';
import { openModal } from './open-modal';
import { Modal, OnSubmit } from './Modal';

export interface DialogOptions<T> {
  title?: ReactNode;
  message: ReactNode;
  modalType?: ModalType;
  buttons: Array<ButtonOptions<T>>;
}

interface Props<T> extends DialogOptions<T> {
  onSubmit: OnSubmit<T>;
}

export function Dialog<T>({
  buttons,
  message,
  title,
  onSubmit,
  modalType,
}: Props<T>) {
  const { i18n } = useTranslation();
  const translatedTitle = translateLegacyNode(title, i18n);
  const translatedMessage = translateLegacyNode(message, i18n);
  const ariaLabel =
    requireString(translatedTitle) ||
    requireString(translatedMessage) ||
    'Dialog';

  const [count, setCount] = useState<number>(0);
  const countRef = useRef(count);
  countRef.current = count;

  useEffect(() => {
    let retFn;

    // only countdown the first button with non-zero timeout
    for (let i = 0; i < buttons.length; i++) {
      const button = buttons[i];
      if (button.timeout) {
        setCount(button.timeout as number);

        const intervalID = setInterval(() => {
          const count = countRef.current;

          setCount(count - 1);
          if (count === 1) {
            onSubmit(button.value);
          }
        }, 1000);

        retFn = () => clearInterval(intervalID);

        break;
      }
    }

    return retFn;
  }, [buttons, onSubmit]);

  return (
    <Modal onDismiss={() => onSubmit()} aria-label={ariaLabel}>
      {title && (
        <Modal.Header title={translatedTitle} modalType={modalType} />
      )}
      <Modal.Body>{translatedMessage}</Modal.Body>
      <Modal.Footer>
        {buttons.map((button, index) => (
          <Button
            onClick={() => onSubmit(button.value)}
            className={button.className}
            color={button.color}
            key={index}
            size="medium"
            data-cy={button.dataCy}
          >
            {translateLegacyString(button.label, i18n)}{' '}
            {button.timeout && count ? `(${count})` : null}
          </Button>
        ))}
      </Modal.Footer>
    </Modal>
  );
}

function requireString(value: ReactNode) {
  return typeof value === 'string' ? value : undefined;
}

function translateLegacyNode(value: ReactNode, i18n: I18n) {
  return typeof value === 'string' ? translateLegacyString(value, i18n) : value;
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

export async function openDialog<T>(options: DialogOptions<T>) {
  return openModal<DialogOptions<T>, T>(Dialog, options);
}
