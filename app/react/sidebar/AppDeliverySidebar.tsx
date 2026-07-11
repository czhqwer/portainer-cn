import {
  Box,
  Database,
  GitBranch,
  History,
  Layers,
  Package,
  Rocket,
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
        label={t('platform.navigation.projects', { defaultValue: 'Projects' })}
        to="portainer.platform.projects"
        icon={Box}
        data-cy="portainerSidebar-platform-projects"
      />

      <SidebarItem
        label={t('platform.navigation.applications', {
          defaultValue: 'Applications',
        })}
        to="portainer.platform.applications"
        icon={Layers}
        data-cy="portainerSidebar-platform-applications"
      />

      <SidebarItem
        label={t('platform.navigation.deploy', { defaultValue: 'Deploy' })}
        to="portainer.platform.deploy"
        icon={Rocket}
        data-cy="portainerSidebar-platform-deploy"
      />

      <SidebarItem
        label={t('platform.navigation.artifacts', {
          defaultValue: 'Artifacts',
        })}
        to="portainer.platform.artifacts"
        icon={Package}
        data-cy="portainerSidebar-platform-artifacts"
      />

      <SidebarItem
        label={t('platform.navigation.releases', { defaultValue: 'Releases' })}
        to="portainer.platform.releases"
        icon={History}
        data-cy="portainerSidebar-platform-releases"
      />

      <SidebarItem
        label={t('platform.navigation.gitopsWorkflows', {
          defaultValue: 'GitOps Workflows',
        })}
        to="portainer.gitops.workflows"
        icon={GitBranch}
        data-cy="portainerSidebar-workflows"
      />

      <SidebarItem
        label={t('platform.navigation.gitopsSources', {
          defaultValue: 'GitOps Sources',
        })}
        to="portainer.gitops.sources"
        icon={Database}
        data-cy="portainerSidebar-sources"
      />
    </SidebarSection>
  );
}
