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
        aria-label={t('价格排序')}
        value={sortOrder}
        onChange={setSortOrder}
        optionList={pricingSortOptions(t)}
        title={t('动态或不适用的价格排在末尾')}
      />
      <Button
        icon={<SlidersHorizontal size={16} />}
        theme='outline'
        type='tertiary'
        onClick={() => setShowFilterModal(true)}
      >
        {t('更多筛选')}
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
        title={t('按K显示单位')}
        onClick={() => setTokenUnit(tokenUnit === 'M' ? 'K' : 'M')}
      >
        {tokenUnit}
      </Button>
      <Button
        icon={<RotateCcw size={14} />}
        theme='borderless'
        type='tertiary'
        onClick={resetCatalogFilters}
      >
        {t('重置')}
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
