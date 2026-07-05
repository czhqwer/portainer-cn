import React, { MouseEvent } from 'react';
import Tippy, { type TippyProps } from '@tippyjs/react';
import clsx from 'clsx';
import _ from 'lodash';
import { useTranslation } from 'react-i18next';

import 'tippy.js/dist/tippy.css';

import { FeatureId } from '@/react/portainer/feature-flags/enums';

import styles from './TooltipWithChildren.module.css';

export type Position = 'top' | 'right' | 'bottom' | 'left';

export interface Props {
  position?: Position;
  message: React.ReactNode;
  className?: string;
  children: React.ReactElement;
  heading?: string;
  BEFeatureID?: FeatureId;
  appendTo?: TippyProps['appendTo'];
}

export function TooltipWithChildren({
  message,
  position = 'bottom',
  className,
  children,
  heading,
  appendTo,
}: Props) {
  const id = _.uniqueId('tooltip-');
  const { t } = useTranslation();

  const translatedHeading =
    typeof heading === 'string'
      ? t(`legacyText.${heading}`, { defaultValue: heading })
      : heading;
  const translatedMessage =
    typeof message === 'string'
      ? t(`legacyText.${message}`, { defaultValue: message })
      : message;

  const messageHTML = (
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
    <div
      className={styles.tooltipContainer}
      onClick={onClickHandler}
      onMouseDown={onMouseDownHandler}
    >
      {heading && (
        <div className="mb-3 inline-flex w-full justify-between">
          <span>{translatedHeading}</span>
        </div>
      )}
      <div className={styles.tooltipMessage}>{translatedMessage}</div>
    </div>
  );

  return (
    <Tippy
      className={clsx(id, styles.tooltip, className)}
      content={messageHTML}
      delay={[50, 500]} // 50ms to open, 500ms to hide
      zIndex={1000}
      placement={position}
      maxWidth={400}
      appendTo={appendTo}
      arrow
      allowHTML
      interactive
      disabled={!message}
    >
      <span>{children}</span>
    </Tippy>
  );
}

// Preventing click bubbling to the parent as it is affecting
// mainly toggles when full row is clickable.
function onClickHandler(e: MouseEvent) {
  e.stopPropagation();
}

function onMouseDownHandler(e: MouseEvent) {
  e.stopPropagation();
}
