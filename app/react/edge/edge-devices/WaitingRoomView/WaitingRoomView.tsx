import { withLimitToBE } from '@/react/hooks/useLimitToBE';

import { InformationPanel } from '@@/InformationPanel';
import { TextTip } from '@@/Tip/TextTip';
import { PageHeader } from '@@/PageHeader';
import { Link } from '@@/Link';

import { Datatable } from './Datatable';

export default withLimitToBE(WaitingRoomView);

function WaitingRoomView() {
  return (
    <>
      <PageHeader
        title="Waiting Room"
        breadcrumbs={[{ label: 'Waiting Room' }]}
        reload
      />

      <div className="row">
        <div className="col-sm-12">
          <InformationPanel>
            <TextTip color="blue">
              Only environments generated from the{' '}
              <Link
                to="portainer.endpoints.edgeAutoCreateScript"
                data-cy="waitingRoom-edgeAutoCreateScriptLink"
              >
                auto onboarding
              </Link>{' '}
              script will appear here, manually added environments and edge
              devices will bypass the waiting room.
            </TextTip>
          </InformationPanel>
        </div>
      </div>

      <Datatable />
    </>
  );
}
