import { useRouter } from '@uirouter/react';
import { useTranslation } from 'react-i18next';

import { humanize } from '@/portainer/filters/filters';
import { EnvironmentId } from '@/react/portainer/environments/types';

import { Widget } from '@@/Widget/Widget';
import { WidgetBody } from '@@/Widget/WidgetBody';
import { WidgetTitle } from '@@/Widget/WidgetTitle';
import { DetailsTable } from '@@/DetailsTable';
import { Tooltip } from '@@/Tip/Tooltip';

import { DockerStorageInfo } from '../DockerStorageInfo';

type HostOs = {
  type: string;
  arch: string;
  name: string;
};

type HostInfo = {
  name: string;
  os?: HostOs;
  kernelVersion?: string;
  totalCPU: number;
  totalMemory: number;
};

type Props = {
  host: HostInfo;
  isBrowseEnabled: boolean;
  browseUrl: string;
  endpointId?: EnvironmentId;
};

export function HostDetailsPanel({
  host,
  isBrowseEnabled,
  browseUrl,
  endpointId,
}: Props) {
  const router = useRouter();
  const { t } = useTranslation();

  return (
    <div className="row">
      <div className="col-lg-12 col-md-12 col-sm-12 col-xs-12">
        <Widget>
          <WidgetTitle title="Host Details" icon="code" />
          <WidgetBody className="no-padding">
            <DetailsTable dataCy="host-details" className="!mb-0">
              <DetailsTable.Row label="Hostname">{host.name}</DetailsTable.Row>
              {host.os && (
                <DetailsTable.Row label="OS Information">
                  {host.os.type} {host.os.arch} {host.os.name}
                </DetailsTable.Row>
              )}
              {host.kernelVersion && (
                <DetailsTable.Row label="Kernel Version">
                  {host.kernelVersion}
                </DetailsTable.Row>
              )}
              <DetailsTable.Row label="Total CPU">
                {host.totalCPU}
              </DetailsTable.Row>
              <DetailsTable.Row label="Total memory">
                {humanize(host.totalMemory)}
              </DetailsTable.Row>
              {endpointId && (
                <DetailsTable.Row
                  label={
                    <span className="flex items-center">
                      {t('legacyText.Disk usage', {
                        defaultValue: 'Disk usage',
                      })}
                      <Tooltip
                        message={t(
                          "legacyText.Disk usage on the partition backing Docker's data directory. Docker usage includes images, container layers, volumes, and build cache.",
                          {
                            defaultValue:
                              "Disk usage on the partition backing Docker's data directory. Docker usage includes images, container layers, volumes, and build cache.",
                          }
                        )}
                      />
                    </span>
                  }
                >
                  <DockerStorageInfo endpointId={endpointId} />
                </DetailsTable.Row>
              )}
              {isBrowseEnabled && (
                <tr>
                  <td colSpan={2}>
                    <button
                      type="button"
                      className="btn btn-primary btn-sm"
                      title={t('legacyText.Browse', {
                        defaultValue: 'Browse',
                      })}
                      onClick={() => router.stateService.go(browseUrl)}
                    >
                      {t('legacyText.Browse', { defaultValue: 'Browse' })}
                    </button>
                  </td>
                </tr>
              )}
            </DetailsTable>
          </WidgetBody>
        </Widget>
      </div>
    </div>
  );
}
