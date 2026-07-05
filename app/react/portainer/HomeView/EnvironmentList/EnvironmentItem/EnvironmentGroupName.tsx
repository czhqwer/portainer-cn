import { LayoutGrid } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export function EnvironmentGroupName({ groupName }: { groupName?: string }) {
  const { t } = useTranslation();
  const translatedGroupName = groupName
    ? t(`legacyText.${groupName}`, { defaultValue: groupName })
    : t('legacyText.Unassigned', { defaultValue: 'Unassigned' });

  return (
    <span className="small text-muted vertical-center">
      <LayoutGrid aria-hidden="true" className="icon icon-xs" />{' '}
      {t('legacyText.Group', { defaultValue: 'Group' })}: {translatedGroupName}
    </span>
  );
}
