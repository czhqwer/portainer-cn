import { useTranslation } from 'react-i18next';

interface Props {
  label: string;
  onClear: () => void;
}

export function FilterBarActiveIndicator({ label, onClear }: Props) {
  const { t } = useTranslation();
  const translatedLabel = t(`legacyText.${label}`, { defaultValue: label });

  return (
    <div
      role="status"
      className="flex items-center gap-4 whitespace-nowrap bg-gray-2 px-5 th-highcontrast:bg-transparent th-dark:bg-gray-iron-10"
      data-cy="active-filter-indicator"
    >
      <span className="text-sm text-[var(--text-muted-color)]">
        {t('legacyText.Showing', { defaultValue: 'Showing' })}:{' '}
        <span className="font-semibold text-[var(--text-summary-color)]">
          {translatedLabel}
        </span>
      </span>
      <button
        type="button"
        className="cursor-pointer border-0 bg-transparent p-0 text-xl font-bold leading-none text-[var(--button-close-color)] opacity-[var(--button-opacity)] hover:opacity-[var(--button-opacity-hover)]"
        onClick={onClear}
        aria-label={t('legacyText.Clear filter', {
          defaultValue: 'Clear filter',
        })}
        data-cy="clear-filter-button"
      >
        &times;
      </button>
    </div>
  );
}
