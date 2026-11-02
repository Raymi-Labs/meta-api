import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { API, isRoot, showError, showSuccess } from '../../../helpers';
import {
  cashCSV,
  cashDateRange,
  cashMoney,
  cashRequestID,
} from '../cashFinance';
import CashLedgerDialog, { cashEntryKinds } from './CashLedgerDialog';

const { Text } = Typography;
const sourceNames = {
  opening: '期初余额',
  topup: '充值',
  gift: '赠送',
  unknown: '待核对',
};
const dateTime = (value) =>
  value ? new Date(value * 1000).toLocaleString() : '—';
const apiData = async (url, config) => {
  const { data } = await API.get(url, { skipErrorHandler: true, ...config });
  if (!data.success) throw new Error(data.message);
  return data.data;
};

function Trend({ days, t }) {
  const container = useRef(null);
  const [width, setWidth] = useState(840);
  useEffect(() => {
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(260, entry.contentRect.width)),
    );
    if (container.current) observer.observe(container.current);
    return () => observer.disconnect();
  }, [days]);
  const values = days
    .flatMap((d) => [d.income, d.cost])
    .filter((v) => typeof v === 'number' && Number.isFinite(v));
  if (!values.length)
    return <Empty description={t('补齐凭证后显示收益趋势')} />;
  const max = Math.max(1, ...values),
    min = Math.min(0, ...values),
    height = 200;
  const x = (i) => 62 + (i * (width - 92)) / Math.max(1, days.length - 1);
  const y = (n) => 160 - ((n - min) / (max - min)) * 140;
  const colors = {
    income: 'var(--semi-color-primary)',
    cost: 'var(--semi-color-warning)',
  };
  return (
    <div ref={container}>
      <Space>
        <Text style={{ color: colors.income }}>{t('分摊净到账收入')}</Text>
        <Text style={{ color: colors.cost }}>{t('上游现金成本')}</Text>
      </Space>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role='img'
        aria-label={t('每日收益趋势')}
        className='block w-full'
        style={{ minHeight: 160 }}
      >
        <title>{t('每日收益趋势')}</title>
        {[0, 0.5, 1].map((p) => (
          <g key={p}>
            <line
              x1='62'
              x2={width - 30}
              y1={y(min + (max - min) * p)}
              y2={y(min + (max - min) * p)}
              stroke='var(--semi-color-border)'
            />
            <text
              x='52'
              y={y(min + (max - min) * p) + 4}
              textAnchor='end'
              fill='var(--semi-color-text-2)'
              fontSize='11'
            >
              {(min + (max - min) * p).toFixed(2)}
            </text>
          </g>
        ))}
        {['income', 'cost'].map((field) => (
          <g key={field}>
            {days.map((d, i) => {
              if (typeof d[field] !== 'number') return null;
              const prior = days[i - 1]?.[field];
              return (
                <g key={d.name}>
                  {typeof prior === 'number' && (
                    <line
                      x1={x(i - 1)}
                      y1={y(prior)}
                      x2={x(i)}
                      y2={y(d[field])}
                      stroke={colors[field]}
                      strokeWidth='2'
                    />
                  )}
                  <circle cx={x(i)} cy={y(d[field])} r='3' fill={colors[field]}>
                    <title>
                      {d.name}: {cashMoney(d[field])}
                    </title>
                  </circle>
                </g>
              );
            })}
          </g>
        ))}
        {[0, Math.floor((days.length - 1) / 2), days.length - 1]
          .filter((v, i, a) => a.indexOf(v) === i)
          .map((i) => (
            <text
              key={i}
              x={x(i)}
              y='188'
              textAnchor='middle'
              fill='var(--semi-color-text-2)'
              fontSize='11'
            >
              {days[i]?.name}
            </text>
          ))}
      </svg>
    </div>
  );
}

