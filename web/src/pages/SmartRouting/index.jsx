import React from 'react';
import { useTranslation } from 'react-i18next';
import { TabPane, Tabs } from '@douyinfe/semi-ui';
import RouteInsightTab from './RouteInsightTab';
import RoutePlansTab from './RoutePlansTab';

export default function SmartRouting() {
  const { t } = useTranslation();
  return (
    <div className='p-4 md:p-6'>
      <Tabs type='line' lazyRender keepDOM={false}>
        <TabPane tab={t('线路观测')} itemKey='insight'>
          <div style={{ paddingTop: 12 }}>
            <RouteInsightTab />
          </div>
        </TabPane>
        <TabPane tab={t('路由方案')} itemKey='plans'>
          <div style={{ paddingTop: 12 }}>
            <RoutePlansTab />
          </div>
        </TabPane>
      </Tabs>
    </div>
  );
}
