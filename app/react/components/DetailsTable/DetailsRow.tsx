import clsx from 'clsx';
import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
  children: ReactNode;
  label: ReactNode;
  colClassName?: string;
  className?: string;
  columns?: Array<ReactNode>;
  ariaLabel?: string;
}

export function DetailsRow({
  label,
  children,
  colClassName,
  className,
  columns,
  ariaLabel,
}: Props) {
  const { t } = useTranslation();
  const labelString = typeof label === 'string' ? label : undefined;
  const translatedLabelString = labelString
    ? t(`legacyText.${labelString}`, { defaultValue: labelString })
    : undefined;
  const translatedLabel = translatedLabelString ?? label;

  return (
    <tr
      className={className}
      aria-label={ariaLabel ?? translatedLabelString ?? labelString}
    >
      <td className={clsx(colClassName, '!break-normal')}>
        {translatedLabel}
      </td>
      <td
        className={colClassName}
        data-cy={`detailsTable-${labelString ?? 'custom'}Value`}
      >
        {children}
      </td>
      {columns?.map((column, index) => (
        <td key={index} className={colClassName}>
          {column}
        </td>
      ))}
    </tr>
  );
}
