import clsx from 'clsx';

import { Link } from '@@/Link';

import portainerIcon from './portainer-p-icon-white.svg';
import { useSidebarState } from './useSidebarState';
import styles from './Header.module.css';

interface Props {
  logo?: string;
}

export function Header({ logo: customLogo }: Props) {
  const { isOpen } = useSidebarState();

  return (
    <div
      className={clsx('flex w-full flex-wrap', {
        'justify-center pr-5': !isOpen,
      })}
    >
      <Link
        to="portainer.home"
        data-cy="portainerSidebar-homeImage"
        className="text-2xl text-white no-underline hover:text-white hover:no-underline focus:text-white focus:no-underline focus:outline-none"
      >
        <Logo customLogo={customLogo} isOpen={isOpen} />
      </Link>
    </div>
  );
}

function getLogo(isOpen: boolean, customLogo?: string) {
  if (customLogo) {
    return customLogo;
  }

  if (!isOpen) {
    return portainerIcon;
  }

  return undefined;
}

function Logo({
  customLogo,
  isOpen,
}: {
  customLogo?: string;
  isOpen: boolean;
}) {
  const logo = getLogo(isOpen, customLogo);

  if (isOpen && !logo) {
    return (
      <div className="leading-none">
        <div className="text-[31px] font-black tracking-normal text-white">
          PORTAINER.CN
        </div>
        <div className="mt-1 flex items-center gap-1 text-[12px] font-bold uppercase tracking-normal text-white">
          <span className="h-2 w-2 bg-pink-5" />
          DEPLOY
        </div>
      </div>
    );
  }

  return (
    <img
      src={logo}
      className={clsx('img-responsive', styles.logo, {
        '!max-h-[27px]': !isOpen,
      })}
      alt="Logo"
    />
  );
}
