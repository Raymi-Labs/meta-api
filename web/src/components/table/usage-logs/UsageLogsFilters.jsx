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
import { Button, Form, Switch } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';

import { DATE_RANGE_PRESETS } from '../../../constants/console.constants';
import {
  buildTokenGroupVendorOptions,
  filterTokenGroupsByVendor,
} from '../tokens/tokenGroupUtils';
import LogChannelPicker from './components/LogChannelPicker';

const LogsFilters = ({
  formInitValues,
  setFormApi,
  submitFilters,
  resetFilters,
  groupPrivacy,
  setGroupPrivacy,
  formApi,
  groupOptions,
  loading,
  isAdminUser,
  t,
}) => {
  const filterSize = isAdminUser ? 'small' : 'default';
  const sensitiveClass = groupPrivacy ? 'usage-log-group-private' : undefined;
  const [vendor, setVendor] = useState('');
  const [vendorSearch, setVendorSearch] = useState('');
  const [groupSearch, setGroupSearch] = useState('');
  const withHistoricalOption = (options, search) => {
    const value = search.trim();
    return value && !options.some((option) => option.value === value)
      ? [...options, { value, label: value }]
      : options;
  };
  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={submitFilters}
      onValueChange={(values) => setVendor(values.group_vendor || '')}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div
        className={`usage-logs-filters${isAdminUser ? '' : ' usage-logs-filters-user'}`}
      >
        <div className='usage-logs-filter-fields grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2'>
          {/* 时间选择器 */}
          <div className='usage-logs-filter-date col-span-1 lg:col-span-2'>
            <Form.DatePicker
              field='dateRange'
              className='w-full'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size={filterSize}
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
            />
          </div>

          {/* 其他搜索字段 */}
          <Form.Input
            field='token_name'
            className='usage-logs-filter-token'
            prefix={<IconSearch />}
            placeholder={t('令牌名称')}
            showClear
            pure
            size={filterSize}
          />

          <Form.Input
            field='model_name'
            className='usage-logs-filter-model'
            prefix={<IconSearch />}
            placeholder={t('模型名称')}
            showClear
            pure
            size={filterSize}
          />

          <div
            className={`usage-logs-filter-group${isAdminUser ? ' grid grid-cols-2 gap-2 min-w-0' : ''}`}
          >
            {isAdminUser && (
              <Form.Select
                field='group_vendor'
                className={sensitiveClass}
                dropdownClassName={sensitiveClass}
                placeholder={t('全部厂商')}
                optionList={withHistoricalOption(
                  buildTokenGroupVendorOptions(groupOptions, t('多厂商通用')),
                  vendorSearch,
                )}
                onSearch={setVendorSearch}
                onChange={() => {
                  formApi?.setValue('group', '');
                  setGroupSearch('');
                }}
                filter
                showClear
                pure
                size={filterSize}
              />
            )}
            <Form.Select
              field='group'
              className={sensitiveClass}
              dropdownClassName={sensitiveClass}
              placeholder={t('选择分组')}
              optionList={withHistoricalOption(
                isAdminUser && vendor
                  ? filterTokenGroupsByVendor(groupOptions, vendor)
                  : groupOptions,
                isAdminUser ? groupSearch : '',
              )}
              onSearch={setGroupSearch}
              filter
              showClear
              pure
              size={filterSize}
            />
          </div>

          <Form.Input
            field='request_id'
            className='usage-logs-filter-request'
            prefix={<IconSearch />}
            placeholder={t('Request ID')}
            showClear
            pure
            size={filterSize}
          />

          {isAdminUser && (
            <>
              <LogChannelPicker formApi={formApi} t={t} />
              <Form.Input
                field='username'
                prefix={<IconSearch />}
                placeholder={t('用户名称 / ID')}
                extraText={t(
                  '纯数字精确匹配用户 ID；纯数字用户名请使用 name:用户名',
                )}
                showClear
                pure
                size={filterSize}
              />
            </>
          )}
        </div>

        {/* 操作按钮区域 */}
        <div className='usage-logs-filter-footer flex flex-col sm:flex-row justify-between items-start sm:items-center gap-3 mt-3'>
          {/* 日志类型选择器 */}
          <div className='usage-logs-filter-type w-full sm:w-auto'>
            <Form.Select
              field='logType'
              placeholder={t('日志类型')}
              className='w-full sm:w-auto min-w-[120px]'
              showClear
              pure
              size={filterSize}
            >
              <Form.Select.Option value='0'>{t('全部')}</Form.Select.Option>
              <Form.Select.Option value='1'>{t('充值')}</Form.Select.Option>
              <Form.Select.Option value='2'>{t('消费')}</Form.Select.Option>
              <Form.Select.Option value='3'>{t('管理')}</Form.Select.Option>
              <Form.Select.Option value='4'>{t('系统')}</Form.Select.Option>
              <Form.Select.Option value='5'>{t('调用失败')}</Form.Select.Option>
              <Form.Select.Option value='6'>{t('退款')}</Form.Select.Option>
            </Form.Select>
          </div>

          <div className='usage-logs-filter-actions flex gap-2 w-full sm:w-auto justify-end items-center flex-wrap'>
            <label className='flex items-center gap-2 text-sm'>
              <Switch
                size='small'
                aria-label={t('分组隐私模式')}
                checked={groupPrivacy}
                onChange={(checked) => {
                  setGroupPrivacy(checked);
                  try {
                    localStorage.setItem(
                      'usage-logs-group-privacy',
                      String(checked),
                    );
                  } catch {
                    // Privacy still works when browser storage is unavailable.
                  }
                }}
              />
              {t('分组隐私模式')}
            </label>
            <Button
              type='primary'
              theme='solid'
              htmlType='submit'
              loading={loading}
              size={filterSize}
            >
              {t('查询')}
            </Button>
            <Button type='tertiary' onClick={resetFilters} size={filterSize}>
              {t('重置')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default LogsFilters;
