import { Terminal } from 'lucide-react';
import clsx from 'clsx';
import { v4 as uuidv4 } from 'uuid';
import { useTranslation } from 'react-i18next';

import { EnvironmentId } from '@/react/portainer/environments/types';
import { baseHref } from '@/portainer/helpers/pathHelper';

import { Button } from '@@/buttons';

import { useSidebarState } from '../useSidebarState';
import { SidebarTooltip } from '../SidebarItem/SidebarTooltip';

interface Props {
  environmentId: EnvironmentId;
}
export function KubectlShellButton({ environmentId }: Props) {
  const { isOpen: isSidebarOpen } = useSidebarState();
  const { t } = useTranslation();
  const label = t('legacyText.kubectl shell', {
    defaultValue: 'kubectl shell',
  });

  const button = (
    <Button
      color="primary"
      size="small"
      data-cy="k8sSidebar-shellButton"
      onClick={() => handleOpen()}
      className={clsx('sidebar', !isSidebarOpen && '!p-1')}
      icon={Terminal}
    >
      {isSidebarOpen ? label : ''}
    </Button>
  );

  return (
    <>
      {!isSidebarOpen && (
        <SidebarTooltip
          content={
            <span className="whitespace-nowrap text-sm">{label}</span>
          }
        >
          <span className="flex w-full justify-center">{button}</span>
        </SidebarTooltip>
      )}
      {isSidebarOpen && button}
    </>
  );

  function handleOpen() {
    const url = window.location.origin + baseHref();
    window.open(
      `${url}#!/${environmentId}/kubernetes/kubectl-shell`,
      // give the window a unique name so that more than one can be opened
      `kubectl-shell-${environmentId}-${uuidv4()}`,
      'width=800,height=600'
    );
  }
}
