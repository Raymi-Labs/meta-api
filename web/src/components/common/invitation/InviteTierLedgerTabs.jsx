import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Select,
  Space,
  Tabs,
  TabPane,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import CardTable from '../ui/CardTable';
import { inviteRequest, inviteStatusName } from './InviteTierPanel';
export const inviteDate = (value) =>
  value
    ? new Date(value * 1000).toLocaleString(undefined, {
        timeZone: 'Asia/Shanghai',
      })
    : '—';
export default function InviteTierLedgerTabs({
  visible = true,
  admin = false,
  userId = 0,
}) {
  const { t } = useTranslation();
  const [tab, setTab] = useState('statements'),
    [page, setPage] = useState(1),
    [filter, setFilter] = useState(''),
    [statement, setStatement] = useState(0),
    [data, setData] = useState({ items: [], total: 0 }),
    [loading, setLoading] = useState(false),
    [error, setError] = useState(''),
    [refresh, setRefresh] = useState(0);
  const seq = useRef(0);
  useEffect(() => {
    setData({ items: [], total: 0 });
    setPage(1);
    setStatement(0);
  }, [userId, admin]);
  useEffect(() => {
    if (!visible) return;
    const n = ++seq.current;
    setLoading(true);
    const params = new URLSearchParams({
      p: String(page),
      page_size: '10',
      status: filter,
    });
    if (userId) params.set('user_id', userId);
    if (statement && tab === 'ledgers') params.set('statement_id', statement);
    inviteRequest(
      `/api/${admin ? 'admin/' : ''}invite/${admin ? '' : 'commission/'}${tab}?${params}`,
    )
      .then((result) => {
        if (n === seq.current) {
          setData(result || { items: [], total: 0 });
          setError('');
        }
      })
      .catch((e) => {
        if (n === seq.current) setError(e.message);
      })
      .finally(() => {
        if (n === seq.current) setLoading(false);
      });
    return () => {
      seq.current++;
    };
  }, [visible, admin, userId, tab, page, filter, statement, refresh]);
  const money = (title, key) => ({
    title: t(title),
    dataIndex: key,
    render: (value) => renderQuota(value || 0),
  });
  const columns =
    tab === 'statements'
      ? [
          { title: t('周期'), dataIndex: 'period_key' },
          {
            title: t('状态'),
            dataIndex: 'status',
            render: (value) => t(inviteStatusName(value)),
          },
          money('应发', 'due_amount'),
          money('实发', 'paid_amount'),
          money('扣减', 'deducted_amount'),
          money('结转补足', 'carry_over_amount'),
          {
            title: t('异议截止'),
            dataIndex: 'dispute_deadline',
            render: inviteDate,
          },
          {
            title: t('操作'),
            render: (_, row) => (
              <Button
                onClick={() => {
                  setStatement(row.id);
                  setTab('ledgers');
                  setPage(1);
                  setFilter('');
                }}
              >
                {t('查看明细')}
              </Button>
            ),
          },
        ]
      : [
          { title: t('订单'), dataIndex: 'topup_trade_no' },
          {
            title: t('级别'),
            dataIndex: 'commission_level',
            render: (value) =>
              t(
                [
                  '',
                  '一级',
                  '二级',
                  '达标奖金',
                  '部门分成',
                  '成员的部长位',
                  '注册奖励',
                ][value] || '—',
              ),
          },
          {
            title: t('状态'),
            dataIndex: 'status',
            render: (value, row) =>
              value === 'pending'
                ? t(row.statement_id ? '已出账' : '待出账')
                : t(inviteStatusName(value)),
          },
          money('返佣基数', 'base_quota'),
          money('排除基数', 'excluded_base_quota'),
          money('应发', 'commission_quota'),
          money('实发', 'settled_quota'),
          money('欠款抵扣', 'debt_offset'),
          money('池调节', 'pool_deduction'),
          {
            title: t('预计到账时间'),
            dataIndex: 'payout_at',
            render: inviteDate,
          },
          { title: t('原因'), dataIndex: 'risk_reason' },
        ];
  return (
    <div className='space-y-3'>
      <Tabs
        activeKey={tab}
        onChange={(key) => {
          setTab(key);
          setPage(1);
          setFilter('');
        }}
      >
        <TabPane tab={t('周期账单')} itemKey='statements' />
        <TabPane tab={t('台账明细')} itemKey='ledgers' />
      </Tabs>
      <Typography.Text type='tertiary'>
        {t('时间均为北京时间，预计到账仍受观察期、实名与冻结状态影响。')}
      </Typography.Text>
      <Space wrap>
        <Select
          value={filter}
          onChange={(v) => {
            setFilter(v);
            setPage(1);
          }}
          optionList={[
            { value: '', label: t('全部状态') },
            ...(tab === 'statements'
              ? ['1', '2', '3', '4']
              : ['pending', 'settled', 'skipped']
            ).map((value) => ({ value, label: t(inviteStatusName(value)) })),
          ]}
        />
        <Button onClick={() => setRefresh((v) => v + 1)}>{t('刷新')}</Button>
        {statement > 0 && (
          <Button
            onClick={() => {
              setStatement(0);
              setPage(1);
            }}
          >
            {t('全部明细')}
          </Button>
        )}
      </Space>
      {error && <Typography.Text type='danger'>{error}</Typography.Text>}
      <CardTable
        empty={<div className='p-6 text-center'>{t('暂无数据')}</div>}
        rowKey='id'
        columns={columns}
        dataSource={data.items || []}
        loading={loading}
        scroll={{ x: 1100 }}
        pagination={{
          currentPage: page,
          pageSize: 10,
          total: data.total || 0,
          onPageChange: setPage,
        }}
        hidePagination={false}
      />
    </div>
  );
}
