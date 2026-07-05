import { Languages } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { notifySuccess } from '@/portainer/services/notifications';

import { Widget, WidgetBody, WidgetTitle } from '@@/Widget';
import { LoadingButton } from '@@/buttons';
import { FormControl } from '@@/form-components/FormControl';
import { Select } from '@@/form-components/Input/Select';

const languageOptions = [
  { value: 'en', label: 'English' },
  { value: 'zh-CN', label: '简体中文' },
] as const;

type Language = (typeof languageOptions)[number]['value'];

export function LanguageSettingsWidget() {
  const { i18n, t } = useTranslation();
  const [language, setLanguage] = useState<Language>(getCurrentLanguage());
  const [isSaving, setIsSaving] = useState(false);

  return (
    <div className="row">
      <div className="col-sm-12">
        <Widget>
          <WidgetTitle
            icon={Languages}
            title={t('account.languageSettings.title')}
          />
          <WidgetBody>
            <form className="form-horizontal" onSubmit={handleSubmit}>
              <FormControl
                inputId="account-language"
                label={t('account.languageSettings.languageLabel')}
              >
                <Select
                  id="account-language"
                  value={language}
                  options={languageOptions}
                  data-cy="account-language-select"
                  onChange={(event) =>
                    setLanguage(event.target.value as Language)
                  }
                />
                <p className="small text-muted mt-2">
                  {t('account.languageSettings.description')}
                </p>
              </FormControl>

              <div className="form-group">
                <div className="col-sm-12">
                  <LoadingButton
                    loadingText={t('common.saving')}
                    isLoading={isSaving}
                    disabled={language === getCurrentLanguage()}
                    className="!ml-0"
                    data-cy="account-language-save-button"
                  >
                    {t('common.save')}
                  </LoadingButton>
                </div>
              </div>
            </form>
          </WidgetBody>
        </Widget>
      </div>
    </div>
  );

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    setIsSaving(true);
    await i18n.changeLanguage(language);
    notifySuccess(
      t('common.success'),
      t('account.languageSettings.updatedMessage')
    );

    setTimeout(() => window.location.reload(), 500);
  }

  function getCurrentLanguage(): Language {
    return i18n.resolvedLanguage === 'zh-CN' || i18n.language === 'zh-CN'
      ? 'zh-CN'
      : 'en';
  }
}
