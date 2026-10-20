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
import { Tabs, TabPane, Tag, Typography } from '@douyinfe/semi-ui';
import { CHANNEL_OPTIONS } from '../../../constants';
import { getChannelIcon, getLobeHubIcon } from '../../../helpers';
import {
  CHANNEL_CATEGORY_ALL,
  CHANNEL_DISPLAY_VENDORS,
  channelTypeCategoryKey,
} from '../../../helpers/channelCategory';

const ChannelsTabs = ({
  enableTagMode,
  activeCategoryKey,
  handleCategoryChange,
  categoryCounts,
  availableCategoryKeys,
  t,
}) => {
  if (enableTagMode) return null;

  const handleTabChange = (key) => {
    handleCategoryChange(key);
  };

  const visibleVendors = CHANNEL_DISPLAY_VENDORS.filter(
    ({ value }) =>
      Number(categoryCounts?.[`vendor:${value}`] || 0) > 0 ||
      activeCategoryKey === `vendor:${value}`,
  );
  const visibleTypeOptions = CHANNEL_OPTIONS.filter(
    (opt) =>
      availableCategoryKeys.includes(channelTypeCategoryKey(opt.value)) ||
      activeCategoryKey === channelTypeCategoryKey(opt.value),
  );
  const renderTab = (key, label, icon) => (
    <TabPane
      key={key}
      itemKey={key}
      tab={
        <span className='flex items-center gap-2'>
          {icon}
          {label}
          <Tag
            color={activeCategoryKey === key ? 'red' : 'grey'}
            shape='circle'
          >
            {categoryCounts[key] || 0}
          </Tag>
        </span>
      }
    />
  );
  const vendorTabs = visibleVendors.map((vendor) =>
    renderTab(
      `vendor:${vendor.value}`,
      t(vendor.label),
      vendor.type
        ? getChannelIcon(vendor.type)
        : getLobeHubIcon(vendor.icon, 16),
    ),
  );
  const typeTabs = visibleTypeOptions.map((option) =>
    renderTab(
      channelTypeCategoryKey(option.value),
      visibleVendors.some((vendor) => vendor.type === option.value)
        ? `${option.label} (${t('渠道类型')})`
        : option.label,
      getChannelIcon(option.value),
    ),
  );

  return (
    <div className='mb-2 flex flex-col gap-1'>
      <Typography.Text type='tertiary' size='small'>
        {t('渠道分类')}
      </Typography.Text>
      <Tabs
        activeKey={activeCategoryKey}
        type='card'
        collapsible
        onChange={handleTabChange}
      >
        <TabPane
          itemKey={CHANNEL_CATEGORY_ALL}
          tab={
            <span className='flex items-center gap-2'>
              {t('全部')}
              <Tag
                color={
                  activeCategoryKey === CHANNEL_CATEGORY_ALL ? 'red' : 'grey'
                }
                shape='circle'
              >
                {categoryCounts[CHANNEL_CATEGORY_ALL] || 0}
              </Tag>
            </span>
          }
        />

        {vendorTabs}
        {typeTabs}
      </Tabs>
    </div>
  );
};

export default ChannelsTabs;
