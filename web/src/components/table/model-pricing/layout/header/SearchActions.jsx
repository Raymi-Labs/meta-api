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

import { pricingSortOptions } from '../../../../../helpers/modelPriceSorting';

import React, { memo, useCallback } from 'react';
import { Input, Button, Switch, Select, Divider } from '@douyinfe/semi-ui';
import { IconSearch, IconCopy, IconFilter } from '@douyinfe/semi-icons';

const SearchActions = memo(
  ({
    sortOrder,
    setSortOrder,
    selectedRowKeys = [],
    copyText,
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    isMobile = false,
    searchValue = '',
    setShowFilterModal,
    showWithRecharge,
    setShowWithRecharge,
    priceConvertMode = 'recharge',
    setPriceConvertMode,
    subscriptionPlans = [],
    availablePlans = [],
    selectedPlanId,
    setSelectedPlanId,
    currency,
    setCurrency,
    showRatio,
    setShowRatio,
    viewMode,
    setViewMode,
    tokenUnit,
    setTokenUnit,
    t,
  }) => {
    const hasAvailablePlans = availablePlans.length > 0;

    const handleCopyClick = useCallback(() => {
      if (copyText && selectedRowKeys.length > 0) {
        copyText(selectedRowKeys);
      }
    }, [copyText, selectedRowKeys]);

    const handleFilterClick = useCallback(() => {
      setShowFilterModal?.(true);
    }, [setShowFilterModal]);

    const handleViewModeToggle = useCallback(() => {
      setViewMode?.(viewMode === 'table' ? 'card' : 'table');
    }, [viewMode, setViewMode]);

    const handleTokenUnitToggle = useCallback(() => {
      setTokenUnit?.(tokenUnit === 'K' ? 'M' : 'K');
    }, [tokenUnit, setTokenUnit]);

    const handlePriceModeChange = useCallback(
      (value) => {
        if (value === 'package' && !hasAvailablePlans) {
          return;
        }
        setPriceConvertMode?.(value);
      },
      [hasAvailablePlans, setPriceConvertMode],
    );

    return (
      <div className='flex flex-wrap items-center gap-2 w-full'>
        <div className='flex-1 min-w-[120px]'>
          <Input
            prefix={<IconSearch />}
            placeholder={t('模糊搜索模型名称')}
            value={searchValue}
            onCompositionStart={handleCompositionStart}
            onCompositionEnd={handleCompositionEnd}
            onChange={handleChange}
            showClear
          />
        </div>

        <Select
          aria-label={t('价格排序')}
          value={sortOrder}
          onChange={setSortOrder}
          style={{ width: isMobile ? '100%' : 180 }}
          optionList={pricingSortOptions(t)}
          title={t('动态或不适用的价格排在末尾')}
        />

        <Button
          theme='outline'
          type='primary'
          icon={<IconCopy />}
          onClick={handleCopyClick}
          disabled={selectedRowKeys.length === 0}
          className='!bg-blue-500 hover:!bg-blue-600 !text-white disabled:!bg-gray-300 disabled:!text-gray-500'
        >
          {t('复制')}
        </Button>

        {!isMobile && (
          <>
            <Divider layout='vertical' margin='8px' />

            {/* 价格显示模式下拉框 + 开关 */}
            <Select
              value={priceConvertMode}
              onChange={handlePriceModeChange}
              style={{ width: 120 }}
              optionList={[
                { value: 'recharge', label: t('充值价格') },
                {
                  value: 'package',
                  label: t('套餐价格'),
                  disabled: !hasAvailablePlans,
                },
              ]}
            />
            <Switch
              checked={showWithRecharge}
              onChange={setShowWithRecharge}
              disabled={priceConvertMode === 'package' && !hasAvailablePlans}
            />

            {/* 套餐选择器（选了套餐价格时始终显示） */}
            {priceConvertMode === 'package' && (
              <Select
                value={selectedPlanId}
                onChange={setSelectedPlanId}
                placeholder={
                  hasAvailablePlans ? t('选择套餐') : t('暂无可用套餐')
                }
                style={{ width: 180 }}
                disabled={!hasAvailablePlans}
                optionList={availablePlans.map((p) => ({
                  value: p.id,
                  label: `${p.title} - ¥${p.price_amount}`,
                }))}
              />
            )}

            {/* 货币单位选择：保留逻辑，仅通过样式隐藏 */}
            <div style={{ display: 'none' }} aria-hidden='true'>
              <Select
                value={currency}
                onChange={setCurrency}
                optionList={[
                  { value: 'USD', label: 'USD' },
                  { value: 'CNY', label: 'CNY' },
                  { value: 'CUSTOM', label: t('自定义货币') },
                ]}
              />
            </div>

            {/* 显示倍率开关 */}
            <div className='flex items-center gap-2'>
              <span className='text-sm text-gray-600'>{t('倍率')}</span>
              <Switch checked={showRatio} onChange={setShowRatio} />
            </div>

            {/* 视图模式切换按钮 */}
            <Button
              theme={viewMode === 'table' ? 'solid' : 'outline'}
              type={viewMode === 'table' ? 'primary' : 'tertiary'}
              onClick={handleViewModeToggle}
            >
              {t('表格视图')}
            </Button>

            {/* Token单位切换按钮 */}
            <Button
              theme={tokenUnit === 'K' ? 'solid' : 'outline'}
              type={tokenUnit === 'K' ? 'primary' : 'tertiary'}
              onClick={handleTokenUnitToggle}
            >
              {tokenUnit}
            </Button>
          </>
        )}

        {
          <Button
            theme='outline'
            type='tertiary'
            icon={<IconFilter />}
            onClick={handleFilterClick}
          >
            {t('更多筛选')}
          </Button>
        }
      </div>
    );
  },
);

SearchActions.displayName = 'SearchActions';

export default SearchActions;
