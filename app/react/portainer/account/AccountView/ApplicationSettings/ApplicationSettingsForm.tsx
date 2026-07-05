import { Form, Formik } from 'formik';
import { useTranslation } from 'react-i18next';

import { useCurrentUser } from '@/react/hooks/useUser';
import { notifySuccess } from '@/portainer/services/notifications';
import { updateAxiosAdapter } from '@/portainer/services/axios/axios';
import { withError } from '@/react-tools/react-query';

import { TextTip } from '@@/Tip/TextTip';
import { LoadingButton } from '@@/buttons';
import { SwitchField } from '@@/form-components/SwitchField';

import { useUpdateUserMutation } from '../../useUpdateUserMutation';

type FormValues = {
  useCache: boolean;
};

export function ApplicationSettingsForm() {
  const { user } = useCurrentUser();
  const updateSettingsMutation = useUpdateUserMutation();
  const { t } = useTranslation();

  const initialValues = {
    useCache: user.UseCache,
  };

  return (
    <Formik<FormValues>
      initialValues={initialValues}
      onSubmit={handleSubmit}
      validateOnMount
      enableReinitialize
    >
      {({ isValid, dirty, values, setFieldValue }) => (
        <Form className="form-horizontal">
          <TextTip color="orange" className="mb-3">
            {t('account.applicationSettings.cacheDescription')}
          </TextTip>
          <SwitchField
            label={t('account.applicationSettings.cacheLabel')}
            data-cy="account-applicationSettingsUseCacheSwitch"
            checked={values.useCache}
            onChange={(value) => setFieldValue('useCache', value)}
            labelClass="col-lg-2 col-sm-3" // match the label width of the other fields in the page
            fieldClass="!mb-4"
          />
          <div className="form-group">
            <div className="col-sm-12">
              <LoadingButton
                loadingText={t('common.saving')}
                isLoading={updateSettingsMutation.isLoading}
                disabled={!isValid || !dirty}
                className="!ml-0"
                data-cy="account-applicationSettingsSaveButton"
              >
                {t('common.save')}
              </LoadingButton>
            </div>
          </div>
        </Form>
      )}
    </Formik>
  );

  function handleSubmit(values: FormValues) {
    updateSettingsMutation.mutate(
      {
        useCache: values.useCache,
      },
      {
        onSuccess() {
          updateAxiosAdapter(values.useCache);
          notifySuccess(
            t('common.success'),
            t('account.applicationSettings.updatedMessage')
          );
          // a full reload is required to update the angular $http cache setting
          setTimeout(() => window.location.reload(), 2000); // allow 2s to show the success notification
        },
        ...withError(t('account.applicationSettings.updateError')),
      }
    );
  }
}
