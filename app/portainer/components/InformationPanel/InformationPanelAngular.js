import i18n from '@/i18n';

export const InformationPanelAngular = {
  templateUrl: './InformationPanelAngular.html',
  bindings: {
    titleText: '@',
    dismissAction: '&?',
  },
  transclude: true,
  controller: class InformationPanelAngularController {
    translateTitle(title) {
      if (!title) {
        return '';
      }

      return i18n.t('panelTitles.' + title, { defaultValue: title });
    }

    t(key) {
      return i18n.t(key);
    }
  },
};
