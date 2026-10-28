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

import React, { useContext, useState } from 'react';
import { Banner, Button } from '@douyinfe/semi-ui';
import { timestamp2string } from '../../../helpers';
import { UserContext } from '../../../context/User';
import './usageLogs.css';
import LogsTable from './UsageLogsTable';
import LogsActions from './UsageLogsActions';
import LogsFilters from './UsageLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import UserInfoModal from './modals/UserInfoModal';
import ChannelAffinityUsageCacheModal from './modals/ChannelAffinityUsageCacheModal';
import ParamOverrideModal from './modals/ParamOverrideModal';
import TopUsersDrawer from './modals/TopUsersDrawer';
import LogGroupSummaryModal from './modals/LogGroupSummaryModal';
import { useLogsData } from '../../../hooks/usage-logs/useUsageLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const LogsPageContent = ({ admin }) => {
  const logsData = useLogsData(admin);
  const isMobile = useIsMobile();
  const [filtersOpen, setFiltersOpen] = useState(logsData.fromChannelUsage);

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...logsData} />
      <UserInfoModal {...logsData} />
      <ChannelAffinityUsageCacheModal {...logsData} />
      <ParamOverrideModal {...logsData} />
      <TopUsersDrawer {...logsData} />
      {logsData.isAdminUser && <LogGroupSummaryModal {...logsData} />}

      {/* Main Content */}
      {logsData.fromChannelUsage && (
        <Banner
          type='info'
          closeIcon={null}
          style={{ marginBottom: 12 }}
          description={logsData.t(
            '已带入渠道与时间筛选。渠道用量汇总消费和错误日志；本页额度统计仅包含消费记录，全部日志还可能包含退款等记录。',
          )}
        />
      )}
      <section className='usage-logs-page'>
        <LogsActions
          {...logsData}
          filtersOpen={filtersOpen}
          setFiltersOpen={setFiltersOpen}
        />
        <LogsFilters {...logsData} filtersOpen={filtersOpen} />
        <div className='usage-logs-applied' aria-label={logsData.t('应用筛选')}>
          <span>
            {logsData.t('时间范围')}:{' '}
            {timestamp2string(logsData.appliedFilters.startTimestamp)} —{' '}
            {timestamp2string(logsData.appliedFilters.endTimestamp)} (
            {Intl.DateTimeFormat().resolvedOptions().timeZone})
          </span>
          {Object.entries({
            logType: [
              logsData.t('类型'),
              [
                logsData.t('全部'),
                logsData.t('充值'),
                logsData.t('消费'),
                logsData.t('管理'),
                logsData.t('系统'),
                logsData.t('调用失败'),
                logsData.t('退款'),
              ][logsData.appliedFilters.logType],
            ],
            model_name: [
              logsData.t('模型'),
              logsData.appliedFilters.model_name,
            ],
            token_name: [
              logsData.t('令牌'),
              logsData.appliedFilters.token_name,
            ],
            group: [logsData.t('分组'), logsData.appliedFilters.group],
            request_id: [
              logsData.t('Request ID'),
              logsData.appliedFilters.request_id,
            ],
            ...(logsData.isAdminUser
              ? {
                  username: [
                    logsData.t('用户'),
                    logsData.appliedFilters.username,
                  ],
                  channel_ids: [
                    logsData.t('渠道'),
                    logsData.appliedFilters.channel_ids,
                  ],
                  channel: [
                    logsData.t('渠道'),
                    logsData.appliedFilters.channel,
                  ],
                  channel_keyword: [
                    logsData.t('渠道'),
                    logsData.appliedFilters.channel_keyword,
                  ],
                  group_vendor: [
                    logsData.t('展示厂商'),
                    logsData.appliedFilters.group_vendor,
                  ],
                }
              : {}),
          })
            .filter(([, [, value]]) => value)
            .map(([key, [label, value]]) => (
              <span
                key={key}
                className={
                  key === 'group' || key === 'group_vendor'
                    ? logsData.groupPrivacy
                      ? 'usage-log-group-private'
                      : ''
                    : ''
                }
              >
                {label}: {value}
              </span>
            ))}
        </div>
        {logsData.listError ? (
          <div className='usage-logs-error' role='alert'>
            <p>{logsData.listError}</p>
            <Button onClick={logsData.retryList} loading={logsData.loading}>
              {logsData.t('重试')}
            </Button>
          </div>
        ) : (
          <LogsTable {...logsData} />
        )}
        {!logsData.listError && (
          <div className='usage-logs-pagination'>
            {createCardProPagination({
              currentPage: logsData.activePage,
              pageSize: logsData.pageSize,
              total: logsData.logCount,
              onPageChange: logsData.handlePageChange,
              onPageSizeChange: logsData.handlePageSizeChange,
              isMobile,
              t: logsData.t,
            })}
          </div>
        )}
      </section>
    </>
  );
};

const LogsPage = () => {
  const [userState] = useContext(UserContext);
  return (
    <LogsPageContent
      admin={Number(userState?.user?.role) >= 10}
      key={`${userState?.user?.id}:${userState?.user?.role}`}
    />
  );
};
export default LogsPage;
