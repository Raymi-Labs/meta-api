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
import {
  Banner,
  Button,
  Empty,
  Radio,
  Select,
  SideSheet,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { Activity, Crown, RefreshCw, Users, Wallet } from 'lucide-react';
import { renderNumber, renderQuota } from '../../../../helpers';
import './TopUsersDrawer.css';

const { Text, Title } = Typography;
const RadioGroup = Radio.Group;

const limitOptions = [10, 20, 50].map((value) => ({
  label: String(value),
  value,
}));

const orderOptions = (t) => [
  { label: t('降序'), value: 'desc' },
  { label: t('升序'), value: 'asc' },
];

const viewModeOptions = (t) => [
  { label: t('同时看两个'), value: 'both' },
  { label: t('只看额度'), value: 'quota' },
  { label: t('只看请求数'), value: 'requests' },
];

const viewModeLabelMap = (t) => ({
  both: t('双榜对比'),
  quota: t('额度榜'),
  requests: t('请求榜'),
});

function getUserIdentity(record) {
  return `${record?.user_id || 'anonymous'}-${record?.username || '-'}`;
}

function getRankClassName(index) {
  if (index === 0) return 'top-users-drawer__rank top-users-drawer__rank--gold';
  if (index === 1)
    return 'top-users-drawer__rank top-users-drawer__rank--silver';
  if (index === 2)
    return 'top-users-drawer__rank top-users-drawer__rank--bronze';
  return 'top-users-drawer__rank';
}

const TopUsersDrawer = ({
  showTopUsersDrawer,
  setShowTopUsersDrawer,
  topUsersData,
  topUsersLoading,
  topUsersError,
  topUsersViewMode,
  setTopUsersViewMode,
  topUsersQuotaOrder,
  setTopUsersQuotaOrder,
  topUsersRequestOrder,
  setTopUsersRequestOrder,
  topUsersLimit,
  setTopUsersLimit,
  selectTopUser,
  refreshTopUsers,
  currentTopUsersLogType,
  t,
}) => {
  const boardSummary = useMemo(() => {
    const quotaList = topUsersData?.by_quota || [];
    const requestList = topUsersData?.by_requests || [];
    const uniqueUsers = new Map();

    [...quotaList, ...requestList].forEach((item) => {
      uniqueUsers.set(getUserIdentity(item), item);
    });

    return {
      userCount: uniqueUsers.size,
      quotaLeader: quotaList[0] || null,
      requestLeader: requestList[0] || null,
    };
  }, [topUsersData]);

  const rankColumns = useMemo(
    () => [
      {
        title: '#',
        dataIndex: 'rank',
        width: 72,
        render: (_, __, index) => (
          <span className={getRankClassName(index)}>{index + 1}</span>
        ),
      },
      {
        title: t('用户'),
        dataIndex: 'username',
        width: 220,
        render: (text, record) => (
          <Button
            className='top-users-drawer__user-button'
            type='tertiary'
            theme='borderless'
            onClick={() => selectTopUser(record.username)}
          >
            <span className='top-users-drawer__user-name'>{text}</span>
          </Button>
        ),
      },
      {
        title: t('分组'),
        dataIndex: 'group',
        width: 120,
        render: (text) => (
          <Tag size='small' color='white' shape='circle'>
            {text || '-'}
          </Tag>
        ),
      },
      {
        title: t('消耗额度'),
        dataIndex: 'quota',
        width: 140,
        render: (value) => (
          <span className='top-users-drawer__metric'>{renderQuota(value)}</span>
        ),
      },
      {
        title: t('请求数'),
        dataIndex: 'request_count',
        width: 120,
        render: (value) => (
          <span className='top-users-drawer__metric'>
            {renderNumber(value)}
          </span>
        ),
      },
      {
        title: t('输入 Tokens'),
        dataIndex: 'prompt_tokens',
        width: 140,
        render: (value) => (
          <span className='top-users-drawer__metric'>
            {renderNumber(value)}
          </span>
        ),
      },
      {
        title: t('输出 Tokens'),
        dataIndex: 'completion_tokens',
        width: 140,
        render: (value) => (
          <span className='top-users-drawer__metric'>
            {renderNumber(value)}
          </span>
        ),
      },
    ],
    [selectTopUser, t],
  );

  const showQuotaTable =
    topUsersViewMode === 'both' || topUsersViewMode === 'quota';
  const showRequestTable =
    topUsersViewMode === 'both' || topUsersViewMode === 'requests';
  const consumeOnlyMode =
    currentTopUsersLogType !== 0 && currentTopUsersLogType !== 2;

  const overviewCards = [
    {
      key: 'users',
      icon: <Users size={18} />,
      label: t('上榜用户'),
      value: boardSummary.userCount || 0,
      helper: t('当前筛选范围内去重统计'),
    },
    {
      key: 'view',
      icon: <Activity size={18} />,
      label: t('当前视图'),
      value: viewModeLabelMap(t)[topUsersViewMode] || t('双榜对比'),
      helper: `${t('榜单条数')} ${topUsersLimit}`,
    },
    {
      key: 'quota',
      icon: <Wallet size={18} />,
      label: t('额度榜榜首'),
      value: boardSummary.quotaLeader?.username || '-',
      helper: boardSummary.quotaLeader
        ? renderQuota(boardSummary.quotaLeader.quota)
        : t('暂无额度榜数据'),
    },
    {
      key: 'requests',
      icon: <Crown size={18} />,
      label: t('请求榜榜首'),
      value: boardSummary.requestLeader?.username || '-',
      helper: boardSummary.requestLeader
        ? `${renderNumber(boardSummary.requestLeader.request_count)} ${t('次请求')}`
        : t('暂无请求数榜数据'),
    },
  ];

  const renderTable = ({
    title,
    tone,
    dataSource,
    emptyDescription,
    order,
    leader,
  }) => (
    <section
      className={`top-users-drawer__board top-users-drawer__board--${tone}`}
    >
      <div className='top-users-drawer__board-head'>
        <div>
          <div className='top-users-drawer__board-title-row'>
            <Title heading={6} className='top-users-drawer__board-title'>
              {title}
            </Title>
            <Tag color={tone === 'blue' ? 'blue' : 'green'} shape='circle'>
              {order === 'desc' ? t('降序') : t('升序')}
            </Tag>
          </div>
        </div>
        <div className='top-users-drawer__board-highlight'>
          <span className='top-users-drawer__board-highlight-label'>
            {t('榜首')}
          </span>
          <strong>{leader?.username || '-'}</strong>
          <span className='top-users-drawer__board-highlight-value'>
            {tone === 'blue'
              ? leader
                ? renderQuota(leader.quota)
                : emptyDescription
              : leader
                ? `${renderNumber(leader.request_count)} ${t('次请求')}`
                : emptyDescription}
          </span>
        </div>
      </div>

      <Table
        className='top-users-drawer__table'
        rowKey={(record) => `${title}-${record.user_id}-${record.username}`}
        columns={rankColumns}
        dataSource={dataSource}
        loading={topUsersLoading}
        size='small'
        pagination={false}
        scroll={{ x: 'max-content' }}
        empty={
          <Empty
            description={emptyDescription}
            image={null}
            className='top-users-drawer__empty'
          />
        }
      />
    </section>
  );

  return (
    <SideSheet
      className='top-users-drawer'
      title={t('大用户榜单')}
      visible={showTopUsersDrawer}
      onCancel={() => setShowTopUsersDrawer(false)}
      width={980}
      bodyStyle={{ padding: 0 }}
    >
      {topUsersError ? (
        <div className='usage-logs-error' role='alert'>
          <p>{t('加载失败，请重试')}</p>
          <Button onClick={refreshTopUsers}>{t('重试')}</Button>
        </div>
      ) : (
        <div className='top-users-drawer__shell'>
          <section className='top-users-drawer__hero'>
            <div className='top-users-drawer__hero-copy'>
              <Text className='top-users-drawer__eyebrow'>
                {t('消费日志排行')}
              </Text>
              <Title heading={4} className='top-users-drawer__hero-title'>
                {t('快速定位高消耗和高频用户')}
              </Title>
              <Text className='top-users-drawer__hero-description'>
                {t(
                  '榜单只统计当前时间范围内的消费日志，点击用户名可直接回填筛选条件继续排查。',
                )}
              </Text>
            </div>

            <div className='top-users-drawer__overview-grid'>
              {overviewCards.map((item) => (
                <div className='top-users-drawer__overview-card' key={item.key}>
                  <span className='top-users-drawer__overview-icon'>
                    {item.icon}
                  </span>
                  <span className='top-users-drawer__overview-label'>
                    {item.label}
                  </span>
                  <strong className='top-users-drawer__overview-value'>
                    {item.value}
                  </strong>
                  <span className='top-users-drawer__overview-helper'>
                    {item.helper}
                  </span>
                </div>
              ))}
            </div>
          </section>

          {consumeOnlyMode ? (
            <Banner
              className='top-users-drawer__banner'
              type='warning'
              closeIcon={null}
              description={t(
                '当前榜单始终按消费日志统计，已忽略当前非消费日志类型筛选。',
              )}
            />
          ) : null}

          <section className='top-users-drawer__controls'>
            <div className='top-users-drawer__control-block'>
              <Text className='top-users-drawer__control-label'>
                {t('查看方式')}
              </Text>
              <RadioGroup
                type='button'
                value={topUsersViewMode}
                onChange={(e) => setTopUsersViewMode(e.target.value)}
              >
                {viewModeOptions(t).map((option) => (
                  <Radio value={option.value} key={option.value}>
                    {option.label}
                  </Radio>
                ))}
              </RadioGroup>
            </div>

            <div className='top-users-drawer__control-grid'>
              <div className='top-users-drawer__control-field'>
                <Text className='top-users-drawer__control-label'>
                  {t('榜单条数')}
                </Text>
                <Select
                  value={topUsersLimit}
                  optionList={limitOptions}
                  onChange={(value) => setTopUsersLimit(Number(value))}
                  style={{ width: '100%' }}
                />
              </div>

              <div className='top-users-drawer__control-field'>
                <Text className='top-users-drawer__control-label'>
                  {t('额度榜排序')}
                </Text>
                <Select
                  value={topUsersQuotaOrder}
                  optionList={orderOptions(t)}
                  onChange={(value) => setTopUsersQuotaOrder(value)}
                  style={{ width: '100%' }}
                />
              </div>

              <div className='top-users-drawer__control-field'>
                <Text className='top-users-drawer__control-label'>
                  {t('请求数榜排序')}
                </Text>
                <Select
                  value={topUsersRequestOrder}
                  optionList={orderOptions(t)}
                  onChange={(value) => setTopUsersRequestOrder(value)}
                  style={{ width: '100%' }}
                />
              </div>

              <div className='top-users-drawer__control-action'>
                <Button
                  block
                  icon={<RefreshCw size={16} />}
                  onClick={refreshTopUsers}
                >
                  {t('刷新榜单')}
                </Button>
              </div>
            </div>
          </section>

          <div className='top-users-drawer__boards'>
            {showQuotaTable
              ? renderTable({
                  title: t('额度榜'),
                  tone: 'blue',
                  dataSource: topUsersData?.by_quota || [],
                  emptyDescription: t('暂无额度榜数据'),
                  order: topUsersQuotaOrder,
                  leader: boardSummary.quotaLeader,
                })
              : null}

            {showRequestTable
              ? renderTable({
                  title: t('请求榜'),
                  tone: 'teal',
                  dataSource: topUsersData?.by_requests || [],
                  emptyDescription: t('暂无请求数榜数据'),
                  order: topUsersRequestOrder,
                  leader: boardSummary.requestLeader,
                })
              : null}
          </div>
        </div>
      )}
    </SideSheet>
  );
};

export default TopUsersDrawer;
