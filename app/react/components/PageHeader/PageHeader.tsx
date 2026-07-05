import { useRouter } from '@uirouter/react';
import { PropsWithChildren } from 'react';
import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { dispatchCacheRefreshEvent } from '@/portainer/services/http-request.helper';

import { VerticalSeparator } from '../VerticalSeparator';
import { Button } from '../buttons';

import { Breadcrumbs } from './Breadcrumbs';
import { Crumb } from './Breadcrumbs/Breadcrumbs';
import { HeaderContainer } from './HeaderContainer';
import { HeaderTitle } from './HeaderTitle';
import { PageTitle } from './PageTitle';
import { SidebarToggleButton } from './SidebarToggleButton';

interface Props {
  id?: string;
  reload?: boolean;
  loading?: boolean;
  onReload?(): Promise<void> | void;
  breadcrumbs?: (Crumb | string)[] | string;
  title?: string;
  /** Render the visible page title row. Defaults to true when title is provided.
   * Set to false on screens that display the title via another component (e.g.
   * `ResourceDetailHeader`) to avoid showing it twice. */
  showTitle?: boolean;
}

export function PageHeader({
  id,
  title,
  breadcrumbs = [],
  reload,
  loading,
  onReload,
  showTitle = !!title,
  children,
}: PropsWithChildren<Props>) {
  const router = useRouter();
  const { t } = useTranslation();
  const translatedTitle = title ? translateHeaderText(title, t) : '';

  return (
    <>
      <HeaderContainer id={id}>
        <div className="flex min-w-0 items-center">
          <SidebarToggleButton />
          <VerticalSeparator />
          <Breadcrumbs breadcrumbs={breadcrumbs} />
        </div>
        <HeaderTitle />
      </HeaderContainer>

      {showTitle && title && (
        <PageTitle title={translatedTitle}>
          {(reload || children) && (
            <div className="ml-auto flex items-center gap-2">
              {reload && (
                <Button
                  color="none"
                  size="large"
                  onClick={onClickedRefresh}
                  className="m-0 p-0 focus:text-inherit"
                  disabled={loading}
                  title={t('pageTitles.Refresh page', {
                    defaultValue: 'Refresh page',
                  })}
                  data-cy="refresh-page-button"
                >
                  <RefreshCw className="icon" />
                </Button>
              )}
              {children}
            </div>
          )}
        </PageTitle>
      )}
    </>
  );

  function onClickedRefresh() {
    dispatchCacheRefreshEvent();
    return onReload ? onReload() : router.stateService.reload();
  }
}

function translateHeaderText(
  value: string,
  t: (key: string, options: { defaultValue: string }) => string
) {
  const normalizedValue = stripAngularStringQuotes(value);
  const i18nExpressionMatch = normalizedValue.match(/^t\('(.+)'\)$/);
  if (i18nExpressionMatch) {
    return t(i18nExpressionMatch[1], { defaultValue: normalizedValue });
  }

  return t('pageTitles.' + normalizedValue, { defaultValue: normalizedValue });
}

function stripAngularStringQuotes(value: string) {
  const quoteMatch = value.match(/^['"](.+)['"]$/);
  return quoteMatch ? quoteMatch[1] : value;
}
