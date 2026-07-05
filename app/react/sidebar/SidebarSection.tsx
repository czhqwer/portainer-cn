import { PropsWithChildren, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { useSidebarState } from './useSidebarState';

interface Props {
  title: ReactNode;
  hoverText?: string;
  showTitleWhenOpen?: boolean;
  'aria-label'?: string;
}

export function SidebarSection({
  title,
  hoverText,
  children,
  showTitleWhenOpen,
  'aria-label': ariaLabel,
}: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const translatedTitle =
    typeof title === 'string'
      ? t(`legacyText.${title}`, { defaultValue: title })
      : title;

  return (
    <div>
      <SidebarSectionTitle
        showWhenOpen={showTitleWhenOpen}
        hoverText={hoverText}
      >
        {translatedTitle}
      </SidebarSectionTitle>

      <nav
        aria-label={
          typeof title === 'string' ? String(translatedTitle) : ariaLabel
        }
        className="mt-4"
      >
        <ul>{children}</ul>
      </nav>
    </div>
  );
}

interface TitleProps {
  showWhenOpen?: boolean;
  hoverText?: string;
}

export function SidebarSectionTitle({
  showWhenOpen,
  hoverText,
  children,
}: PropsWithChildren<TitleProps>) {
  const { isOpen } = useSidebarState();

  if (!isOpen && !showWhenOpen) {
    return null;
  }

  return (
    <li
      title={hoverText}
      className="ml-3 text-sm text-gray-3 transition-all duration-500 ease-in-out be:text-gray-6"
    >
      {children}
    </li>
  );
}
