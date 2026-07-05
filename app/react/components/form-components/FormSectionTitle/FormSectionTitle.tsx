import clsx from 'clsx';
import { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
  htmlFor?: string;
  titleSize?: 'sm' | 'md' | 'lg';
  className?: string;
  id?: string;
}

const tailwindTitleSize = {
  sm: 'text-sm',
  md: 'text-base',
  lg: 'text-lg',
};

export function FormSectionTitle({
  children,
  htmlFor,
  titleSize = 'md',
  className,
  id,
}: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const translatedChildren =
    typeof children === 'string'
      ? t('panelTitles.' + children, { defaultValue: children })
      : children;

  if (htmlFor) {
    return (
      <label
        htmlFor={htmlFor}
        className={clsx(
          'col-sm-12 mb-2 mt-1 flex cursor-pointer items-center pl-0 font-medium',
          tailwindTitleSize[titleSize],
          className
        )}
        id={id}
      >
        {translatedChildren}
      </label>
    );
  }
  return (
    <div
      className={clsx(
        'col-sm-12 mb-2 mt-4 pl-0 font-medium',
        tailwindTitleSize[titleSize],
        className
      )}
      id={id}
    >
      {translatedChildren}
    </div>
  );
}
