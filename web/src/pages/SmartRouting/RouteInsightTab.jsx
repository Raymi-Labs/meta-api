import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Card,
  Input,
  InputNumber,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/apiCore';
import { isRoot } from '../../helpers/storage';
import { showError, showSuccess } from '../../helpers/toast';
import { formatLatency, formatPercent, statusColor } from './format';

const { Text } = Typography;

const ROUTING_KEYS = {
  health: 'route_routing_setting.health_routing_enabled',
  probe: 'route_routing_setting.probe_percent',
};

export default function RouteInsightTab() {
  const { t } = useTranslation();
  const [minutes, setMinutes] = useState(60);
  const [hours, setHours] = useState(24);
  const [modelInput, setModelInput] = useState('');
  const [modelFilter, setModelFilter] = useState('');
  const [channelData, setChannelData] = useState(null);
  const [shadowData, setShadowData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [routing, setRouting] = useState(null);
  const [saving, setSaving] = useState(false);
  const generation = useRef(0);
  const root = isRoot();
  const statusLabel = (status) =>
    ({ healthy: t('健康'), unhealthy: t('不健康'), unknown: t('未知') })[
      status
    ] || status;

  const load = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    setError('');
    try {
      const model = modelFilter
        ? `&model=${encodeURIComponent(modelFilter)}`
        : '';
      const [channels, shadow] = await Promise.all([
        API.get(`/api/route_insight/channels?minutes=${minutes}${model}`),
        API.get(`/api/route_insight/shadow?hours=${hours}${model}`),
      ]);
      if (current !== generation.current) return;
      if (!channels.data?.success) throw new Error(channels.data?.message);
      if (!shadow.data?.success) throw new Error(shadow.data?.message);
      setChannelData(channels.data.data);
      setShadowData(shadow.data.data);
      setRouting(channels.data.data.routing);
    } catch (failure) {
      if (current !== generation.current) return;
      setError(
        failure?.response?.data?.message || failure?.message || t('加载失败'),
      );
    } finally {
      if (current === generation.current) setLoading(false);
    }
  }, [minutes, hours, modelFilter, t]);

  useEffect(() => {
    load();
  }, [load]);

  const saveRouting = async (key, value, next) => {
    setSaving(true);
    try {
      const response = await API.put('/api/option/', {
        key,
        value: String(value),
      });
      if (!response.data?.success) throw new Error(response.data?.message);
      setRouting(next);
      showSuccess(t('保存成功'));
    } catch (failure) {
      showError(failure?.response?.data?.message || failure?.message);
    } finally {
      setSaving(false);
    }
  };

  const snapshotAge = channelData?.snapshot_at
    ? Math.max(0, Math.round(Date.now() / 1000 - channelData.snapshot_at))
    : null;

  const shadowColumns = [
    { title: t('模型'), dataIndex: 'model' },
    {
      title: t('请求 / 交付率'),
      render: (_, row) =>
        `${row.total.requests} / ${formatPercent(row.delivery_rate)}`,
    },
    {
      title: t('平均尝试次数'),
      render: (_, row) =>
        row.avg_attempts == null ? '-' : row.avg_attempts.toFixed(2),
    },
    {
      title: t('一致 / 分歧'),
      render: (_, row) => `${row.total.agree} / ${row.total.disagree}`,
    },
    {
      title: t('一致时失败率'),
      render: (_, row) => formatPercent(row.agree_failure_rate),
    },
    {
      title: t('分歧时失败率'),
      render: (_, row) => formatPercent(row.disagree_failure_rate),
    },
    {
      title: t('避开不健康'),
      render: (_, row) => row.total.avoid_unhealthy,
    },
    { title: t('更便宜'), render: (_, row) => row.total.cheaper },
    { title: t('跨分组'), render: (_, row) => row.total.cross_group },
    {
      title: t('无数据 / 缺成本'),
      render: (_, row) => `${row.total.no_data} / ${row.total.cost_unknown}`,
    },
  ];

  const channelColumns = [
    { title: t('模型'), dataIndex: 'model' },
    {
      title: t('渠道'),
      render: (_, row) => `#${row.channel_id} ${row.channel_name || ''}`,
    },
    { title: t('分组'), dataIndex: 'group' },
    {
      title: t('当前状态'),
      render: (_, row) => (
        <Tag color={statusColor(row.current_status)}>
          {statusLabel(row.current_status)}
        </Tag>
      ),
    },
    { title: t('尝试'), render: (_, row) => row.window.attempts },
    {
      title: t('成功率（下界）'),
      render: (_, row) =>
        `${formatPercent(row.success_rate)} (${formatPercent(row.success_lower_bound)})`,
    },
    {
      title: t('上游失败 / 429'),
      render: (_, row) =>
        `${row.window.service_failure} / ${row.window.rate_limited}`,
    },
    {
      title: t('未知 / 用户 / 取消'),
      render: (_, row) =>
        `${row.window.unknown_failure} / ${row.window.user_error} / ${row.window.client_canceled}`,
    },
    {
      title: t('首字 P50 / P95'),
      render: (_, row) =>
        `${formatLatency(row.ttft_p50_ms)} / ${formatLatency(row.ttft_p95_ms)}`,
    },
    {
      title: t('完成 P95'),
      render: (_, row) => formatLatency(row.duration_p95_ms),
    },
    {
      title: t('采购成本倍率'),
      render: (_, row) =>
        row.purchase_ratio == null ? (
          <Text type='tertiary'>{t('未知')}</Text>
        ) : (
          row.purchase_ratio
        ),
    },
    {
      title: t('故障域'),
      render: (_, row) => row.fault_domain || '-',
    },
  ];

  const sampleColumns = [
    {
      title: t('时间'),
      render: (_, row) => new Date(row.at * 1000).toLocaleString(),
    },
    { title: t('模型'), dataIndex: 'model' },
    {
      title: t('实际'),
      render: (_, row) =>
        `#${row.actual_channel_id} (${row.actual_group}) ${statusLabel(row.actual_status)} → ${row.actual_outcome}`,
    },
    {
      title: t('影子推荐'),
      render: (_, row) =>
        `#${row.shadow.channel_id} (${row.shadow.group}) ${row.shadow.reason}`,
    },
  ];

  return (
    <Space vertical align='start' style={{ width: '100%' }} spacing='medium'>
      <Banner
        type='info'
        description={t(
          '影子决策只记录智能路由会选哪条线路，不改变调度；样本不足或数据过期视为未知。',
        )}
      />
      <Card
        title={t('路由开关')}
        style={{ width: '100%' }}
        headerExtraContent={
          !root && <Text type='tertiary'>{t('仅超级管理员可修改')}</Text>
        }
      >
        {routing ? (
          <Space vertical align='start'>
            <Space>
              <Switch
                checked={routing.health_routing_enabled}
                disabled={!root || saving}
                onChange={(checked) =>
                  saveRouting(ROUTING_KEYS.health, checked, {
                    ...routing,
                    health_routing_enabled: checked,
                  })
                }
              />
              <Text strong>{t('智能路由总开关')}</Text>
              <Text type='tertiary'>
                {t('在当前分组内跳过不健康线路，不换分组、不改价格。')}
              </Text>
            </Space>
            <Space>
              <Text strong>{t('探测比例')}</Text>
              <InputNumber
                min={0}
                max={20}
                value={routing.probe_percent}
                disabled={!root || saving}
                suffix='%'
                style={{ width: 120 }}
                onBlur={(event) => {
                  const value = Math.min(
                    20,
                    Math.max(0, Number(event.target.value) || 0),
                  );
                  if (value !== routing.probe_percent) {
                    saveRouting(ROUTING_KEYS.probe, value, {
                      ...routing,
                      probe_percent: value,
                    });
                  }
                }}
              />
              <Text type='tertiary'>
                {t('允许少量请求尝试不健康或未知线路以便恢复，0 表示关闭。')}
              </Text>
            </Space>
          </Space>
        ) : (
          <Spin />
        )}
      </Card>
      <Space wrap>
        <Input
          placeholder={t('按模型筛选')}
          value={modelInput}
          onChange={setModelInput}
          onEnterPress={() => setModelFilter(modelInput.trim())}
          showClear
          onClear={() => setModelFilter('')}
          style={{ width: 260 }}
        />
        <Select
          value={minutes}
          onChange={setMinutes}
          prefix={t('渠道')}
          optionList={[
            { value: 15, label: t('最近 15 分钟') },
            { value: 60, label: t('最近 1 小时') },
            { value: 360, label: t('最近 6 小时') },
            { value: 1440, label: t('最近 24 小时') },
          ]}
        />
        <Select
          value={hours}
          onChange={setHours}
          prefix={t('影子决策')}
          optionList={[
            { value: 24, label: t('最近 24 小时') },
            { value: 168, label: t('最近 7 天') },
            { value: 720, label: t('最近 30 天') },
          ]}
        />
        <Button onClick={() => setModelFilter(modelInput.trim())}>
          {t('查询')}
        </Button>
        <Button onClick={load} loading={loading}>
          {t('刷新')}
        </Button>
        {snapshotAge != null && (
          <Text type={channelData?.snapshot_failing ? 'danger' : 'tertiary'}>
            {channelData?.snapshot_failing
              ? t('健康快照刷新失败，数据按未知处理')
              : t('健康快照 {{seconds}} 秒前更新', { seconds: snapshotAge })}
          </Text>
        )}
      </Space>
      {channelData && !channelData.enabled && (
        <Banner
          type='warning'
          description={t('路由观测已关闭（ROUTE_INSIGHT_ENABLED=false）')}
        />
      )}
      {channelData && channelData.memory_cache_enabled === false && (
        <Banner
          type='warning'
          description={t(
            '未启用内存缓存（未配置 Redis 且 MEMORY_CACHE_ENABLED 未开启），影子决策与智能路由不会生效。',
          )}
        />
      )}
      {error && <Banner type='danger' description={error} />}
      <Card title={t('影子决策对照')} style={{ width: '100%' }}>
        <Table
          size='small'
          rowKey='model'
          loading={loading}
          columns={shadowColumns}
          dataSource={shadowData?.models || []}
          pagination={{ pageSize: 10 }}
          scroll={{ x: 'max-content' }}
          empty={t('暂无数据')}
        />
      </Card>
      <Card title={t('渠道逐次尝试')} style={{ width: '100%' }}>
        <Table
          size='small'
          rowKey={(row) => `${row.model}-${row.channel_id}-${row.group}`}
          loading={loading}
          columns={channelColumns}
          dataSource={channelData?.channels || []}
          pagination={{ pageSize: 20 }}
          scroll={{ x: 'max-content' }}
          empty={t('暂无数据')}
        />
        <Text type='tertiary' size='small'>
          {t('成功率只计有上游证据的结果；P50/P95 为保守估计。')}
        </Text>
      </Card>
      <Card title={t('最近分歧样本（本实例）')} style={{ width: '100%' }}>
        <Table
          size='small'
          rowKey={(row, index) => `${row.at}-${index}`}
          loading={loading}
          columns={sampleColumns}
          dataSource={shadowData?.recent_disagreements || []}
          pagination={{ pageSize: 10 }}
          scroll={{ x: 'max-content' }}
          empty={t('暂无数据')}
        />
      </Card>
    </Space>
  );
}
