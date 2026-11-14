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
import { API, renderQuota } from '../../../helpers';
import { cashCSV, cashDateRange } from '../cashFinance';
import { openRouterUSD } from '../openRouterFinance';
const { Text } = Typography;
const dateTime = (v) => (v ? new Date(v * 1000).toLocaleString() : '—');
const apiData = async (url, config) => {
  const { data } = await API.get(url, { skipErrorHandler: true, ...config });
  if (!data.success) throw new Error(data.message);
  return data.data;
};
export default function OpenRouterCashDashboard({
  account,
  active,
  view,
  refresh,
  t,
}) {
  const [range, setRange] = useState(() => cashDateRange());
  const [report, setReport] = useState(null),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(false);
  const [dimension, setDimension] = useState('channels'),
    [search, setSearch] = useState(''),
    [page, setPage] = useState(1);
  const sequence = useRef(0);
  const identity = `${account.id}:${account.enabled}:${account.access_token_masked}`;
  const base = `/api/upstream_accounts/${account.id}/cash`;
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
  }, [identity, active, range, refresh]);

  const op = report?.operations;
  const rows = useMemo(
    () =>
      (report?.[dimension] || []).filter((r) =>
        r.name.toLowerCase().includes(search.trim().toLowerCase()),
      ),
    [report, dimension, search],
  );
  useEffect(() => setPage(1), [dimension, search, range, identity]);
  const exportRows = () => {
    const url = URL.createObjectURL(
      new Blob([cashCSV(rows, t, (v) => renderQuota(v, 6))], {
        type: 'text/csv;charset=utf-8',
      }),
    );
    const a = document.createElement('a');
    a.href = url;
    a.download = `openrouter-usage-${account.id}-${range.join('_')}.csv`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const balance = op?.credits
    ? op.credits.total_credits - op.credits.total_usage
    : null;
  const cards =
    view === 'flow'
      ? [
          [
            t('累计购入额度') + ' (USD)',
            openRouterUSD(op?.credits?.total_credits),
          ],
          [t('累计消费') + ' (USD)', openRouterUSD(op?.credits?.total_usage)],
          [t('余额') + ' (USD)', openRouterUSD(balance)],
        ]
      : [
          [
            t('本站消耗'),
            !op?.local_available
              ? '—'
              : renderQuota(report?.summary?.quota || 0, 6),
          ],
          [
            t('本站请求数'),
            !op?.local_available ? '—' : report?.summary?.requests || 0,
          ],
          [t('上游消耗') + ' (USD)', openRouterUSD(op?.usage)],
        ];
  return (
    <div className='mt-3 space-y-4' data-testid='openrouter-cash-dashboard'>
      <Space wrap>
        <Input
          type='date'
          aria-label={t('开始日期')}
          value={range[0]}
          onChange={(v) => setRange((r) => [v, r[1]])}
          style={{ width: 160 }}
        />
        <Text>—</Text>
        <Input
          type='date'
          aria-label={t('结束日期')}
          value={range[1]}
          onChange={(v) => setRange((r) => [r[0], v])}
          style={{ width: 160 }}
        />
        <Button onClick={() => setRange(cashDateRange(7))}>
          {t('最近 7 天')}
        </Button>
        <Button onClick={() => setRange(cashDateRange(30))}>
          {t('最近 30 天')}
        </Button>
      </Space>
      <Text type='tertiary' className='block'>
        {t('后台每 5 分钟更新快照')} · {t('快照时间')}：
        {dateTime(report?.generated_at)}
      </Text>
      {error && <Banner type='warning' description={t(error)} />}
      {report?.refreshing && (
        <Banner
          type='info'
          description={t('正在后台准备快照，完成后自动显示')}
        />
      )}
      <Spin spinning={loading && !report}>
        {!report?.generated_at ? (
          <Empty description={t('加载中')} />
        ) : (
          <>
            {[
              op?.credits_error,
              view === 'profit' ? op?.usage_error : '',
              view === 'profit' ? op?.local_error : '',
            ]
              .filter(Boolean)
              .map((e, i) => (
                <Banner key={i} type='warning' description={t(e)} />
              ))}
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
              {cards.map(([label, value]) => (
                <Card key={label}>
                  <Text type='tertiary'>{label}</Text>
                  <div className='my-2 text-2xl font-semibold'>{value}</div>
                </Card>
              ))}
            </div>
            {view === 'flow' ? (
              <Text type='tertiary'>
                {t(
                  'OpenRouter 金额以 USD 展示；累计购入额度不含支付手续费，逐笔充值订单请查看官方账单。',
                )}
              </Text>
            ) : (
              <Card>
                <Text type='tertiary' className='mb-3 block'>
                  {t(
                    '本站消耗来自当前归属渠道的留存日志；闪电额度与上游 USD 不能相减作为现金利润。',
                  )}
                </Text>
                <Space wrap className='mb-3'>
                  <Select
                    aria-label={t('归集维度')}
                    value={dimension}
                    onChange={setDimension}
                    optionList={['channels', 'models', 'days'].map(
                      (value, i) => ({
                        value,
                        label: t(['渠道', '模型', '日期'][i]),
                      }),
                    )}
                  />
                  <Input
                    aria-label={t('搜索')}
                    placeholder={t('搜索')}
                    value={search}
                    onChange={setSearch}
                    showClear
                    style={{ maxWidth: 240 }}
                  />
                  <Button onClick={exportRows} disabled={!rows.length}>
                    {t('导出 CSV')}
                  </Button>
                </Space>
                <Table
                  rowKey='name'
                  columns={[
                    { title: t('归集对象'), dataIndex: 'name', width: 240 },
                    {
                      title: t('本站消耗'),
                      dataIndex: 'quota',
                      render: (v) => renderQuota(v, 6),
                      width: 180,
                    },
                    { title: t('请求数'), dataIndex: 'requests', width: 120 },
                  ]}
                  dataSource={rows.slice((page - 1) * 20, page * 20)}
                  scroll={{ x: 540 }}
                  pagination={{
                    currentPage: page,
                    pageSize: 20,
                    total: rows.length,
                    onPageChange: setPage,
                  }}
                />
              </Card>
            )}
          </>
        )}
      </Spin>
    </div>
  );
}
