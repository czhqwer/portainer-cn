import clsx from 'clsx';
import { PropsWithChildren, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { WidgetIcon } from './WidgetIcon';
import { useWidgetContext } from './Widget';

interface Props {
  title: ReactNode;
  icon?: ReactNode;
  className?: string;
  subtitle?: string;
}

export function WidgetTitle({
  title,
  icon,
  className,
  children,
  subtitle,
}: PropsWithChildren<Props>) {
  const { titleId } = useWidgetContext();
  const { t } = useTranslation();
  const translatedTitle =
    typeof title === 'string'
      ? t('panelTitles.' + title, { defaultValue: title })
      : title;
  const translatedSubtitle = subtitle
    ? t('panelTitles.' + subtitle, { defaultValue: subtitle })
    : subtitle;

  return (
    <div className="widget-header">
      <div className="flex items-center justify-between">
        <span className={clsx('inline-flex items-center gap-1', className)}>
          {icon && <WidgetIcon icon={icon} />}
          <h2 id={titleId} className={clsx('m-0 text-base', icon && 'ml-1')}>
            {translatedTitle}
          </h2>
        </span>
        <span className={clsx('flex items-center', className)}>{children}</span>
      </div>
      {translatedSubtitle && (
        <span className="text-muted small">{translatedSubtitle}</span>
      )}
    </div>
  );
}
