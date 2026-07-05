import i18n from '@/i18n';

export const rdWidgetTitle = {
  requires: '^rdWidget',
  bindings: {
    titleText: '@',
    icon: '@',
    classes: '@?',
    parentClasses: '@?',
  },
  transclude: {
    title: '?headerTitle',
  },
  controller: class RdWidgetTitleController {
    titleText?: string;

    translateTitle(title?: string) {
      if (!title) {
        return '';
      }

      return i18n.t('panelTitles.' + title, { defaultValue: title });
    }
  },
  template: `
    <div class="widget-header" ng-class="$ctrl.parentClasses">
      <div class="row">
        <span ng-class="$ctrl.classes" class="pull-left vertical-center">
          <div class="widget-icon space-right">
            <pr-icon icon="$ctrl.icon"></pr-icon>
          </div>
          <span ng-transclude="title">{{ $ctrl.translateTitle($ctrl.titleText) }}</span>
        </span>
        <span ng-class="$ctrl.classes" class="pull-right" ng-transclude></span>
      </div>
    </div>
`,
};