export default function OpenRouterCashDashboard({
  account,
  active,
  view,
  onViewChange,
  refresh,
  t,
}) {
  const root = isRoot();
  const [range, setRange] = useState(() => cashDateRange());
  const [report, setReport] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [revision, setRevision] = useState(0);
  const [syncing, setSyncing] = useState(false);
  const syncingRef = useRef(false);
  const sequence = useRef(0);
  const [dimension, setDimension] = useState('models');
  const [search, setSearch] = useState('');
  const [detailPage, setDetailPage] = useState(1);
  const [draft, setDraft] = useState(null);
  const [candidates, setCandidates] = useState(null);
  const [candidateLoading, setCandidateLoading] = useState(false);
  const [funding, setFunding] = useState({ items: [], total: 0 });
  const [entries, setEntries] = useState({ items: [], total: 0 });
  const [fundingPage, setFundingPage] = useState(1);
  const [entryPage, setEntryPage] = useState(1);
  const [pendingOnly, setPendingOnly] = useState(true);
  const [userID, setUserID] = useState('');
  const [ledgerError, setLedgerError] = useState('');
  const [ledgerLoading, setLedgerLoading] = useState(false);
  const [history, setHistory] = useState(null);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyData, setHistoryData] = useState({ items: [], total: 0 });
  const base = `/api/upstream_accounts/${account.id}/cash`;
  const identity = `${account.id}:${account.enabled}:${account.access_token_masked}`;
  const changed = () => {
    setDraft(null);
    setCandidates(null);
    setRevision((n) => n + 1);
  };

  useEffect(() => {
    setDraft(null);
    setCandidates(null);
    setHistory(null);
    setEntryPage(1);
    setFundingPage(1);
  }, [identity]);
  useEffect(() => {
    const controller = new AbortController(),
      request = ++sequence.current;
    setReport(null);
    setError('');
    setLoading(false);
    if (!active) return () => controller.abort();
    setLoading(true);
    apiData(`${base}/report`, {
      params: { start_date: range[0], end_date: range[1] },
      signal: controller.signal,
    })
      .then((data) => {
        if (request === sequence.current) setReport(data);
      })
      .catch((e) => {
        if (!controller.signal.aborted && request === sequence.current)
          setError(e.message);
      })
      .finally(() => {
        if (request === sequence.current) setLoading(false);
      });
    return () => {
      ++sequence.current;
      controller.abort();
    };
  }, [identity, active, range, revision, refresh]);
  useEffect(() => {
    const controller = new AbortController();
    setFunding({ items: [], total: 0 });
    setEntries({ items: [], total: 0 });
    setLedgerError('');
    if (!active || !['review', 'flow'].includes(view))
      return () => controller.abort();
    setLedgerLoading(true);
    Promise.all([
      apiData('/api/cash_accounting/funding', {
        params: { page: fundingPage, pending: pendingOnly, user_id: userID },
        signal: controller.signal,
      }),
      apiData(`${base}/entries`, {
        params: { page: entryPage },
        signal: controller.signal,
      }),
    ])
      .then(([f, e]) => {
        if (!controller.signal.aborted) {
          setFunding(f);
          setEntries(e);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) setLedgerError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLedgerLoading(false);
      });
    return () => controller.abort();
  }, [
    identity,
    active,
    view,
    fundingPage,
    entryPage,
    pendingOnly,
    userID,
    revision,
    refresh,
  ]);
  useEffect(() => {
    const controller = new AbortController();
    setHistoryData({ items: [], total: 0 });
    if (!history) return () => controller.abort();
    apiData(`/api/cash_accounting/funding/${history.id}/receipts`, {
      params: { page: historyPage },
      signal: controller.signal,
    })
      .then((data) => {
        if (!controller.signal.aborted) setHistoryData(data);
      })
      .catch((e) => {
        if (!controller.signal.aborted) showError(t(e.message));
      });
    return () => controller.abort();
  }, [history, historyPage]);

  const sync = async () => {
    if (syncingRef.current) return;
    syncingRef.current = true;
    setSyncing(true);
    try {
      const { data } = await API.post(
        `${base}/sync`,
        {},
        { timeout: 260000, skipErrorHandler: true },
      );
      if (!data.success) throw new Error(data.message);
      showSuccess(t('同步成功'));
    } catch (e) {
      showError(t(e.message));
    } finally {
      syncingRef.current = false;
      setSyncing(false);
      setRevision((n) => n + 1);
    }
  };
  const loadCandidates = async () => {
    const current = sequence.current;
    setCandidateLoading(true);
    try {
      const data = await apiData(`${base}/bindings`, { timeout: 70000 });
      if (current === sequence.current) setCandidates(data);
    } catch (e) {
      if (current === sequence.current) showError(t(e.message));
    } finally {
      setCandidateLoading(false);
    }
  };
  const newEntry = () =>
    setDraft({
      type: 'entry',
      request_id: cashRequestID(),
      kind: 'purchase',
      date: cashDateRange(1)[0],
      credits: '',
      currency: 'CNY',
      gross_amount: '',
      fee_amount: '0',
      fx_rate: '1',
      reference: '',
      evidence: '',
    });
  const receipt = (lot) =>
    setDraft({
      type: 'receipt',
      request_id: cashRequestID(),
      lot_id: lot.id,
      user_id: lot.user_id,
      previous_id: lot.receipt_id,
      currency: lot.receipt?.currency || 'CNY',
      gross_amount: lot.receipt?.gross_amount || '',
      fee_amount: lot.receipt?.fee_amount || '0',
      fx_rate: lot.receipt?.fx_rate || '1',
      received_time: new Date(
        (lot.receipt?.received_at || lot.created_at) * 1000,
      )
        .toISOString()
        .slice(0, 16),
      evidence: lot.receipt?.evidence || '',
      note: '',
    });
  const rows = useMemo(
    () =>
      (report?.[dimension] || []).filter((r) =>
        r.name.toLowerCase().includes(search.trim().toLowerCase()),
      ),
    [report, dimension, search],
  );
  useEffect(() => setDetailPage(1), [dimension, search, range, identity]);
  const exportRows = () => {
    const url = URL.createObjectURL(
      new Blob([cashCSV(rows, t)], { type: 'text/csv;charset=utf-8' }),
    );
    const a = document.createElement('a');
    a.href = url;
    a.download = `openrouter-cash-${account.id}-${dimension}-${range.join('_')}-CNY.csv`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const metricColumns = [
    { title: t('归集对象'), dataIndex: 'name', width: 230 },
    ...[
      ['income', '分摊净到账收入'],
      ['cost', '上游现金成本'],
      ['profit', '现金贡献利润'],
    ].map(([key, label]) => ({
      title: `${t(label)} (CNY)`,
      dataIndex: key,
      width: 170,
      render: cashMoney,
    })),
    { title: t('请求数'), dataIndex: 'requests', width: 100 },
    {
      title: t('核实状态'),
      dataIndex: 'status',
      width: 110,
      render: (v) => (
        <Tag color={v === 'verified' ? 'green' : 'orange'}>
          {t(v === 'verified' ? '已核实' : '待核对')}
        </Tag>
      ),
    },
  ];
  const fundingColumns = [
    { title: t('资金批次'), width: 110, render: (_, r) => `#${r.id}` },
    { title: t('用户 ID'), dataIndex: 'user_id', width: 100 },
    {
      title: t('来源'),
      width: 220,
      render: (_, r) => (
        <>
          <Text>{t(sourceNames[r.source_type] || '待核对')}</Text>
          <Text className='block break-all' size='small' type='tertiary'>
            {r.source_ref}
          </Text>
        </>
      ),
    },
    {
      title: t('原始 / 剩余额度'),
      width: 160,
      render: (_, r) =>
        `${(r.total_quota / (funding.quota_per_unit || 500000)).toFixed(4)} / ${(r.remaining_quota / (funding.quota_per_unit || 500000)).toFixed(4)}`,
    },
    {
      title: t('现金基础'),
      width: 140,
      render: (_, r) =>
        r.source_type === 'gift'
          ? cashMoney(0)
          : cashMoney(r.receipt ? r.receipt.net_cny_micros / 1e6 : null),
    },
    {
      title: t('操作'),
      width: 200,
      render: (_, r) => (
        <Space>
          {root && r.source_type !== 'gift' && (
            <Button size='small' onClick={() => receipt(r)}>
              {t(r.receipt_id ? '更正' : '核实')}
            </Button>
          )}
          <Button
            size='small'
            disabled={!r.receipt_id}
            onClick={() => {
              setHistory(r);
              setHistoryPage(1);
            }}
          >
            {t('审计记录')}
          </Button>
        </Space>
      ),
    },
  ];
  const entryColumns = [
    { title: t('日期'), dataIndex: 'date', width: 115 },
    {
      title: t('账单类型'),
      dataIndex: 'kind',
      width: 145,
      render: (v) => t(cashEntryKinds[v] || v),
    },
    { title: t('账单编号'), dataIndex: 'reference', width: 170 },
    {
      title: 'credits (USD)',
      dataIndex: 'credits_nanos',
      width: 120,
      render: (v) => (v / 1e9).toFixed(6),
    },
    {
      title: t('现金基础'),
      dataIndex: 'cost_cny_micros',
      width: 150,
      render: (v) => cashMoney(v / 1e6),
    },
    { title: t('凭证引用'), dataIndex: 'evidence', width: 200 },
    {
      title: t('操作'),
      width: 220,
      render: (_, r) =>
        r.voided_at ? (
          <Text type='tertiary'>
            {t('已作废')} · #{r.voided_by} · {dateTime(r.voided_at)} ·{' '}
            {r.void_reason}
          </Text>
        ) : root ? (
          <Button
            size='small'
            type='danger'
            onClick={() =>
              setDraft({
                type: 'void',
                id: r.id,
                reference: r.reference,
                reason: '',
              })
            }
          >
            {t('作废')}
          </Button>
        ) : (
          '—'
        ),
    },
  ];
  const summary = report?.summary,
    flow = report?.cash_flow;
  const enabled = report?.config?.enabled_at > 0;
  return (
    <div className='space-y-4' data-testid='openrouter-cash-dashboard'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <div>
          <Text strong className='text-lg'>
            {t('OpenRouter 现金收益')}
          </Text>
          <Text type='tertiary' className='block'>
            {t('UTC 完整日 · 折合人民币')}
          </Text>
        </div>
        <Space wrap>
          <Input
            type='date'
            aria-label={t('开始日期')}
            value={range[0]}
            onChange={(v) => setRange((r) => [v, r[1]])}
            style={{ width: 150 }}
          />
          <Text>—</Text>
          <Input
            type='date'
            aria-label={t('结束日期')}
            value={range[1]}
            onChange={(v) => setRange((r) => [r[0], v])}
            style={{ width: 150 }}
          />
          <Button onClick={() => setRange(cashDateRange(7))}>
            {t('最近 7 天')}
          </Button>
          <Button onClick={() => setRange(cashDateRange(30))}>
            {t('最近 30 天')}
          </Button>
          <Button loading={syncing} disabled={!account.enabled} onClick={sync}>
            {t('同步日账')}
          </Button>
        </Space>
      </div>
      {error && <Banner type='danger' description={t(error)} />}
      <Spin spinning={loading}>
        {!report ? (
          <Empty description={t(loading ? '加载中' : '暂无数据')} />
        ) : (
          <div className='space-y-4'>
            {!enabled && (
              <Banner
                type='info'
                description={t(
                  '从启用时开始记录资金来源。历史余额、套餐和未追溯的变动保持待核对；不改变用户扣费。',
                )}
                closeIcon={null}
                children={
                  root ? (
                    <Button onClick={() => setDraft({ type: 'enable' })}>
                      {t('启用现金收益')}
                    </Button>
                  ) : (
                    t('请联系超级管理员完成核对')
                  )
                }
              />
            )}
            {view === 'profit' && (
              <>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <Tag
                    color={summary.status === 'verified' ? 'green' : 'orange'}
                  >
                    {t(summary.status === 'verified' ? '已核实' : '待核对')}
                  </Tag>
                  {report.issues.length > 0 && (
                    <Button
                      theme='borderless'
                      onClick={() => onViewChange('review')}
                    >
                      {t('查看待核对项')} ({report.issues.length})
                    </Button>
                  )}
                </div>
                <div className='grid grid-cols-2 gap-3 xl:grid-cols-4'>
                  {[
                    [
                      '分摊净到账收入',
                      cashMoney(summary.income),
                      `${t('已核实收入')} ${cashMoney(summary.known_income)}`,
                    ],
                    [
                      '上游现金成本',
                      cashMoney(summary.cost),
                      `${t('已核实成本')} ${cashMoney(summary.known_cost)}`,
                    ],
                    [
                      '现金贡献利润',
                      cashMoney(summary.profit),
                      t('净到账收入减对应上游成本'),
                    ],
                    [
                      '利润率',
                      summary.margin == null
                        ? '—'
                        : `${summary.margin.toFixed(2)}%`,
                      t('以分摊净到账收入为分母'),
                    ],
                  ].map(([label, value, hint]) => (
                    <Card key={label}>
                      <Text type='tertiary'>{t(label)}</Text>
                      <div
                        className='my-2 text-2xl font-semibold tabular-nums'
                        style={
                          label === '现金贡献利润' && summary.profit != null
                            ? {
                                color:
                                  summary.profit >= 0
                                    ? 'var(--semi-color-success)'
                                    : 'var(--semi-color-danger)',
                              }
                            : {}
                        }
                      >
                        {value}
                      </div>
                      <Text size='small' type='tertiary'>
                        {hint}
                      </Text>
                    </Card>
                  ))}
                </div>
                <Card title={t('每日收益趋势')}>
                  <Trend days={report.days} t={t} />
                  <Text type='tertiary' size='small'>
                    {t('待核对的日期不连线；已核实部分不代表完整利润。')}
                  </Text>
                </Card>
                <Card title={t('收益明细')}>
                  <div className='mb-4 flex flex-wrap gap-3'>
                    <Select
                      aria-label={t('归集维度')}
                      value={dimension}
                      onChange={setDimension}
                      optionList={[
                        ['models', '上游模型'],
                        ['channels', '渠道'],
                        ['days', '日期'],
                      ].map(([value, label]) => ({ value, label: t(label) }))}
                    />
                    <Input
                      aria-label={t('搜索')}
                      placeholder={t('搜索')}
                      value={search}
                      onChange={setSearch}
                      showClear
                      style={{ maxWidth: 260 }}
                    />
                    <Button onClick={exportRows} disabled={!rows.length}>
                      {t('导出 CSV')}
                    </Button>
                  </div>
                  {dimension === 'channels' && (
                    <Text type='tertiary' className='mb-3 block'>
                      {t('官方日账没有渠道维度，渠道成本和利润保持待核对。')}
                    </Text>
                  )}
                  <Table
                    rowKey='name'
                    columns={metricColumns}
                    dataSource={rows.slice(
                      (detailPage - 1) * 20,
                      detailPage * 20,
                    )}
                    scroll={{ x: 950 }}
                    pagination={{
                      currentPage: detailPage,
                      pageSize: 20,
                      total: rows.length,
                      onPageChange: setDetailPage,
                    }}
                  />
                </Card>
                <Text type='tertiary' className='block' size='small'>
                  {t(
                    '按资金批次分摊净到账收入，按 UTC 日加权平均分摊购款成本；包含已录入费用，不含未录入的间接开支。',
                  )}
                </Text>
              </>
            )}
            {view === 'flow' && (
              <>
                <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                  {[
                    [
                      t('全站客户净到账'),
                      cashMoney(flow.site_confirmed_receipts),
                    ],
                    [t('此账户实际付款'), cashMoney(flow.account_payments)],
                    [t('全站待核实批次'), flow.pending_funding_lots],
                  ].map(([label, value]) => (
                    <Card key={label}>
                      <Text type='tertiary'>{label}</Text>
                      <div className='mt-2 text-2xl font-semibold tabular-nums'>
                        {value}
                      </div>
                    </Card>
                  ))}
                </div>
                <Banner
                  type='info'
                  description={t(
                    '客户到账是全站范围，上游付款仅属于所选账户，不合并为净现金流。未消费的充值不提前计入利润。',
                  )}
                />
              </>
            )}
            {view === 'review' && (
              <>
                <Card title={t('数据核对与成本归属')}>
                  {report.issues.length === 0 ? (
                    <Text>{t('所选范围已核实')}</Text>
                  ) : (
                    <div className='space-y-3'>
                      {report.issues.map((issue) => (
                        <div
                          key={issue.code}
                          className='flex items-start justify-between gap-4'
                        >
                          <Text>{t(issue.message)}</Text>
                          <Tag color='orange'>{issue.count}</Tag>
                        </div>
                      ))}
                    </div>
                  )}
                </Card>
                <Card title={t('调用密钥归属')}>
                  <Space wrap>
                    <Text>
                      {t('已关联密钥')}：{report.bindings.length}
                    </Text>
                    <Button
                      loading={candidateLoading}
                      disabled={!account.enabled}
                      onClick={loadCandidates}
                    >
                      {t('核对可关联渠道')}
                    </Button>
                  </Space>
                  {candidates && (
                    <div className='mt-3 space-y-3'>
                      {candidates.length === 0 ? (
                        <Empty
                          description={t('没有找到属于此账户的本站调用密钥')}
                        />
                      ) : (
                        candidates.map((c) => (
                          <div
                            key={c.key_hash}
                            className='flex flex-wrap items-center justify-between gap-3 rounded border p-3'
                            style={{ borderColor: 'var(--semi-color-border)' }}
                          >
                            <div>
                              <Text>{c.key_hash.slice(0, 12)}…</Text>
                              <Text type='tertiary' className='block'>
                                {c.channels
                                  .map((ch) => `${ch.name} (#${ch.id})`)
                                  .join(' · ')}
                              </Text>
                              <Text>
                                {t(
                                  c.binding?.site_only
                                    ? '仅用于本站'
                                    : '待核对',
                                )}
                              </Text>
                            </div>
                            {root && enabled && (
                              <Button
                                disabled={!c.channels.length}
                                onClick={() =>
                                  setDraft({
                                    type: 'binding',
                                    key_hash: c.key_hash,
                                    channels: c.channels,
                                    site_only: c.binding?.site_only || false,
                                  })
                                }
                              >
                                {t(c.binding ? '核实' : '关联')}
                              </Button>
                            )}
                          </div>
                        ))
                      )}
                    </div>
                  )}
                </Card>
              </>
            )}
            {['flow', 'review'].includes(view) && (
              <>
                {!root && (
                  <Text type='tertiary'>{t('请联系超级管理员完成核对')}</Text>
                )}
                {ledgerError && (
                  <Banner type='danger' description={t(ledgerError)} />
                )}
                <Card
                  title={t('上游购款与费用')}
                  headerExtraContent={
                    root && (
                      <Button onClick={newEntry}>{t('录入上游账单')}</Button>
                    )
                  }
                >
                  <Table
                    rowKey='id'
                    columns={entryColumns}
                    dataSource={entries.items}
                    loading={ledgerLoading}
                    scroll={{ x: 1200 }}
                    pagination={{
                      currentPage: entryPage,
                      pageSize: 20,
                      total: entries.total,
                      onPageChange: setEntryPage,
                    }}
                  />
                </Card>
                <Card title={t('全站资金来源')}>
                  <Space wrap className='mb-4'>
                    <label className='flex items-center gap-2'>
                      <Switch
                        aria-label={t('仅待核实')}
                        checked={pendingOnly}
                        onChange={(v) => {
                          setPendingOnly(v);
                          setFundingPage(1);
                        }}
                      />
                      {t('仅待核实')}
                    </label>
                    <Input
                      aria-label={t('筛选用户 ID')}
                      placeholder={t('筛选用户 ID')}
                      value={userID}
                      onChange={(v) => {
                        if (/^\d*$/.test(v)) {
                          setUserID(v);
                          setFundingPage(1);
                        }
                      }}
                      showClear
                      style={{ width: 180 }}
                    />
                  </Space>
                  <Table
                    rowKey='id'
                    columns={fundingColumns}
                    dataSource={funding.items}
                    loading={ledgerLoading}
                    scroll={{ x: 1000 }}
                    pagination={{
                      currentPage: fundingPage,
                      pageSize: 20,
                      total: funding.total,
                      onPageChange: setFundingPage,
                    }}
                  />
                </Card>
              </>
            )}
            <Text type='tertiary' size='small'>
              {t('最近成功同步')}：{dateTime(report.sync.last_success)} ·{' '}
              {t('采集起点')}：{dateTime(report.config.enabled_at)}
            </Text>
          </div>
        )}
      </Spin>
      {draft && (
        <CashLedgerDialog
          key={`${account.id}:${draft.type}:${draft.request_id || draft.key_hash || draft.id || ''}`}
          initial={draft}
          accountID={account.id}
          onClose={() => setDraft(null)}
          onSaved={changed}
          t={t}
        />
      )}
      <Modal
        visible={!!history}
        title={t('审计记录')}
        footer={null}
        onCancel={() => setHistory(null)}
        width={900}
        style={{ maxWidth: 'calc(100vw - 24px)' }}
      >
        <Table
          rowKey='id'
          scroll={{ x: 1100 }}
          dataSource={historyData.items}
          columns={[
            { title: t('版本'), dataIndex: 'id', width: 80 },
            {
              title: t('现金基础'),
              dataIndex: 'net_cny_micros',
              width: 140,
              render: (v) => cashMoney(v / 1e6),
            },
            {
              title: t('原币金额（不含手续费）'),
              dataIndex: 'gross_amount',
              width: 180,
            },
            { title: t('原币手续费'), dataIndex: 'fee_amount', width: 130 },
            { title: t('原币币种'), dataIndex: 'currency', width: 100 },
            {
              title: t('实际汇率（人民币 / 原币）'),
              dataIndex: 'fx_rate',
              width: 180,
            },
            { title: t('凭证引用'), dataIndex: 'evidence', width: 180 },
            { title: t('核实或更正说明'), dataIndex: 'note', width: 200 },
            { title: t('操作人'), dataIndex: 'created_by', width: 90 },
            {
              title: t('时间'),
              dataIndex: 'created_at',
              width: 180,
              render: dateTime,
            },
          ]}
          pagination={{
            currentPage: historyPage,
            pageSize: 20,
            total: historyData.total,
            onPageChange: setHistoryPage,
          }}
        />
      </Modal>
    </div>
  );
}
