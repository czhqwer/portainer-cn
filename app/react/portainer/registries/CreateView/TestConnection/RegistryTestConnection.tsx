import { useEffect, useState } from 'react';
import { AlertCircle, ArrowLeftRight, CheckCircle } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { LoadingButton } from '@@/buttons';
import { TextTip } from '@@/Tip/TextTip';

import { RegistryTypes } from '../../types/registry';

import { useCheckRegistryConnectionMutation } from './useCheckRegistryConnectionMutation';

interface Props {
  values: {
    Username: string;
    Password: string;
  };
  onTestSuccess: () => void;
  disabled?: boolean;
  isConnectionTested?: boolean;
}

export function RegistryTestConnection({
  values,
  onTestSuccess,
  isConnectionTested,
  disabled,
}: Props) {
  const { t } = useTranslation();
  const [testResult, setTestResult] = useState<{
    success: boolean;
    message: string;
  } | null>(null);

  useEffect(() => {
    if (!isConnectionTested) {
      setTestResult({
        success: false,
        message: t('registries.testConnection.notTested'),
      });
    }
  }, [isConnectionTested, t]);

  const pingMutation = useCheckRegistryConnectionMutation();

  return (
    <div className="flex flex-row items-center gap-3">
      <LoadingButton
        size="small"
        color="default"
        className="!ml-0 w-min"
        isLoading={pingMutation.isLoading}
        icon={ArrowLeftRight}
        loadingText="Testing connection..."
        onClick={handleTestConnection}
        disabled={disabled || !values.Username || !values.Password}
        type="button"
        data-cy="registry-test-connection-button"
      >
        Test connection
      </LoadingButton>

      {testResult && (
        <TextTip
          className="!items-start [&>svg]:mt-0.5"
          icon={testResult.success ? CheckCircle : AlertCircle}
          color={testResult.success ? 'green' : 'red'}
        >
          {testResult.message}
        </TextTip>
      )}
    </div>
  );

  async function handleTestConnection() {
    if (!values.Username || !values.Password) {
      setTestResult({
        success: false,
        message: t('registries.testConnection.requiredFields'),
      });
      return;
    }

    setTestResult(null);

    const testPayload = {
      Username: values.Username,
      Password: values.Password,
      Type: RegistryTypes.DOCKERHUB, // DockerHub registry type
    };

    pingMutation.mutate(testPayload, {
      onSuccess(response) {
        if (response.success) {
          setTestResult({
            success: true,
            message: response.message || t('registries.testConnection.success'),
          });
          onTestSuccess();
        } else {
          setTestResult({
            success: false,
            message:
              response.message || t('registries.testConnection.failure'),
          });
        }
      },
      onError() {
        setTestResult({
          success: false,
          message: t('registries.testConnection.error'),
        });
      },
    });
  }
}
