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
import { SideSheet, Button } from '@douyinfe/semi-ui';
import { resetPricingFilters } from '../../../../helpers/utils';
import FilterModalContent from './components/FilterModalContent';
import './pricingFilterDrawer.css';

const PricingFilterModal = ({ visible, onClose, sidebarProps, t }) => {
  const handleResetFilters = () => {
    sidebarProps.resetCatalogFilters();
    sidebarProps.setSortOrder('release-desc');
    resetPricingFilters({
      handleChange: sidebarProps.handleChange,
      setShowWithRecharge: sidebarProps.setShowWithRecharge,
      setCurrency: sidebarProps.setCurrency,
      setPriceConvertMode: sidebarProps.setPriceConvertMode,
      setSelectedPlanId: sidebarProps.setSelectedPlanId,
      setShowRatio: sidebarProps.setShowRatio,
      setViewMode: () => sidebarProps.setViewMode(sidebarProps.defaultViewMode),
      setFilterGroup: sidebarProps.handleGroupClick,
      setFilterQuotaType: sidebarProps.setFilterQuotaType,
      setFilterEndpointType: sidebarProps.setFilterEndpointType,
      setFilterVendor: sidebarProps.setFilterVendor,
      setFilterTag: sidebarProps.setFilterTag,
      setCurrentPage: sidebarProps.setCurrentPage,
      setTokenUnit: sidebarProps.setTokenUnit,
    });
  };

  return (
    <SideSheet
      className='pricing-filter-drawer'
      placement='right'
      width='min(640px, 100vw)'
      closeOnEsc
      aria-label={t('筛选')}
      title={
        <div>
          <h2 className='pricing-filter-title'>{t('筛选')}</h2>
          <p className='pricing-filter-subtitle'>
            {t('按供应商、功能、标签和输入模态发现模型。')}
          </p>
        </div>
      }
      visible={visible}
      onCancel={onClose}
      maskStyle={{ backdropFilter: 'blur(3px)' }}
      footer={
        <div className='pricing-filter-footer'>
          <Button theme='outline' onClick={handleResetFilters}>
            {t('重置')}
          </Button>
          <Button theme='solid' onClick={onClose}>
            {t('关闭')}
          </Button>
        </div>
      }
      bodyStyle={{ padding: '0 24px', overflowY: 'auto' }}
    >
      <p className='pricing-filter-hint'>
        {t('选择后立即生效，关闭后保留筛选条件。')}
      </p>
      <FilterModalContent sidebarProps={sidebarProps} t={t} />
    </SideSheet>
  );
};

export default PricingFilterModal;
