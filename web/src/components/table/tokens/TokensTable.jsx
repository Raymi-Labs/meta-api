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

import React, { useMemo } from 'react';
import { Empty, Checkbox } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getTokensColumns } from './TokensColumnDefs';

const TokensTable = (tokensData) => {
  const {
    tokens,
    selectedKeys = [],
    loading,
    activePage,
    pageSize,
    tokenCount,
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
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

  const tableColumns = columns.map((column) =>
    column.dataIndex === 'name'
      ? {
          ...column,
          render: (text, record) => (
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
              {column.render(text, record)}
            </div>
          ),
        }
      : column,
  );

  return (
    <>
      <CardTable
        columns={tableColumns}
        rowKey='id'
        expandRowByClick={false}
        dataSource={tokens}
        scroll={{ x: 'max-content' }}
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
