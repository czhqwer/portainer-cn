import { useTranslation } from 'react-i18next';

import { Button } from '@@/buttons';
import { Modal, ModalType, OnSubmit, openModal } from '@@/modals';

interface Props {
  onSubmit: OnSubmit<boolean>;
  message: string;
  command: string;
  rowsAffected?: number;
}

function ConfirmDatabaseCommandModal({
  onSubmit,
  message,
  command,
  rowsAffected,
}: Props) {
  const { t } = useTranslation();

  return (
    <Modal
      onDismiss={() => onSubmit(false)}
      size="lg"
      aria-label="confirm database command modal"
    >
      <Modal.Header
        title={
          <span>
            {t('legacyText.Confirm database command', {
              defaultValue: 'Confirm database command',
            })}
          </span>
        }
        modalType={ModalType.Warn}
      />
      <Modal.Body>
        <p>{message}</p>

        {typeof rowsAffected === 'number' && (
          <p className="mb-4 rounded-md border border-orange-6 bg-orange-1 px-3 py-2 text-sm text-orange-10 th-dark:border-orange-8 th-dark:bg-orange-10/20 th-dark:text-orange-3">
            {t('legacyText.Affected rows', {
              defaultValue: 'Affected rows',
            })}
            : <strong>{rowsAffected}</strong>
          </p>
        )}

        <div>
          <p className="mb-2 text-sm font-medium">
            {t('legacyText.Command to execute:', {
              defaultValue: 'Command to execute:',
            })}
          </p>
          <pre className="m-0 max-h-64 overflow-auto rounded-md border border-gray-5 bg-gray-2 p-3 font-mono text-xs leading-5 text-gray-9 th-dark:bg-gray-8 th-dark:text-gray-1">
            {command}
          </pre>
        </div>
      </Modal.Body>
      <Modal.Footer>
        <Button
          color="default"
          onClick={() => onSubmit(false)}
          data-cy="database-command-cancel"
        >
          {t('buttons.Cancel', { defaultValue: 'Cancel' })}
        </Button>
        <Button
          color="primary"
          onClick={() => onSubmit(true)}
          data-cy="database-command-confirm"
        >
          {t('buttons.Run', { defaultValue: 'Run' })}
        </Button>
      </Modal.Footer>
    </Modal>
  );
}

/**
 * 数据库写操作在真正提交前必须由用户确认；该对话框沿用应用内的 Modal，
 * 既能展示预执行影响范围，也避免浏览器原生确认框截断多行 SQL/Redis 命令。
 */
export function confirmDatabaseCommand(props: Omit<Props, 'onSubmit'>) {
  return openModal(ConfirmDatabaseCommandModal, props);
}
