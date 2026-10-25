/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useState } from 'react';
import { Tabs, TabPane, Button, Empty } from '@douyinfe/semi-ui';
import PricingTopSection from '../header/PricingTopSection';
import PricingView from './PricingView';
import GroupHealthOverview from '../../view/GroupHealthOverview';

const PricingContent = ({ isMobile, sidebarProps, resultsRef, ...props }) => {
  const [activeTab, setActiveTab] = useState('models');
  return (
    <div className='pricing-content-body' ref={resultsRef}>
      {/* Search, display controls and existing health tab. */}
      <div className='pricing-search-header'>
        <PricingTopSection
          {...props}
          isMobile={isMobile}
          sidebarProps={sidebarProps}
          showWithRecharge={sidebarProps.showWithRecharge}
          setShowWithRecharge={sidebarProps.setShowWithRecharge}
          priceConvertMode={sidebarProps.priceConvertMode}
          setPriceConvertMode={sidebarProps.setPriceConvertMode}
          subscriptionPlans={sidebarProps.subscriptionPlans}
          availablePlans={sidebarProps.availablePlans}
          selectedPlanId={sidebarProps.selectedPlanId}
          setSelectedPlanId={sidebarProps.setSelectedPlanId}
          currency={sidebarProps.currency}
          setCurrency={sidebarProps.setCurrency}
          showRatio={sidebarProps.showRatio}
          setShowRatio={sidebarProps.setShowRatio}
          viewMode={sidebarProps.viewMode}
          setViewMode={sidebarProps.setViewMode}
          tokenUnit={sidebarProps.tokenUnit}
          setTokenUnit={sidebarProps.setTokenUnit}
        />
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          type='line'
          size='small'
        >
          <TabPane
            tab={
              <span>
                {props.t('模型列表')}{' '}
                <span className='pricing-result-count'>
                  {props.filteredModels.length}
                </span>
              </span>
            }
            itemKey='models'
          />
          <TabPane tab={props.t('分组健康')} itemKey='health' />
        </Tabs>
      </div>

      {/* Results follow the page scroll; the group sidebar scrolls independently. */}
      <div className='pricing-scroll-hide pricing-view-container'>
        {props.loadError ? (
          <div className='pricing-catalog-empty' role='alert'>
            <p>{props.t('加载失败，请重试')}</p>
            <Button onClick={props.refresh} loading={props.loading}>
              {props.t('重试')}
            </Button>
          </div>
        ) : activeTab === 'health' ? (
          <GroupHealthOverview {...props} />
        ) : !props.loading && props.filteredModels.length === 0 ? (
          <div className='pricing-catalog-empty'>
            <Empty description={props.t('搜索无结果')} />
            <Button onClick={props.resetCatalogFilters}>
              {props.t('重置')}
            </Button>
          </div>
        ) : (
          <PricingView {...props} viewMode={sidebarProps.viewMode} />
        )}
      </div>
    </div>
  );
};

export default PricingContent;
