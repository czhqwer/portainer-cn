import clsx from 'clsx';
import { Menu, MenuButton, MenuList } from '@reach/menu-button';
import { PropsWithChildren, ReactNode } from 'react';
import { MoreVertical } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface Props {
  quickActions?: ReactNode;
}

export function TableSettingsMenu({
  quickActions,
  children,
}: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const settingsLabel = t('legacyText.Settings', { defaultValue: 'Settings' });

  return (
    <Menu className="setting">
      {({ isExpanded }) => (
        <>
          <MenuButton
            className={clsx('table-setting-menu-btn', {
              'setting-active': isExpanded,
            })}
            aria-label={settingsLabel}
            title={settingsLabel}
          >
            <MoreVertical
              size="13"
              className="space-right"
              strokeWidth="3px"
              aria-hidden="true"
            />
          </MenuButton>
          <MenuList>
            <div className="tableMenu">
              <div className="menuHeader">
                {t('legacyText.Table settings', {
                  defaultValue: 'Table settings',
                })}
              </div>
              <div className="menuContent">{children}</div>
              {quickActions && (
                <div>
                  <div className="menuHeader">
                    {t('legacyText.Quick actions', {
                      defaultValue: 'Quick actions',
                    })}
                  </div>
                  <div className="menuContent">{quickActions}</div>
                </div>
              )}
            </div>
          </MenuList>
        </>
      )}
    </Menu>
  );
}
