import { ComponentType, PropsWithChildren, ReactNode } from 'react';
import clsx from 'clsx';
import { useTranslation } from 'react-i18next';

import { Icon } from '@@/Icon';

interface Props {
  icon?: ReactNode | ComponentType<unknown>;
  label: ReactNode;
  description?: ReactNode;
  className?: string;
  id?: string;
}

export function TableTitle({
  icon,
  label,
  children,
  description,
  className,
  id,
}: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const translatedLabel =
    typeof label === 'string'
      ? t('panelTitles.' + label, { defaultValue: label })
      : label;
  const translatedDescription =
    typeof description === 'string'
      ? t('panelTitles.' + description, { defaultValue: description })
      : description;

  return (
    <>
      <div className={clsx('toolBar flex-col', className)} id={id}>
        <div className="flex w-full items-center gap-1 p-0">
          <h2 className="toolBarTitle m-0 text-base">
            {icon && (
              <div className="widget-icon">
                <Icon icon={icon} className="space-right" />
              </div>
            )}

            {translatedLabel}
          </h2>
          {children}
        </div>
      </div>
      {!!translatedDescription && (
        <div className="toolBar !pt-0">{translatedDescription}</div>
      )}
    </>
  );
}
