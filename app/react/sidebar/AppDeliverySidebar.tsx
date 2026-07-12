import {
  Box,
  LayoutDashboard,
  Package,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { SidebarItem } from './SidebarItem';
import { SidebarSection } from './SidebarSection';

export function AppDeliverySidebar() {
  const { t } = useTranslation();

  return (
    <SidebarSection
      title={t('platform.navigation.section', { defaultValue: 'App Delivery' })}
    >
      <SidebarItem
        label={t('platform.navigation.home', { defaultValue: 'Overview' })}
        to="portainer.platform.home"
        icon={LayoutDashboard}
        data-cy="portainerSidebar-platform-home"
      />

      <SidebarItem
        label={t('platform.navigation.projects', { defaultValue: 'Projects' })}
        to="portainer.platform.projects"
        icon={Box}
        data-cy="portainerSidebar-platform-projects"
      />

      <SidebarItem
        label={t('platform.navigation.artifacts', {
          defaultValue: 'Artifacts',
        })}
        to="portainer.platform.artifacts"
        icon={Package}
        data-cy="portainerSidebar-platform-artifacts"
      />

    </SidebarSection>
  );
}
