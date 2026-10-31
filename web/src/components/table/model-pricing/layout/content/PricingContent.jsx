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

import React from 'react';
import { Button, Empty } from '@douyinfe/semi-ui';
import PricingView from './PricingView';

const PricingContent = ({ isMobile, sidebarProps, resultsRef, ...props }) => {
  return (
    <div className='pricing-content-body' ref={resultsRef}>
      <div className='pricing-search-header'>
        <h2 className='pricing-results-title'>
          {props.t('模型列表')}
          <span className='pricing-result-count'>
            {props.filteredModels.length}
          </span>
        </h2>
      </div>

      {/* Results follow the page scroll. */}
      <div className='pricing-scroll-hide pricing-view-container'>
        {props.loadError ? (
          <div className='pricing-catalog-empty' role='alert'>
            <p>{props.t('加载失败，请重试')}</p>
            <Button onClick={props.refresh} loading={props.loading}>
              {props.t('重试')}
            </Button>
          </div>
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
