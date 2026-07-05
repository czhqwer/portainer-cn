import { notifyError, notifySuccess } from '@/portainer/services/notifications';
import { userQueryKeys } from '@/portainer/users/queries/queryKeys';
import { queryClient } from '@/react-tools/react-query';
import { options } from '@/react/portainer/account/AccountView/theme-options';
import i18n from '@/i18n';

export default class ThemeSettingsController {
  /* @ngInject */
  constructor($async, Authentication, ThemeManager, StateManager, UserService) {
    this.$async = $async;
    this.Authentication = Authentication;
    this.ThemeManager = ThemeManager;
    this.StateManager = StateManager;
    this.UserService = UserService;

    this.setThemeColor = this.setThemeColor.bind(this);
    this.t = i18n.t.bind(i18n);
  }

  async setThemeColor(color) {
    return this.$async(async () => {
      if (color === 'auto' || !color) {
        this.ThemeManager.autoTheme();
      } else {
        this.ThemeManager.setTheme(color);
      }

      this.state.themeColor = color;
      this.updateThemeSettings({ color });
    });
  }

  async updateThemeSettings(theme) {
    try {
      await this.UserService.updateUserTheme(this.state.userId, theme);
      await queryClient.invalidateQueries(userQueryKeys.user(this.state.userId));

      notifySuccess(i18n.t('common.success'), i18n.t('account.theme.updatedMessage'));
    } catch (err) {
      notifyError(i18n.t('common.failure'), err, i18n.t('account.theme.updateError'));
    }
  }

  $onInit() {
    return this.$async(async () => {
      this.state = {
        userId: null,
        themeColor: 'auto',
      };

      this.state.availableThemes = options;

      try {
        this.state.userId = await this.Authentication.getUserDetails().ID;
        const user = await this.UserService.user(this.state.userId);

        this.state.themeColor = user.ThemeSettings.color || this.state.themeColor;
      } catch (err) {
        notifyError(i18n.t('common.failure'), err, i18n.t('account.theme.userDetailsError'));
      }
    });
  }
}
