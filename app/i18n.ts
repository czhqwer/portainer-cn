import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import Backend from 'i18next-http-backend';

const savedLanguage =
  typeof window !== 'undefined'
    ? window.localStorage.getItem('i18nextLng')
    : undefined;

i18n
  .use(Backend)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    debug: false,
    lng: savedLanguage || 'zh-CN',
    fallbackLng: 'en',
    supportedLngs: ['en', 'zh-CN'],
    load: 'currentOnly',
    backend: {
      loadPath: 'locales/{{lng}}/translation.json?v=portainer-cn-20260705-63',
    },
    detection: {
      order: ['localStorage'],
      caches: ['localStorage'],
    },
    interpolation: {
      escapeValue: false,
    },
    react: {
      useSuspense: false,
    },
  });

export default i18n;
