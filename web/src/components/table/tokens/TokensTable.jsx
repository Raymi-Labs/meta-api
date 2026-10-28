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

import React, { useMemo, useState } from 'react';
import { Empty, Button, Checkbox, Tag } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getTokensColumns } from './TokensColumnDefs';

const TokensTable = (tokensData) => {
  const [fullView, setFullView] = useState(false);
  const {
    tokens,
    selectedKeys = [],
    loading,
    activePage,
    pageSize,
    tokenCount,
    compactMode,
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
    handleRow,
    showKeys,
    tokenFullKeys,
    toggleTokenKeyVisibility,
    copyTokenKey,
    manageToken,
    onOpenLink,
    openTestModal,
    setEditingToken,
    setShowEdit,
    refresh,
    t,
  } = tokensData;

  // Get all columns
  const columns = useMemo(() => {
    return getTokensColumns({
      t,
      showKeys,
      tokenFullKeys,
      toggleTokenKeyVisibility,
      copyTokenKey,
      manageToken,
      onOpenLink,
      openTestModal,
      setEditingToken,
      setShowEdit,
      refresh,
    });
  }, [
    t,
    showKeys,
    tokenFullKeys,
    toggleTokenKeyVisibility,
    copyTokenKey,
    manageToken,
    onOpenLink,
    openTestModal,
    setEditingToken,
    setShowEdit,
    refresh,
  ]);

  const cell = (key, record) => {
    const column = columns.find((item) => (item.key || item.dataIndex) === key);
    return column?.render(record[column.dataIndex], record);
  };
  const restrictions = (record) =>
    [
      (record.model_limits_enabled || record.model_limits) && t('模型限制'),
      (record.channel_limits_enabled || record.channel_limits) &&
        t('渠道限制列表'),
      record.allow_ips && t('IP限制'),
      (record.max_concurrency > 0 || record.window_request_limit > 0) &&
        t('运行限制'),
    ].filter(Boolean);
  const tableColumns = [
    {
      title: t('名称'),
      key: 'identity',
      width: 250,
      render: (_, record) => (
        <div className='token-identity-cell'>
          <div className='token-name-line'>
            <Checkbox
              className='token-mobile-select'
              checked={selectedKeys.some((item) => item.id === record.id)}
              aria-label={`${t('批量操作')}: ${record.name}`}
              onChange={(event) => {
                const records = event.target.checked
                  ? [...selectedKeys, record]
                  : selectedKeys.filter((item) => item.id !== record.id);
                rowSelection.onChange(
                  records.map((item) => item.id),
                  records,
                );
              }}
            />
            {cell('name', record)}
          </div>
          {cell('token_key', record)}
          {restrictions(record).length > 0 && (
            <div className='token-restriction-tags'>
              {restrictions(record).map((label) => (
                <Tag key={label} size='small' color='white'>
                  {label}
                </Tag>
              ))}
            </div>
          )}
        </div>
      ),
    },
    { ...columns.find((c) => c.key === 'status'), width: 100 },
    {
      title: t('剩余额度/总额度'),
      key: 'quota',
      width: 180,
      render: (_, record) => (
        <div className='token-quota-cell'>
          {cell('quota_usage', record)}
          <span className='token-cell-caption'>
            {t('已使用余额')}: {cell('used_quota', record)}
          </span>
          {record.package_enabled && cell('package_cycle_balance', record)}
        </div>
      ),
    },
    {
      title: t('分组'),
      key: 'group',
      width: 160,
      render: (_, record) => (
        <div className='token-group-cell'>
          {cell('group', record)}
          {cell('group_ratio', record)}
        </div>
      ),
    },
    { ...columns.find((c) => c.dataIndex === 'expired_time'), width: 155 },
    {
      title: t('操作'),
      key: 'primary-actions',
      width: 110,
      fixed: compactMode ? undefined : 'right',
      render: (_, record) => (
        <Button
          size='small'
          theme='light'
          onClick={() => {
            setEditingToken(record);
            setShowEdit(true);
          }}
        >
          {t('编辑')}
        </Button>
      ),
    },
  ];
  const detailKeys = [
    'model_limits',
    'allow_ips',
    'runtime_limits',
    'created_time',
    'accessed_time',
  ];
  const renderDetails = (record) => (
    <div className='token-expanded-details'>
      <dl>
        {detailKeys.map((key) => {
          const column = columns.find((c) => (c.key || c.dataIndex) === key);
          return (
            <div key={key}>
              <dt>{column.title}</dt>
              <dd>
                {key === 'model_limits' && record.model_limits_enabled ? (
                  <span>{record.model_limits || '—'}</span>
                ) : (
                  cell(key, record)
                )}
              </dd>
            </div>
          );
        })}
        {(record.channel_limits_enabled || record.channel_limits) && (
          <div>
            <dt>{t('渠道限制列表')}</dt>
            <dd>{record.channel_limits || '—'}</dd>
          </div>
        )}
      </dl>
      <div className='token-expanded-actions'>{cell('operate', record)}</div>
    </div>
  );

  return (
    <>
      <div className='tokens-view-options'>
        <Checkbox
          checked={fullView}
          onChange={(event) => setFullView(event.target.checked)}
        >
          {t('显示全部列')}
        </Checkbox>
      </div>
      <CardTable
        columns={
          fullView
            ? columns.map((column) =>
                compactMode && column.fixed
                  ? { ...column, fixed: undefined }
                  : column,
              )
            : tableColumns
        }
        rowKey='id'
        expandedRowRender={fullView ? undefined : renderDetails}
        expandRowByClick={false}
        dataSource={tokens}
        scroll={compactMode ? undefined : { x: 'max-content' }}
        pagination={{
          currentPage: activePage,
          pageSize: pageSize,
          total: tokenCount,
          showSizeChanger: true,
          pageSizeOptions: [10, 20, 50, 100],
          onPageSizeChange: handlePageSizeChange,
          onPageChange: handlePageChange,
        }}
        hidePagination={true}
        loading={loading}
        rowSelection={rowSelection}
        empty={
          <Empty
            image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
            }
            description={t('搜索无结果')}
            style={{ padding: 30 }}
          />
        }
        className='rounded-xl overflow-hidden'
        size='middle'
      />
    </>
  );
};

export default TokensTable;
