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
import {
  openRouterUSD,
  summarizeOpenRouterConsumption,
} from '../openRouterFinance';
const { Text } = Typography;
const localQuota = (value) => (value == null ? '--' : renderQuota(value, 6));

export default function OpenRouterConsumption({ data, loading, t }) {
  const [search, setSearch] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [daily, setDaily] = useState(false);
  const [page, setPage] = useState(1);
  useEffect(() => {
    setStart(data?.start_date || '');
    setEnd(data?.end_date || '');
  }, [data?.start_date, data?.end_date]);
  useEffect(() => setPage(1), [search, start, end, daily]);
  const rows = useMemo(
    () => summarizeOpenRouterConsumption(data, { start, end, search, daily }),
    [data, start, end, search, daily],
  );
  const sum = (field) => rows.reduce((n, row) => n + (row[field] || 0), 0);
  const columns = [
    {
      title: t(daily ? '日期' : '模型'),
      dataIndex: daily ? 'date' : 'model',
      width: 260,
    },
    {
      title: t('上游消耗') + ' (USD)',
      dataIndex: 'usage',
      render: openRouterUSD,
      width: 160,
    },
    {
      title: t('本站消耗'),
      dataIndex: 'quota',
      render: localQuota,
      width: 160,
    },
    {
      title: t('上游请求数'),
      dataIndex: 'upstream_requests',
      render: (v) => v ?? '--',
      width: 130,
    },
    {
      title: t('本站请求数'),
      dataIndex: 'site_requests',
      render: (v) => v ?? '--',
      width: 130,
    },
    {
      title: t('BYOK 外部成本') + ' (USD)',
      dataIndex: 'byok',
      render: openRouterUSD,
      width: 180,
    },
  ];
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

      {data?.credits_error && (
        <Banner type='warning' description={data.credits_error} />
      )}
      {data?.activity_error && (
        <Banner type='warning' description={data.activity_error} />
      )}
      {data?.site_error && (
        <Banner type='warning' description={data.site_error} />
      )}
      <Card title={t('模型消耗看板')}>
        <Text type='tertiary'>
          {t(
            '最近 30 个完整 UTC 日；上游为账户消费，本站为当前归属渠道的留存消费日志。密钥更换、日志清理及站外调用可能造成差异；消耗对照不代表现金利润。',
          )}
        </Text>
        <div className='my-4 flex flex-wrap items-center gap-3'>
          <Input
            style={{ width: 240 }}
            placeholder={t('搜索模型')}
            aria-label={t('搜索模型')}
            value={search}
            onChange={setSearch}
            showClear
          />
          <input
            type='date'
            aria-label={t('开始日期')}
            value={start}
            min={data?.start_date}
            max={end || data?.end_date}
            onChange={(e) => setStart(e.target.value)}
          />
          <span>—</span>
          <input
            type='date'
            aria-label={t('结束日期')}
            value={end}
            min={start || data?.start_date}
            max={data?.end_date}
            onChange={(e) => setEnd(e.target.value)}
          />
          {[7, 30].map((days) => (
            <Button
              key={days}
              onClick={() => {
                if (!data?.end_date) return;
                const d = new Date(`${data.end_date}T00:00:00Z`);
                d.setUTCDate(d.getUTCDate() - days + 1);
                setStart(d.toISOString().slice(0, 10));
                setEnd(data.end_date);
              }}
            >
              {days} {t('天')}
            </Button>
          ))}
        </div>
        <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
          {[
            [
              t('上游消耗') + ' (USD)',
              openRouterUSD(data?.activity ? sum('usage') : null),
            ],
            [
              t('本站消耗'),
              localQuota(data?.site_activity ? sum('quota') : null),
            ],
            [
              t('钱包余额') + ' (USD)',
              openRouterUSD(
                data?.credits
                  ? data.credits.total_credits - data.credits.total_usage
                  : null,
              ),
            ],
            [
              t('本站请求数'),
              data?.site_activity
                ? sum('site_requests').toLocaleString()
                : '--',
            ],
          ].map(([label, value]) => (
            <Card key={label}>
              <Text type='tertiary'>{label}</Text>
              <div className='mt-2 text-2xl font-semibold tabular-nums'>
                {value}
              </div>
            </Card>
          ))}
        </div>
        <Space className='my-4'>
          <Button
            theme={!daily ? 'solid' : 'light'}
            onClick={() => setDaily(false)}
          >
            {t('按模型')}
          </Button>
          <Button
            theme={daily ? 'solid' : 'light'}
            onClick={() => setDaily(true)}
          >
            {t('按日期')}
          </Button>
        </Space>
        <Table
          columns={columns}
          dataSource={rows.slice((page - 1) * 20, page * 20)}
          rowKey='key'
          loading={loading}
          scroll={{ x: 1020 }}
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
