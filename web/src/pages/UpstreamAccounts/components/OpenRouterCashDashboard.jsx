import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Select,
  Space,
  Spin,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import {
  API,
  isRoot,
  renderQuota,
  showError,
  showSuccess,
} from '../../../helpers';
import {
  cashCSV,
  cashDateRange,
  cashMoney,
  cashRequestID,
} from '../cashFinance';
import CashLedgerDialog, { cashEntryKinds } from './CashLedgerDialog';

const { Text } = Typography;
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
  if (!values.length) return <Empty description={t('暂无数据')} />;
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
  const [entries, setEntries] = useState({ items: [], total: 0 });
  const [entryPage, setEntryPage] = useState(1);
  const [ledgerError, setLedgerError] = useState('');
  const [ledgerLoading, setLedgerLoading] = useState(false);
  const base = `/api/upstream_accounts/${account.id}/cash`;
  const identity = `${account.id}:${account.enabled}:${account.access_token_masked}`;
  const changed = () => {
    setDraft(null);
    setRevision((n) => n + 1);
  };

  useEffect(() => {
    setDraft(null);
    setEntryPage(1);
  }, [identity]);
  const displayedKey = useRef('');
  useEffect(() => {
    const controller = new AbortController();
    const request = ++sequence.current;
    const key = `${identity}:${range.join(':')}`;
    let timer;
    if (displayedKey.current !== key) {
      setReport(null);
      displayedKey.current = key;
    }
    setError('');
    if (!active) return () => controller.abort();
    const load = async () => {
      setLoading(true);
      let delay = 5 * 60 * 1000;
      try {
        const data = await apiData(`${base}/report`, {
          params: { start_date: range[0], end_date: range[1] },
          signal: controller.signal,
        });
        if (request !== sequence.current) return;
        setReport(data);
        setError(data.refresh_error || '');
        if (data.refreshing) delay = 2000;
      } catch (e) {
        if (!controller.signal.aborted && request === sequence.current)
          setError(e.message);
      } finally {
        if (!controller.signal.aborted && request === sequence.current) {
          setLoading(false);
          timer = setTimeout(load, delay);
        }
      }
    };
    load();
    return () => {
      ++sequence.current;
      clearTimeout(timer);
      controller.abort();
    };
  }, [identity, active, range, revision, refresh]);
  useEffect(() => {
    const controller = new AbortController();
    setEntries({ items: [], total: 0 });
    setLedgerError('');
    if (!active || view !== 'flow') return () => controller.abort();
    setLedgerLoading(true);
    apiData(`${base}/entries`, {
      params: { page: entryPage },
      signal: controller.signal,
    })
      .then((data) => {
        if (!controller.signal.aborted) setEntries(data);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setLedgerError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLedgerLoading(false);
      });
    return () => controller.abort();
  }, [identity, active, view, entryPage, revision, refresh]);
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
      new Blob([cashCSV(rows, t, (v) => renderQuota(v, 6))], {
        type: 'text/csv;charset=utf-8',
      }),
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
      title: t('本站消耗'),
      dataIndex: 'quota',
      width: 150,
      render: (v) => (v == null ? '—' : renderQuota(v, 6)),
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
            {t(view === 'flow' ? '现金流' : 'OpenRouter 现金收益')}
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
      <Text type='tertiary' className='block'>
        {t('后台每 5 分钟更新快照')} · {t('快照时间')}：
        {dateTime(report?.generated_at)}
      </Text>
      {report?.refreshing && (
        <Banner
          type='info'
          description={t('正在后台准备快照，完成后自动显示')}
        />
      )}
      <Spin spinning={loading && !report}>
        {!report?.generated_at ? (
          <Empty description={t(loading ? '加载中' : '暂无数据')} />
        ) : (
          <div className='space-y-4'>
            {!enabled && (
              <Banner
                type='info'
                description={t('暂无数据')}
                closeIcon={null}
                children={
                  root ? (
                    <Button onClick={() => setDraft({ type: 'enable' })}>
                      {t('启用现金收益')}
                    </Button>
                  ) : null
                }
              />
            )}
            {view === 'profit' && (
              <>
                <div className='grid grid-cols-2 gap-3 xl:grid-cols-4'>
                  {[
                    ['分摊净到账收入', cashMoney(summary.income), ''],
                    ['上游现金成本', cashMoney(summary.cost), ''],
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
            {view === 'flow' && (
              <>
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
    </div>
  );
}
