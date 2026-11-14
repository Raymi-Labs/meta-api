import React, { useMemo, useState, useEffect } from 'react';
import {
  Banner,
  Button,
  Card,
  Input,
  Space,
  Table,
  Typography,
} from '@douyinfe/semi-ui';
import { renderQuota } from '../../../helpers';
import { cashCSV } from '../cashFinance';
import {
  openRouterUSD,
  summarizeOpenRouterChannels,
  summarizeOpenRouterConsumption,
} from '../openRouterFinance';
const { Text } = Typography;
const localQuota = (value) => (value == null ? '--' : renderQuota(value, 6));

export default function OpenRouterConsumption({ data, loading, t }) {
  const [search, setSearch] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [dimension, setDimension] = useState('channels');
  const [page, setPage] = useState(1);
  useEffect(() => {
    setStart(data?.start_date || '');
    setEnd(data?.end_date || '');
  }, [data?.start_date, data?.end_date]);
  useEffect(() => setPage(1), [search, start, end, dimension]);
  const channels = dimension === 'channels';
  const daily = dimension === 'days';
  // Summary is scoped to dates, never to a table search or grouping choice.
  const totals = useMemo(
    () => summarizeOpenRouterConsumption(data, { start, end }),
    [data, start, end],
  );
  const rows = useMemo(
    () =>
      channels
        ? summarizeOpenRouterChannels(data, { start, end, search })
        : summarizeOpenRouterConsumption(data, { start, end, daily }).filter(
            (r) =>
              (daily ? r.date : r.model)
                .toLowerCase()
                .includes(search.trim().toLowerCase()),
          ),
    [data, channels, start, end, search, daily],
  );
  const sum = (field) => totals.reduce((n, row) => n + (row[field] || 0), 0);
  const exportRows = () => {
    const csvRows = rows.map((r) => ({
      name: channels ? r.name : daily ? r.date : r.model,
      requests: r.site_requests,
      quota: r.quota,
    }));
    const url = URL.createObjectURL(
      new Blob([cashCSV(csvRows, t, localQuota)], {
        type: 'text/csv;charset=utf-8',
      }),
    );
    const a = document.createElement('a');
    a.href = url;
    a.download = `openrouter-${dimension}-${start}_${end}.csv`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const columns = [
    {
      title: t(channels ? '渠道' : daily ? '日期' : '模型'),
      dataIndex: channels ? 'name' : daily ? 'date' : 'model',
      width: 260,
    },
    ...(!channels
      ? [
          {
            title: t('上游消耗') + ' (USD)',
            dataIndex: 'usage',
            render: openRouterUSD,
            width: 160,
          },
        ]
      : []),
    {
      title: t('本站消耗'),
      dataIndex: 'quota',
      render: localQuota,
      width: 160,
    },
    ...(!channels
      ? [
          {
            title: t('上游请求数'),
            dataIndex: 'upstream_requests',
            render: (v) => v ?? '--',
            width: 130,
          },
        ]
      : []),
    {
      title: t('本站请求数'),
      dataIndex: 'site_requests',
      render: (v) => v ?? '--',
      width: 130,
    },
    ...(!channels
      ? [
          {
            title: t('BYOK 外部成本') + ' (USD)',
            dataIndex: 'byok',
            render: openRouterUSD,
            width: 180,
          },
        ]
      : []),
  ];
  const cards = (items) => (
    <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
      {items.map(([label, value]) => (
        <Card key={label}>
          <Text type='tertiary'>{label}</Text>
          <div className='mt-2 text-2xl font-semibold tabular-nums'>
            {value}
          </div>
        </Card>
      ))}
    </div>
  );
  return (
    <div className='space-y-4'>
      <Text type='tertiary'>
        {t('后台每 5 分钟更新快照')}
        {data?.fetched_at > 0 &&
          ` · ${t('快照时间')}：${new Date(data.fetched_at * 1000).toLocaleString()}`}
      </Text>
      {data?.refreshing && (
        <Banner
          type='info'
          description={t('正在后台准备快照，完成后自动显示')}
        />
      )}
      {data?.refresh_error && (
        <Banner
          type='warning'
          description={t('部分数据刷新失败，保留上次快照，请留意快照时间')}
        />
      )}
      {[data?.credits_error, data?.activity_error, data?.site_error]
        .filter(Boolean)
        .map((error, i) => (
          <Banner key={i} type='warning' description={error} />
        ))}
      <Card title={t('充值与消耗')}>
        <Text type='tertiary' className='mb-3 block'>
          {t('账户累计')}
        </Text>
        {cards([
          [
            t('累计购入额度') + ' (USD)',
            openRouterUSD(data?.credits?.total_credits),
          ],
          [t('累计消费') + ' (USD)', openRouterUSD(data?.credits?.total_usage)],
          [
            t('余额') + ' (USD)',
            openRouterUSD(
              data?.credits
                ? data.credits.total_credits - data.credits.total_usage
                : null,
            ),
          ],
        ])}
      </Card>
      <Card title={t('区间消耗')}>
        <Text type='tertiary'>
          {t(
            '最近 30 个完整 UTC 日；上游为账户消费，本站为当前归属渠道的留存消费日志。密钥更换、日志清理及站外调用可能造成差异；消耗对照不代表现金利润。',
          )}
        </Text>
        <Space wrap className='my-4'>
          <input
            type='date'
            aria-label={t('开始日期')}
            value={start}
            min={data?.start_date}
            max={end || data?.end_date}
            onChange={(e) => {
              const value = e.target.value;
              if (value >= data?.start_date && value <= end) setStart(value);
            }}
          />
          <span>—</span>
          <input
            type='date'
            aria-label={t('结束日期')}
            value={end}
            min={start || data?.start_date}
            max={data?.end_date}
            onChange={(e) => {
              const value = e.target.value;
              if (value >= start && value <= data?.end_date) setEnd(value);
            }}
          />
          {[7, 30].map((days) => (
            <Button
              key={days}
              disabled={!data?.end_date}
              onClick={() => {
                const d = new Date(`${data.end_date}T00:00:00Z`);
                d.setUTCDate(d.getUTCDate() - days + 1);
                setStart(d.toISOString().slice(0, 10));
                setEnd(data.end_date);
              }}
            >
              {days} {t('天')}
            </Button>
          ))}
        </Space>
        {cards([
          [
            t('上游消耗') + ' (USD)',
            openRouterUSD(data?.activity ? sum('usage') : null),
          ],
          [
            t('本站消耗'),
            localQuota(data?.site_activity ? sum('quota') : null),
          ],
          [
            t('本站请求数'),
            data?.site_activity ? sum('site_requests').toLocaleString() : '--',
          ],
        ])}
        <Space wrap className='my-4'>
          {[
            ['channels', '按渠道'],
            ['models', '按模型'],
            ['days', '按日期'],
          ].map(([key, label]) => (
            <Button
              key={key}
              theme={dimension === key ? 'solid' : 'light'}
              onClick={() => {
                setDimension(key);
                setSearch('');
              }}
            >
              {t(label)}
            </Button>
          ))}
          <Input
            style={{ width: 240 }}
            placeholder={t('搜索')}
            aria-label={t('搜索')}
            value={search}
            onChange={setSearch}
            showClear
          />
          <Button onClick={exportRows} disabled={!rows.length}>
            {t('导出 CSV')}
          </Button>
        </Space>
        <Table
          columns={columns}
          dataSource={rows.slice((page - 1) * 20, page * 20)}
          rowKey='key'
          loading={loading && !data}
          scroll={{ x: channels ? 550 : 1020 }}
          pagination={{
            currentPage: page,
            pageSize: 20,
            total: rows.length,
            onPageChange: setPage,
          }}
          empty={t('暂无数据')}
        />
        <Text type='tertiary' size='small'>
          {t(
            '本站消耗沿用平台额度显示单位；未匹配到的单侧数据以 -- 显示，BYOK 外部成本单列。',
          )}
        </Text>
      </Card>
    </div>
  );
}
