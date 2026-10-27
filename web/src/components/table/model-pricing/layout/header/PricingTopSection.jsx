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
      <SearchActions {...props} setShowFilterModal={setShowFilterModal} />
      <div className='pricing-catalog-context'>
        <p className='pricing-price-context' aria-live='polite'>
          {filterGroup === 'all'
            ? t(
                '参考价：按最低适用分组展示，实际费用取决于所用分组及计费规则。',
              )
            : t('当前分组：{{group}} · 价格已包含分组倍率', {
                group: filterGroup,
              })}
        </p>
        <span className='pricing-price-mode'>
          {props.showWithRecharge
            ? props.priceConvertMode === 'package'
              ? t('套餐价格')
              : t('充值价格')
            : props.currency}{' '}
        </span>
      </div>
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
