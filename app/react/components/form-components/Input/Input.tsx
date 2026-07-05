import clsx from 'clsx';
import { forwardRef, InputHTMLAttributes, Ref } from 'react';
import { useTranslation } from 'react-i18next';

import { AutomationTestingProps } from '@/types';

export const InputWithRef = forwardRef<
  HTMLInputElement,
  InputHTMLAttributes<HTMLInputElement> & AutomationTestingProps
>(
  // eslint-disable-next-line react/jsx-props-no-spreading
  (props, ref) => <Input {...props} mRef={ref} />
);

InputWithRef.displayName = 'InputWithRef';

export function Input({
  className,
  mRef: ref,
  value,
  type,
  'data-cy': dataCy,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & {
  mRef?: Ref<HTMLInputElement>;
} & AutomationTestingProps) {
  const { t } = useTranslation();
  const translatedPlaceholder =
    typeof props.placeholder === 'string'
      ? t('placeholders.' + props.placeholder, {
          defaultValue: props.placeholder,
        })
      : props.placeholder;

  return (
    <input
      // eslint-disable-next-line react/jsx-props-no-spreading
      {...props}
      placeholder={translatedPlaceholder}
      type={type}
      value={type === 'number' && Number.isNaN(value) ? '' : value} // avoid the `"NaN" cannot be parsed, or is out of range.` error for an empty number input
      ref={ref}
      className={clsx('form-control', className)}
      data-cy={dataCy}
    />
  );
}
