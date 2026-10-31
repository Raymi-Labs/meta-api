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
import PricingCatalogFilters from '../../layout/header/PricingCatalogFilters';
import PricingDisplaySettings from '../../filter/PricingDisplaySettings';
import PricingQuotaTypes from '../../filter/PricingQuotaTypes';
import PricingEndpointTypes from '../../filter/PricingEndpointTypes';
import { usePricingFilterCounts } from '../../../../../hooks/model-pricing/usePricingFilterCounts';

export default function FilterModalContent({ sidebarProps: props, t }) {
  const { quotaTypeModels, endpointTypeModels } = usePricingFilterCounts(props);
  return (
    <>
      <PricingCatalogFilters {...props} t={t} drawer />
      <div className='catalog-context-filter'>
        <label id='catalog-context-label'>{t('上下文长度')}</label>
        <Select
          aria-labelledby='catalog-context-label'
          value={props.filterContextLength || 0}
          onChange={props.setFilterContextLength}
          optionList={[
            { value: 0, label: t('全部') },
            ...[32000, 128000, 256000, 1000000].map((v) => ({
              value: v,
              label: `≥ ${v === 1000000 ? '1M' : v / 1000 + 'K'}`,
            })),
          ]}
        />
      </div>
      <PricingQuotaTypes {...props} t={t} models={quotaTypeModels} pills />
      <PricingEndpointTypes
        {...props}
        t={t}
        models={endpointTypeModels}
        allModels={props.models}
        pills
      />
      <PricingDisplaySettings {...props} t={t} pills />
    </>
  );
}
