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

import React, { Suspense, lazy, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Tabs,
  TabPane,
  Table,
  Spin,
  Empty,
  Space,
  Tag,
} from '@douyinfe/semi-ui';
import { PieChart } from 'lucide-react';
import {
  modelToColor,
  renderNumber,
  renderQuota,
} from '../../helpers/dashboardFormat';
import UserBalanceTrendPanel from './UserBalanceTrendPanel';

const VChart = lazy(() =>
  import('@visactor/react-vchart').then((module) => ({
    default: module.VChart,
  })),
);

const ChartFallback = ({ t }) => (
  <div className='h-full flex items-center justify-center text-gray-400 text-sm'>
    {t('加载中...')}
  </div>
);

const formatPercent = (value) => `${(Number(value || 0) * 100).toFixed(1)}%`;

const MODEL_CHANNEL_RANGE_OPTIONS = [
  { label: '近24小时', value: 1 },
  { label: '近7天', value: 7 },
  { label: '近30天', value: 30 },
];

const formatDateTime = (timestamp) => {
  const date = new Date(Number(timestamp || 0) * 1000);
  if (Number.isNaN(date.getTime())) return '-';
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(
    2,
    '0',
  )}-${String(date.getDate()).padStart(2, '0')} ${String(
    date.getHours(),
  ).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
};

