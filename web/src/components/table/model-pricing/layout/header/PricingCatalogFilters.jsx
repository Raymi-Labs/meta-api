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
import { X } from 'lucide-react';
import PricingVendors from '../../filter/PricingVendors';
import PricingQuotaTypes from '../../filter/PricingQuotaTypes';
import PricingTags from '../../filter/PricingTags';
import { usePricingFilterCounts } from '../../../../../hooks/model-pricing/usePricingFilterCounts';

export default function PricingCatalogFilters(props) {
  const { t } = props;
  const { quotaTypeModels, tagModels } = usePricingFilterCounts(props);
  const selected = [
    {
      value: props.filterVendor,
      label: t('供应商'),
      text:
        props.filterVendor === 'unknown' ? t('未知供应商') : props.filterVendor,
      clear: props.setFilterVendor,
    },
    {
      value: props.filterQuotaType,
      label: t('计费类型'),
      text: t(
        props.filterQuotaType === 0
          ? '按量计费'
          : props.filterQuotaType === 1
            ? '按次计费'
            : '按小时计费',
      ),
      clear: props.setFilterQuotaType,
    },
    {
      value: props.filterTag,
      label: t('标签'),
      text: props.filterTag,
      clear: props.setFilterTag,
    },
    {
      value: props.filterEndpointType,
      label: t('端点类型'),
      text: props.filterEndpointType,
      clear: props.setFilterEndpointType,
    },
  ].filter(({ value }) => value != null && value !== 'all');
  return (
    <section className='pricing-catalog-filters' aria-label={t('筛选')}>
      <div className='pricing-catalog-vendors'>
        <span className='pricing-facet-label'>{t('供应商')}</span>
        <PricingVendors {...props} allModels={props.models} horizontal />
      </div>
      <PricingQuotaTypes
        {...props}
        models={quotaTypeModels}
        pills
        hideUnavailable
      />
      {props.models.some((model) => model.tags) && (
        <PricingTags
          {...props}
          models={tagModels}
          allModels={props.models}
          pills
          hideUnavailable
        />
      )}
      {selected.length > 0 && (
        <div className='pricing-active-filters' aria-label={t('筛选')}>
          {selected.map(({ label, text, clear }) => (
            <button
              key={label}
              type='button'
              onClick={() => clear('all')}
              aria-label={`${t('清除')} · ${label}: ${text}`}
            >
              <span>
                {label}: {text}
              </span>
              <X size={13} aria-hidden='true' />
            </button>
          ))}
        </div>
      )}
    </section>
  );
}
