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
import { Input, Spin } from '@douyinfe/semi-ui';
import SelectableButtonGroup from '../../../common/ui/SelectableButtonGroup';

/**
 * 分组筛选组件
 * @param {string} filterGroup 当前选中的分组，'all' 表示不过滤
 * @param {Function} setFilterGroup 设置选中分组
 * @param {Record<string, any>} usableGroup 后端返回的可用分组对象
 * @param {Record<string, number>} groupRatio 分组倍率对象
 * @param {Array} models 模型列表
 * @param {boolean} loading 是否加载中
 * @param {Function} t i18n
 */
const PricingGroups = ({
  filterGroup,
  setFilterGroup,
  usableGroup = {},
  groupRatio = {},
  models = [],
  loading = false,
  pills = false,
  hideUnavailable = false,
  vertical = false,
  t,
}) => {
  const [search, setSearch] = useState('');
  const groups = [
    'all',
    ...Object.keys(usableGroup)
      .filter((key) => key !== '')
      .sort((a, b) => (groupRatio[b] ?? 1) - (groupRatio[a] ?? 1)),
  ];

  const items = groups.map((g) => {
    const modelCount =
      g === 'all'
        ? models.length
        : models.filter((m) => m.enable_groups && m.enable_groups.includes(g))
            .length;
    let ratioDisplay = '';
    if (g === 'all') {
      ratioDisplay = t('全部');
    } else {
      const ratio = groupRatio[g];
      if (ratio !== undefined && ratio !== null) {
        ratioDisplay = `x${ratio}`;
      } else {
        ratioDisplay = 'x1';
      }
    }
    return {
      value: g,
      label: g === 'all' ? t('全部分组') : g,
      tagCount: pills && g === 'all' ? undefined : ratioDisplay,
      disabled: modelCount === 0,
    };
  });

  if (vertical) {
    const visible = items.filter(
      (item) =>
        item.value === 'all' ||
        item.value === filterGroup ||
        (!item.disabled &&
          item.label.toLowerCase().includes(search.toLowerCase().trim())),
    );
    return (
      <section aria-label={t('可用令牌分组')}>
        <h2 className='font-semibold mb-3'>{t('可用令牌分组')}</h2>
        <Input
          value={search}
          onChange={setSearch}
          showClear
          aria-label={t('搜索分组')}
          placeholder={t('搜索分组')}
        />
        {loading ? (
          <Spin />
        ) : (
          <div className='pricing-group-list'>
            {visible.map((item) => (
              <button
                type='button'
                key={item.value}
                aria-pressed={filterGroup === item.value}
                onClick={() => setFilterGroup(item.value)}
              >
                <span className='pricing-group-label'>
                  <span>{item.label}</span>
                  {item.value !== 'all' && (
                    <span className='pricing-group-ratio'>
                      {groupRatio[item.value] ?? 1}×
                    </span>
                  )}
                </span>
                {usableGroup[item.value] && (
                  <span className='pricing-group-description'>
                    {usableGroup[item.value]}
                  </span>
                )}
                {item.disabled &&
                  item.value === filterGroup &&
                  item.value !== 'all' && (
                    <span className='pricing-group-description'>
                      {t('当前厂商无可用模型，请选择其他分组')}
                    </span>
                  )}
              </button>
            ))}
            {visible.every((item) => item.value === 'all' || item.disabled) && (
              <p className='pricing-group-description'>{t('搜索无结果')}</p>
            )}
          </div>
        )}
      </section>
    );
  }

  return (
    <SelectableButtonGroup
      pills={pills}
      title={t('可用令牌分组')}
      items={items}
      activeValue={filterGroup}
      onChange={setFilterGroup}
      loading={loading}
      hideDisabled={hideUnavailable}
      resetValue='all'
      t={t}
    />
  );
};

export default PricingGroups;
