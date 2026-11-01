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
import { Empty, Descriptions, Button } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { buildRequestFailureReport } from '../../../helpers/requestFailure';
import { getLogsColumns } from './UsageLogsColumnDefs';

const LogsTable = (logsData) => {
  const {
    logs,
    expandData,
    loading,
    activePage,
    pageSize,
    logCount,
    compactMode,
    effectiveVisibleColumns,
    handlePageChange,
    handlePageSizeChange,
    copyText,
    showUserInfoFunc,
    openChannelAffinityUsageCacheModal,
    hasExpandableRows,
    isAdminUser,
    billingDisplayMode,
    t,
    COLUMN_KEYS,
  } = logsData;

  // Get all columns
  const allColumns = useMemo(() => {
    return getLogsColumns({
      t,
      COLUMN_KEYS,
      copyText,
      showUserInfoFunc,
      openChannelAffinityUsageCacheModal,
      isAdminUser,
      billingDisplayMode,
    });
  }, [
    t,
    COLUMN_KEYS,
    copyText,
    showUserInfoFunc,
    openChannelAffinityUsageCacheModal,
    isAdminUser,
    billingDisplayMode,
  ]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    const order = [
      COLUMN_KEYS.TIME,
      COLUMN_KEYS.MODEL,
      COLUMN_KEYS.TYPE,
      COLUMN_KEYS.COST,
      COLUMN_KEYS.USE_TIME,
      COLUMN_KEYS.PROMPT,
      COLUMN_KEYS.COMPLETION,
      COLUMN_KEYS.TOKEN,
      COLUMN_KEYS.GROUP,
      COLUMN_KEYS.CHANNEL,
      COLUMN_KEYS.USERNAME,
      COLUMN_KEYS.RETRY,
      COLUMN_KEYS.IP,
      COLUMN_KEYS.DETAILS,
    ];
    return allColumns
      .filter((column) => effectiveVisibleColumns[column.key])
      .sort((a, b) => order.indexOf(a.key) - order.indexOf(b.key));
  };

  const effectiveVisibleColumnsList = useMemo(() => {
    return getVisibleColumns();
  }, [effectiveVisibleColumns, allColumns]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? effectiveVisibleColumnsList.map(({ fixed, ...rest }) => rest)
      : effectiveVisibleColumnsList;
  }, [compactMode, effectiveVisibleColumnsList]);

  const expandRowRender = (record, index) => {
    return (
      <div className='usage-logs-detail'>
        <Descriptions data={expandData[record.key]} />
        {record.type === 5 && (
          <Button
            size='small'
            onClick={(event) => {
              copyText(
                event,
                buildRequestFailureReport(record, t, isAdminUser),
              );
            }}
          >
            {t('复制排查信息')}
          </Button>
        )}
      </div>
    );
  };

  return (
    <CardTable
      columns={tableColumns}
      {...(hasExpandableRows() && {
        expandedRowRender: expandRowRender,
        expandRowByClick: true,
        rowExpandable: (record) =>
          expandData[record.key] && expandData[record.key].length > 0,
      })}
      dataSource={logs}
      rowKey='key'
      loading={loading}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      className='usage-logs-table'
      size='middle'
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={loading ? t('加载中...') : t('搜索无结果')}
          style={{ padding: 30 }}
        />
      }
      pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: logCount,
        pageSizeOptions: [10, 20, 50, 100],
        showSizeChanger: true,
        onPageSizeChange: (size) => {
          handlePageSizeChange(size);
        },
        onPageChange: handlePageChange,
      }}
      hidePagination={true}
    />
  );
};

export default LogsTable;
