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
import { Select } from '@douyinfe/semi-ui';

export default function ModelPriceSettings({ data }) {
  const { t } = data;
  const priceMode = data.showWithRecharge ? data.priceConvertMode : 'base';
  return (
    <div className='model-detail-settings'>
      <label>
        <span>{t('价格模式')}</span>
        <Select
          aria-label={t('价格模式')}
          value={priceMode}
          onChange={(value) => {
            data.setShowWithRecharge(value !== 'base');
            if (value !== 'base') data.setPriceConvertMode(value);
          }}
          optionList={[
            {
              value: 'base',
              label:
                data.siteDisplayType === 'CUSTOM' ? t('自定义货币') : 'USD ($)',
            },
            { value: 'recharge', label: t('充值价格') },
            ...(data.availablePlans.length
              ? [{ value: 'package', label: t('套餐价格') }]
              : []),
          ]}
        />
      </label>
      {priceMode === 'package' && data.availablePlans.length > 0 && (
        <label>
          <span>{t('请选择套餐')}</span>
          <Select
            aria-label={t('请选择套餐')}
            value={data.selectedPlanId}
            onChange={data.setSelectedPlanId}
            optionList={data.availablePlans.map((plan) => ({
              value: plan.id,
              label: plan.title,
            }))}
          />
        </label>
      )}
    </div>
  );
}
