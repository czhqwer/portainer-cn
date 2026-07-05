import {
  Menu,
  MenuButton,
  MenuList,
  MenuLink as ReachMenuLink,
} from '@reach/menu-button';
import { UISrefProps, useSref } from '@uirouter/react';
import clsx from 'clsx';
import { UserIcon, ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { queryClient } from '@/react-tools/react-query';
import { AutomationTestingProps } from '@/types';
import { useCurrentUser } from '@/react/hooks/useUser';

import styles from './HeaderTitle.module.css';
import { ThemeSelector } from './UserMenuThemeSelector';

export function UserMenu() {
  const { user } = useCurrentUser();
  const { t } = useTranslation();

  return (
    <Menu>
      <MenuButton
        className={clsx(
          'ml-auto flex items-center gap-1 self-start',
          styles.menuButton
        )}
        data-cy="userMenu-button"
        aria-label={t('legacyText.User menu toggle', {
          defaultValue: 'User menu toggle',
        })}
      >
        <div
          className={clsx(
            styles.menuIcon,
            'icon-badge mr-1 !p-2 text-lg',
            'text-gray-8',
            'th-dark:text-gray-warm-7'
          )}
        >
          <UserIcon className="lucide" />
        </div>
        {user && <span>{user.Username}</span>}
        <ChevronDown className={styles.arrowDown} />
      </MenuButton>

      <MenuList
        className={styles.menuList}
        aria-label={t('legacyText.User Menu', { defaultValue: 'User Menu' })}
        data-cy="userMenu"
      >
        <MenuLink
          to="portainer.account"
          label="My account"
          data-cy="userMenu-myAccount"
        />

        <MenuLink
          to="portainer.logout"
          label="Log out"
          data-cy="userMenu-logOut"
        />

        <hr className="my-1 border-t border-gray-5 th-highcontrast:border-gray-7 th-dark:border-gray-7" />

        <ThemeSelector user={user} />
      </MenuList>
    </Menu>
  );
}

interface MenuLinkProps extends AutomationTestingProps, UISrefProps {
  label: string;
}

function MenuLink({
  to,
  label,
  params,
  options,
  'data-cy': dataCy,
}: MenuLinkProps) {
  const anchorProps = useSref(to, params, options);
  const { t } = useTranslation();
  const translatedLabel = t(`legacyText.${label}`, { defaultValue: label });

  return (
    <ReachMenuLink
      href={anchorProps.href}
      onClick={(e: React.MouseEvent) => {
        queryClient.clear();
        anchorProps.onClick(e);
      }}
      className={styles.menuLink}
      aria-label={translatedLabel}
      data-cy={dataCy}
    >
      {translatedLabel}
    </ReachMenuLink>
  );
}
