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
import { Input, Button, Select } from '@douyinfe/semi-ui';
import {
  Search,
  Copy,
  SlidersHorizontal,
  LayoutGrid,
  List,
  RotateCcw,
} from 'lucide-react';
import { pricingSortOptions } from '../../../../../helpers/modelPriceSorting';

export default function SearchActions({
  sortOrder,
  setSortOrder,
  selectedRowKeys = [],
  copyText,
  handleChange,
  handleCompositionStart,
  handleCompositionEnd,
  searchValue = '',
  setShowFilterModal,
  viewMode,
  setViewMode,
  tokenUnit,
  setTokenUnit,
  resetCatalogFilters,
  t,
}) {
  return (
    <div className='pricing-catalog-toolbar'>
      <Input
        className='pricing-catalog-search'
        prefix={<Search size={18} />}
        aria-label={t('模糊搜索模型名称')}
        placeholder={t('模糊搜索模型名称')}
        value={searchValue}
        onCompositionStart={handleCompositionStart}
        onCompositionEnd={handleCompositionEnd}
        onChange={handleChange}
        showClear
      />
      <Select
        className='pricing-catalog-sort'
        aria-label={t('排序方式')}
        value={sortOrder}
        onChange={setSortOrder}
        optionList={pricingSortOptions(t)}
        title={t(
          sortOrder === 'release-desc'
            ? '按发布日期从新到旧，未知日期排在最后'
            : '动态或不适用的价格排在末尾',
        )}
      />
      <Button
        className='pricing-filter-trigger'
        aria-label={t('更多筛选')}
        title={t('更多筛选')}
        icon={<SlidersHorizontal size={16} />}
        theme='outline'
        type='tertiary'
        onClick={() => setShowFilterModal(true)}
      >
        <span className='pricing-filter-label'>{t('更多筛选')}</span>
      </Button>
      <div
        className='pricing-view-switch'
        role='group'
        aria-label={t('显示设置')}
      >
        {['card', 'table'].map((mode) => (
          <Button
            key={mode}
            icon={
              mode === 'card' ? <LayoutGrid size={16} /> : <List size={16} />
            }
            theme={viewMode === mode ? 'light' : 'borderless'}
            type={viewMode === mode ? 'primary' : 'tertiary'}
            aria-label={t(mode === 'card' ? '卡片视图' : '表格视图')}
            title={t(mode === 'card' ? '卡片视图' : '表格视图')}
            aria-pressed={viewMode === mode}
            onClick={() => setViewMode(mode)}
          />
        ))}
      </div>
      <Button
        theme='borderless'
        type='tertiary'
        className='pricing-unit-toggle'
        aria-label={`${t('单位')}: 1${tokenUnit} Tokens`}
        title={`${t('单位')}: 1${tokenUnit} → 1${tokenUnit === 'M' ? 'K' : 'M'} Tokens`}
        onClick={() => setTokenUnit(tokenUnit === 'M' ? 'K' : 'M')}
      >
        {tokenUnit}
      </Button>
      <Button
        className='pricing-reset'
        aria-label={t('重置')}
        title={t('重置')}
        icon={<RotateCcw size={16} />}
        theme='borderless'
        type='tertiary'
        onClick={resetCatalogFilters}
      >
        <span className='pricing-reset-label'>{t('重置')}</span>
      </Button>
      {selectedRowKeys.length > 0 && (
        <Button
          icon={<Copy size={14} />}
          theme='light'
          onClick={() => copyText(selectedRowKeys)}
        >
          {t('复制')} ({selectedRowKeys.length})
        </Button>
      )}
    </div>
  );
}
