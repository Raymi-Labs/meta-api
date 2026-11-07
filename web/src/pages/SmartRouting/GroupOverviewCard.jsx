import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import {
  Banner,
  Button,
  Card,
  Popover,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { API } from '../../helpers/apiCore';
import {
  formatHealthLatency,
  formatHealthRate,
  healthTone,
} from '../../components/group-health/healthUtils';
import { formatHealthOutcomeDescription } from '../../components/group-health';

const { Text } = Typography;

const TONE_COLOR = {
  healthy: 'green',
  warning: 'orange',
  degraded: 'red',
  limited: 'grey',
  empty: 'grey',
};

export default function GroupOverviewCard({ selectedGroup, onSelectGroup }) {
  const { t } = useTranslation();
  const [hours, setHours] = useState(24);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);

  const reasonLabel = (reason) =>
    ({
      low_success_rate: t('成功率低'),
      recent_failure_burst: t('近期集中失败'),
      slow_ttft: t('首字过慢'),
    })[reason] || reason;

  const load = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    setError('');
    try {
      const response = await API.get(
        `/api/route_insight/groups?hours=${hours}`,
      );
      if (current !== generation.current) return;
      if (!response.data?.success) throw new Error(response.data?.message);
      setData(response.data.data);
    } catch (failure) {
      if (current !== generation.current) return;
      setData(null);
      setError(
        failure?.response?.data?.message || failure?.message || t('加载失败'),
      );
    } finally {
      if (current === generation.current) setLoading(false);
    }
  }, [hours, t]);

  useEffect(() => {
    load();
  }, [load]);

  const renderUnhealthy = (row) => {
    if (!data?.snapshot_fresh) {
      return (
        <Tooltip content={t('健康快照已过期，当前状态未知')}>
          <Text type='tertiary'>—</Text>
        </Tooltip>
      );
    }
    const channels = row.unhealthy_channels || [];
    if (channels.length === 0) return <Text type='tertiary'>0</Text>;
    return (
      <Popover
        showArrow
        position='left'
        content={
          <div style={{ maxWidth: 360, padding: 12 }}>
            {channels.map((channel) => (
              <div key={channel.channel_id} style={{ marginBottom: 6 }}>
                <Text
                  strong
                >{`#${channel.channel_id} ${channel.channel_name || ''}`}</Text>
                <div>
                  <Text type='tertiary' size='small'>
                    {channel.models
                      .map(
                        (name, index) =>
                          `${name}（${reasonLabel(channel.reasons[index])}）`,
                      )
                      .join('、')}
                  </Text>
                </div>
              </div>
            ))}
          </div>
        }
      >
        <Tag color='red' style={{ cursor: 'pointer' }}>
          {t('{{count}} 个渠道', { count: channels.length })}
        </Tag>
      </Popover>
    );
  };

  const columns = [
    {
      title: t('分组'),
      dataIndex: 'group',
      render: (group, row) => (
        <Space spacing={4} wrap>
          <Tag size='large' color={group === selectedGroup ? 'blue' : 'cyan'}>
            {group}
          </Tag>
          {!row.configured && (
            <Text type='tertiary' size='small'>
              {t('已不在分组配置中')}
            </Text>
          )}
        </Space>
      ),
    },
    {
      title: t('请求数'),
      dataIndex: 'request_count',
      sorter: (a, b) => a.request_count - b.request_count,
      render: (value) => (value || 0).toLocaleString(),
    },
    {
      title: t('成功率'),
      dataIndex: 'success_rate',
      sorter: (a, b) => (a.success_rate ?? -1) - (b.success_rate ?? -1),
      render: (_, row) => {
        if (!row.request_count) return <Text type='tertiary'>—</Text>;
        return (
          <Tooltip content={formatHealthOutcomeDescription(row, t)}>
            <Tag color={TONE_COLOR[healthTone(row)]} size='large'>
              {formatHealthRate(row.success_rate)}
            </Tag>
          </Tooltip>
        );
      },
    },
    {
      title: t('服务失败 / 未归因'),
      render: (_, row) =>
        `${row.service_failure_count || 0} / ${row.unknown_failure_count || 0}`,
    },
    {
      title: t('已排除用户错误'),
      dataIndex: 'excluded_user_error_count',
      render: (value) => (value || 0).toLocaleString(),
    },
    {
      title: t('平均首字'),
      dataIndex: 'avg_ttft_ms',
      sorter: (a, b) =>
        (a.avg_ttft_ms ?? Infinity) - (b.avg_ttft_ms ?? Infinity),
      render: (value) => formatHealthLatency(value),
    },
    {
      title: t('当前不健康渠道'),
      render: (_, row) => renderUnhealthy(row),
    },
    {
      title: '',
      render: (_, row) => (
        <Button
          size='small'
          theme='borderless'
          onClick={() => onSelectGroup(row.group)}
        >
          {t('查看渠道')}
        </Button>
      ),
    },
  ];

  return (
    <Card
      title={t('分组概览')}
      style={{ width: '100%' }}
      headerExtraContent={
        <Space>
          <Select
            value={hours}
            onChange={setHours}
            size='small'
            optionList={[
              { value: 1, label: t('最近 1 小时') },
              { value: 6, label: t('最近 6 小时') },
              { value: 24, label: t('最近 24 小时') },
              { value: 72, label: t('最近 3 天') },
              { value: 168, label: t('最近 7 天') },
            ]}
          />
          <Button size='small' onClick={load} loading={loading}>
            {t('刷新')}
          </Button>
        </Space>
      }
    >
      {error && <Banner type='danger' description={error} />}
      {data?.collection_incomplete && (
        <Banner
          type='warning'
          description={t('部分统计采集延迟或缺失，当前数据仅供参考。')}
        />
      )}
      <Table
        size='small'
        rowKey='group'
        loading={loading}
        columns={columns}
        dataSource={data?.groups || []}
        pagination={{ pageSize: 10 }}
        scroll={{ x: 'max-content' }}
        empty={error ? t('加载失败') : t('暂无数据')}
      />
      <Text type='tertiary' size='small'>
        {t(
          '成功率与模型广场的分组健康同一口径：同一请求在同组内重试只算一次，明确的用户错误不计入；计入统计不足 30 次显示为灰色。按半小时汇总，包含当前半小时。',
        )}{' '}
        {t('各分组消耗额度见使用日志中的「分组汇总」。')}{' '}
        <Link to='/console/log'>{t('打开使用日志')}</Link>
      </Text>
    </Card>
  );
}