const ChartsPanel = ({
  quotaLoading,
  quotaError,
  activeChartTab,
  setActiveChartTab,
  spec_line,
  spec_rank_bar,
  spec_user_rank,
  perfMetricsSummary,
  perfMetricsLoading,
  modelChannelStats,
  modelChannelStatsLoading,
  modelChannelStatsDays,
  onModelChannelStatsDaysChange,
  userBalanceTrend,
  userBalanceTrendLoading,
  userBalanceTrendDays,
  onUserBalanceTrendDaysChange,
  onUserBalanceTrendUsersChanged,
  isAdminUser,
  CARD_PROPS,
  CHART_CONFIG,
  FLEX_CENTER_GAP2,
  hasApiInfoPanel,
  t,
}) => {
  const [selectedModel, setSelectedModel] = useState('');

  const modelSpendRows = useMemo(
    () => (modelChannelStats?.models || []).filter((item) => item.quota > 0),
    [modelChannelStats],
  );
  const channelTagRows = useMemo(
    () =>
      (modelChannelStats?.channel_tags || [])
        .filter((item) => item.model_name === selectedModel && item.quota > 0)
        .sort((a, b) => b.quota - a.quota),
    [modelChannelStats, selectedModel],
  );

  useEffect(() => {
    if (activeChartTab !== '6' || modelSpendRows.length === 0) {
      return;
    }
    if (!modelSpendRows.some((item) => item.model_name === selectedModel)) {
      setSelectedModel(modelSpendRows[0].model_name);
    }
  }, [activeChartTab, modelSpendRows, selectedModel]);

  const modelShareSpec = useMemo(() => {
    const values = modelSpendRows.map((item) => ({
      type: item.model_name,
      value: item.quota,
      request_count: item.request_count,
      share: item.share,
    }));
    const colors = {};
    modelSpendRows.forEach((item) => {
      colors[item.model_name] = modelToColor(item.model_name);
    });
    return {
      type: 'pie',
      data: [{ id: 'modelSpendShare', values }],
      outerRadius: 0.78,
      innerRadius: 0.52,
      padAngle: 0.8,
      valueField: 'value',
      categoryField: 'type',
      legends: { visible: true, orient: 'left' },
      label: {
        visible: true,
        formatMethod: (_, datum) => formatPercent(datum?.share),
      },
      title: {
        visible: true,
        text: t('各模型消耗金额占比'),
        subtext: `${t('总计')}：${renderQuota(modelChannelStats?.total_quota || 0, 2)} · ${formatDateTime(modelChannelStats?.start_timestamp)} ~ ${formatDateTime(modelChannelStats?.end_timestamp)}`,
      },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum.type,
              value: (datum) => renderQuota(datum.value || 0, 4),
            },
            { key: t('占比'), value: (datum) => formatPercent(datum.share) },
            {
              key: t('请求数'),
              value: (datum) => renderNumber(datum.request_count || 0),
            },
          ],
        },
      },
      color: { specified: colors },
    };
  }, [modelChannelStats?.total_quota, modelSpendRows, t]);

  const channelTagShareSpec = useMemo(() => {
    const tagColors = [
      '#1664FF',
      '#3CC780',
      '#FF8A00',
      '#7442D4',
      '#1AC6FF',
      '#FFC400',
      '#009488',
      '#FF7DDA',
    ];
    const values = channelTagRows.map((item) => ({
      type: item.tag,
      value: item.quota,
      request_count: item.request_count,
      share: item.share,
    }));
    return {
      type: 'pie',
      data: [{ id: 'channelTagShare', values }],
      outerRadius: 0.78,
      innerRadius: 0.52,
      padAngle: 0.8,
      valueField: 'value',
      categoryField: 'type',
      legends: { visible: true, orient: 'left' },
      label: {
        visible: true,
        formatMethod: (_, datum) => formatPercent(datum?.share),
      },
      title: {
        visible: true,
        text: t('渠道标签占比'),
        subtext: selectedModel
          ? `${selectedModel} · ${formatDateTime(modelChannelStats?.start_timestamp)} ~ ${formatDateTime(modelChannelStats?.end_timestamp)}`
          : t('请选择模型'),
      },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum.type,
              value: (datum) => renderQuota(datum.value || 0, 4),
            },
            { key: t('占比'), value: (datum) => formatPercent(datum.share) },
            {
              key: t('请求数'),
              value: (datum) => renderNumber(datum.request_count || 0),
            },
          ],
        },
      },
      color: { type: 'ordinal', range: tagColors },
    };
  }, [
    channelTagRows,
    modelChannelStats?.end_timestamp,
    modelChannelStats?.start_timestamp,
    selectedModel,
    t,
  ]);

  const perfMetricColumns = [
    {
      title: t('模型'),
      dataIndex: 'model_name',
      render: (text, record) => (
        <span className='font-medium'>
          {record.rank}. {text}
        </span>
      ),
    },
    {
      title: t('请求数'),
      dataIndex: 'request_count',
      render: (value) => Number(value || 0).toLocaleString(),
    },
    {
      title: t('平均延迟'),
      dataIndex: 'avg_latency_ms',
      render: (value) => `${Number(value || 0).toLocaleString()} ms`,
    },
    {
      title: 'TTFT',
      dataIndex: 'avg_ttft_ms',
      render: (value) => `${Number(value || 0).toLocaleString()} ms`,
    },
    {
      title: t('成功率'),
      dataIndex: 'success_rate',
      render: (value) => `${Number(value || 0).toFixed(2)}%`,
    },
    {
      title: t('平均TPS'),
      dataIndex: 'avg_tps',
      render: (value) => Number(value || 0).toFixed(2),
    },
  ];
  const perfMetricData = (perfMetricsSummary || [])
    .slice(0, 10)
    .map((item, index) => ({
      ...item,
      rank: index + 1,
    }));
  const isModelChannelTab = activeChartTab === '6' && isAdminUser;
  const isUserBalanceTrendTab = activeChartTab === '8' && isAdminUser;

  return (
    <Card
      {...CARD_PROPS}
      className={`!rounded-2xl ${hasApiInfoPanel ? 'lg:col-span-3' : ''}`}
      title={
        <div className='flex flex-col lg:flex-row lg:items-center lg:justify-between w-full gap-3'>
          <div className={FLEX_CENTER_GAP2}>
            <PieChart size={16} />
            {t('模型数据分析')}
          </div>
          <Tabs
            type='slash'
            activeKey={activeChartTab}
            onChange={setActiveChartTab}
          >
            <TabPane tab={<span>{t('消耗分布')}</span>} itemKey='1' />
            <TabPane tab={<span>{t('调用次数排行')}</span>} itemKey='4' />
            {isAdminUser && (
              <TabPane tab={<span>{t('用户消耗排行')}</span>} itemKey='5' />
            )}
            {isAdminUser && (
              <TabPane tab={<span>{t('模型金额占比')}</span>} itemKey='6' />
            )}
            {isAdminUser && (
              <TabPane tab={<span>{t('用户余额走势')}</span>} itemKey='8' />
            )}
            <TabPane tab={<span>{t('性能表现')}</span>} itemKey='7' />
          </Tabs>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <div
        className={
          isModelChannelTab || isUserBalanceTrendTab ? 'p-2' : 'h-96 p-2'
        }
      >
        <Suspense fallback={<ChartFallback t={t} />}>
          {['1', '4'].includes(activeChartTab) &&
            (quotaLoading ? (
              <ChartFallback t={t} />
            ) : quotaError ? (
              <div className='dashboard-chart-unavailable'>
                {t('加载失败，请重试')}
              </div>
            ) : (
              <VChart
                spec={activeChartTab === '1' ? spec_line : spec_rank_bar}
                option={CHART_CONFIG}
              />
            ))}
          {activeChartTab === '5' && isAdminUser && (
            <VChart spec={spec_user_rank} option={CHART_CONFIG} />
          )}
          {isModelChannelTab && (
            <Spin spinning={modelChannelStatsLoading}>
              <div className='space-y-3'>
                <div className='flex flex-col gap-2 md:flex-row md:items-center md:justify-between'>
                  <Space wrap>
                    {MODEL_CHANNEL_RANGE_OPTIONS.map((item) => (
                      <Button
                        key={item.value}
                        size='small'
                        theme={
                          modelChannelStatsDays === item.value
                            ? 'solid'
                            : 'light'
                        }
                        type='primary'
                        onClick={() =>
                          onModelChannelStatsDaysChange?.(item.value)
                        }
                      >
                        {t(item.label)}
                      </Button>
                    ))}
                  </Space>
                  <div className='flex flex-wrap gap-2'>
                    <Tag color='blue' shape='circle'>
                      {t('模型数量')} {modelSpendRows.length}
                    </Tag>
                    <Tag color='green' shape='circle'>
                      {t('总请求')}{' '}
                      {renderNumber(modelChannelStats?.total_request || 0)}
                    </Tag>
                  </div>
                </div>
                {modelSpendRows.length > 0 ? (
                  <>
                    <div className='grid grid-cols-1 xl:grid-cols-2 gap-3'>
                      <div className='h-64 rounded-xl border border-gray-100 bg-white p-2'>
                        <VChart spec={modelShareSpec} option={CHART_CONFIG} />
                      </div>
                      <div className='h-64 rounded-xl border border-gray-100 bg-white p-2'>
                        <VChart
                          spec={channelTagShareSpec}
                          option={CHART_CONFIG}
                        />
                      </div>
                    </div>
                    <div className='grid grid-cols-1 xl:grid-cols-2 gap-3'>
                      <div className='rounded-xl border border-gray-100 bg-gray-50 p-3'>
                        <div className='mb-2 text-sm font-semibold text-gray-700'>
                          {t('模型消耗排行')}
                        </div>
                        <div className='space-y-2'>
                          {modelSpendRows.map((item, index) => (
                            <button
                              key={item.model_name}
                              type='button'
                              onClick={() => setSelectedModel(item.model_name)}
                              className={`w-full rounded-lg p-2 text-left transition-colors ${
                                selectedModel === item.model_name
                                  ? 'bg-blue-50 ring-1 ring-blue-200'
                                  : 'bg-white hover:bg-gray-100'
                              }`}
                            >
                              <div className='flex items-center justify-between gap-3'>
                                <span className='truncate text-sm font-medium'>
                                  {index + 1}. {item.model_name}
                                </span>
                                <span className='text-sm font-semibold text-gray-900'>
                                  {renderQuota(item.quota, 2)}
                                </span>
                              </div>
                              <div className='mt-2 h-1.5 rounded-full bg-gray-200'>
                                <div
                                  className='h-1.5 rounded-full bg-blue-500'
                                  style={{ width: formatPercent(item.share) }}
                                />
                              </div>
                              <div className='mt-1 text-xs text-gray-500'>
                                {formatPercent(item.share)} · {t('请求数')}{' '}
                                {renderNumber(item.request_count || 0)}
                              </div>
                            </button>
                          ))}
                        </div>
                      </div>
                      <div className='rounded-xl border border-gray-100 bg-gray-50 p-3'>
                        <div className='mb-2 text-sm font-semibold text-gray-700'>
                          {selectedModel
                            ? `${selectedModel} · ${t('渠道标签占比')}`
                            : t('渠道标签占比')}
                        </div>
                        <div className='space-y-2'>
                          {channelTagRows.map((item) => (
                            <div
                              key={`${item.model_name}-${item.tag}`}
                              className='rounded-lg bg-white p-2'
                            >
                              <div className='flex items-center justify-between gap-3'>
                                <span className='truncate text-sm font-medium'>
                                  {item.tag}
                                </span>
                                <span className='text-sm font-semibold text-gray-900'>
                                  {renderQuota(item.quota, 2)}
                                </span>
                              </div>
                              <div className='mt-2 h-1.5 rounded-full bg-gray-200'>
                                <div
                                  className='h-1.5 rounded-full bg-emerald-500'
                                  style={{ width: formatPercent(item.share) }}
                                />
                              </div>
                              <div className='mt-1 text-xs text-gray-500'>
                                {formatPercent(item.share)} · {t('请求数')}{' '}
                                {renderNumber(item.request_count || 0)}
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    </div>
                  </>
                ) : (
                  <div className='min-h-96 flex items-center justify-center'>
                    <Empty title={t('暂无模型消耗数据')} />
                  </div>
                )}
              </div>
            </Spin>
          )}
          {isUserBalanceTrendTab && (
            <UserBalanceTrendPanel
              report={userBalanceTrend}
              loading={userBalanceTrendLoading}
              days={userBalanceTrendDays}
              onDaysChange={onUserBalanceTrendDaysChange}
              onUsersChanged={onUserBalanceTrendUsersChanged}
              embedded
              CHART_CONFIG={CHART_CONFIG}
              t={t}
            />
          )}
        </Suspense>
        {activeChartTab === '7' && (
          <div className='h-full overflow-auto p-2'>
            <Table
              columns={perfMetricColumns}
              dataSource={perfMetricData}
              pagination={false}
              loading={perfMetricsLoading}
              rowKey='model_name'
              size='small'
            />
          </div>
        )}
      </div>
    </Card>
  );
};

export default ChartsPanel;
