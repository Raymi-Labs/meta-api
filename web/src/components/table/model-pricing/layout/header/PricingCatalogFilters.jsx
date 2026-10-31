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
import { getLobeHubIcon } from '../../../../../helpers/render';
import { sortPricingVendors } from '../../../../../helpers/pricingVendorSelection';
import { getModelProtocolLabel } from '../../../../../helpers/modelProtocols';
import {
  MODEL_FUNCTION_LABELS,
  CATALOG_MODALITIES,
  getModelFunctions,
  getCatalogTags,
  catalogTagLabel,
  catalogVendor,
  filterValues,
  normalizeCatalogTag,
  matchesCatalogFilters,
} from '../../../../../helpers/modelCatalogFacets';
import CatalogFacetRow from '../../filter/CatalogFacetRow';

export default function PricingCatalogFilters(props) {
  const { models = [], t, drawer = false } = props;
  const vendors = sortPricingVendors([
    ...new Set(models.map((m) => catalogVendor(m.vendor_name))),
  ]);
  const dimensions = [
    {
      key: 'vendor',
      title: t('供应商'),
      value: filterValues(props.filterVendor).map(catalogVendor),
      set: props.setFilterVendor,
      options: vendors,
      get: (m) => [catalogVendor(m.vendor_name)],
      label: (v) => (v === 'unknown' ? t('未知供应商') : v),
      icon: (v) =>
        getLobeHubIcon(
          models.find((m) => catalogVendor(m.vendor_name) === v)?.vendor_icon,
          16,
        ),
    },
    {
      key: 'function',
      title: t('功能'),
      value: props.filterFunctions,
      set: props.setFilterFunctions,
      options: Object.keys(MODEL_FUNCTION_LABELS).filter((k) =>
        models.some((m) => getModelFunctions(m).includes(k)),
      ),
      get: getModelFunctions,
      label: (k) => t(MODEL_FUNCTION_LABELS[k] || k),
    },
    {
      key: 'tag',
      title: t('标签'),
      value: filterValues(props.filterTag).map(normalizeCatalogTag),
      set: props.setFilterTag,
      options: [...new Set(models.flatMap(getCatalogTags))].sort(),
      get: getCatalogTags,
      label: (k) => catalogTagLabel(k, t, models),
      all: true,
    },
    {
      key: 'input',
      title: t('输入模态'),
      value: props.filterInputModalities,
      set: props.setFilterInputModalities,
      options: Object.keys(CATALOG_MODALITIES).filter((k) =>
        models.some((m) => m.input_modalities?.includes(k)),
      ),
      get: (m) => m.input_modalities || [],
      label: (k) => t(CATALOG_MODALITIES[k] || k),
      all: true,
    },
  ];
  const chips = dimensions.flatMap((d) =>
    filterValues(d.value).map((value) => ({
      key: d.key + value,
      label: d.title,
      text: d.label(value),
      clear: () => d.set(filterValues(d.value).filter((x) => x !== value)),
    })),
  );
  if (props.filterQuotaType !== 'all' && props.filterQuotaType != null)
    chips.push({
      key: 'quota',
      label: t('计费类型'),
      text: t(
        props.filterQuotaType === 0
          ? '按量计费'
          : props.filterQuotaType === 1
            ? '按次计费'
            : '按小时计费',
      ),
      clear: () => props.setFilterQuotaType('all'),
    });
  if (props.filterEndpointType && props.filterEndpointType !== 'all')
    chips.push({
      key: 'endpoint',
      label: t('已声明协议'),
      text: getModelProtocolLabel(props.filterEndpointType),
      clear: () => props.setFilterEndpointType('all'),
    });
  if (props.filterContextLength)
    chips.push({
      key: 'context',
      label: t('上下文长度'),
      text: `≥ ${props.filterContextLength >= 1000000 ? '1M' : props.filterContextLength / 1000 + 'K'}`,
      clear: () => props.setFilterContextLength(0),
    });
  return (
    <section
      className={`pricing-catalog-filters catalog-four-facets${drawer ? ' catalog-facets-drawer' : ''}`}
      aria-label={t('筛选')}
    >
      <div className='catalog-facet-rows'>
        {dimensions.map((d) => {
          const base = models.filter((m) =>
            matchesCatalogFilters(m, props, [d.key]),
          );
          const selected = filterValues(d.value);
          const options = [...new Set([...d.options, ...selected])];
          const items = options.map((value) => ({
            value,
            label: d.label(value),
            icon: d.icon?.(value),
            count: base.filter(
              (m) =>
                d.get(m).includes(value) &&
                (!d.all ||
                  selected
                    .filter((x) => x !== value)
                    .every((x) => d.get(m).includes(x))),
            ).length,
          }));
          return (
            <CatalogFacetRow
              key={d.key}
              dimension={d.key}
              title={d.title}
              items={items}
              total={base.length}
              value={d.value}
              onChange={d.set}
              loading={props.loading}
              t={t}
            />
          );
        })}
      </div>
      {!!chips.length && (
        <div className='pricing-active-filters' aria-label={t('已选条件')}>
          {chips.map((c) => (
            <button
              type='button'
              key={c.key}
              onClick={c.clear}
              aria-label={`${t('清除')} · ${c.label}: ${c.text}`}
            >
              <span>
                {c.label}: {c.text}
              </span>
              <X size={13} />
            </button>
          ))}
          <button type='button' onClick={props.resetCatalogFilters}>
            {t('清空')}
          </button>
        </div>
      )}
    </section>
  );
}
