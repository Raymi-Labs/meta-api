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
import PricingFilterModal from '../../modal/PricingFilterModal';
import SearchActions from './SearchActions';

const PricingTopSection = ({ sidebarProps, ...props }) => {
  const [showFilterModal, setShowFilterModal] = useState(false);
  const { filterGroup, t } = sidebarProps;
  return (
    <>
      <div className='pricing-section-heading'>
        <h1>{t('模型广场')}</h1>
        <span
          className='pricing-current-vendor'
          title={sidebarProps.filterVendor}
        >
          {sidebarProps.filterVendor === 'all'
            ? t('全部供应商')
            : sidebarProps.filterVendor === 'unknown'
              ? t('未知供应商')
              : sidebarProps.filterVendor}
        </span>
        <span className='pricing-title-count'>
          {t('共 {{count}} 个模型', {
            count: props.filteredModels?.length ?? 0,
          })}
        </span>
      </div>
      <SearchActions {...props} setShowFilterModal={setShowFilterModal} />
      <p className='pricing-price-context' aria-live='polite'>
        {filterGroup === 'all'
          ? t('全部分组：每个模型展示其最低适用倍率价格')
          : t('当前分组：{{group}} · 价格已包含分组倍率', {
              group: filterGroup,
            })}
      </p>
      <PricingFilterModal
        visible={showFilterModal}
        onClose={() => setShowFilterModal(false)}
        sidebarProps={sidebarProps}
        t={t}
      />
    </>
  );
};

export default PricingTopSection;
