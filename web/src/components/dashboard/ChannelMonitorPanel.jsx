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

import React, { useState, useEffect, useRef } from 'react';
import {
  Card,
  Table,
  Spin,
  Button,
  Select,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconRefresh, IconFilter } from '@douyinfe/semi-icons';
import { API } from '../../helpers/apiCore';
import { showError } from '../../helpers/toast';
import { renderQuota } from '../../helpers/dashboardFormat';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

const { Text } = Typography;

const ChannelMonitorPanel = ({ CARD_PROPS, ILLUSTRATION_SIZE }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const requestId = useRef(0);
  const [error, setError] = useState(false);
  const [loadedRange, setLoadedRange] = useState(null);
  const [timeRange, setTimeRange] = useState(86400); // 默认24小时
  const [groupStats, setGroupStats] = useState([]);

  // 表格列定义 - 按分组
  const groupColumns = [
    {
      title: t('渠道分组'),
      dataIndex: 'channel_group',
      key: 'channel_group',
      render: (text) => (
        <Tag size='large' color='cyan'>
          {text || t('默认分组')}
        </Tag>
      ),
    },
    {
      title: t('总请求数'),
      dataIndex: 'total_requests',
      key: 'total_requests',
      sorter: (a, b) => a.total_requests - b.total_requests,
      render: (val) => val?.toLocaleString() || 0,
    },
    {
      title: t('成功率'),
      dataIndex: 'success_rate',
      key: 'success_rate',
      sorter: (a, b) => a.success_rate - b.success_rate,
      render: (rate) => {
        const color = rate >= 95 ? 'green' : rate >= 80 ? 'orange' : 'red';
        return (
          <Tag color={color} size='large'>
            {rate?.toFixed(2)}%
          </Tag>
        );
      },
    },
    {
      title: t('成功/失败'),
      key: 'success_failed',
      render: (_, record) => (
        <Space>
          <Text type='success'>
            {record.success_requests?.toLocaleString() || 0}
          </Text>
          <Text type='tertiary'>/</Text>
          <Text type='danger'>
            {record.failed_requests?.toLocaleString() || 0}
          </Text>
        </Space>
      ),
    },
    {
      title: t('总消耗额度'),
      dataIndex: 'total_quota',
      key: 'total_quota',
      sorter: (a, b) => a.total_quota - b.total_quota,
      render: (quota) => renderQuota(quota || 0, 4),
    },
    {
      title: t('平均响应时间'),
      dataIndex: 'avg_response_time',
      key: 'avg_response_time',
      sorter: (a, b) => a.avg_response_time - b.avg_response_time,
      render: (time) => `${(time || 0).toFixed(0)}ms`,
    },
  ];

  // 加载监控数据
  const loadMonitorData = async () => {
    const id = ++requestId.current;
    setLoading(true);
    setError(false);
    try {
      const endTime = Math.floor(Date.now() / 1000);
      const startTime = endTime - timeRange;

      const res = await API.get('/api/monitor/channels', {
        params: {
          start_time: startTime,
          end_time: endTime,
          group_by: 'group',
          exact_range: true,
        },
      });

      if (id !== requestId.current) return;
      if (res.data.success) {
        setGroupStats(res.data.data || []);
        setLoadedRange([startTime, endTime]);
      } else {
        setError(true);
        showError(res.data.message || t('加载监控数据失败'));
      }
    } catch (error) {
      if (id !== requestId.current) return;
      setError(true);
      showError(t('加载监控数据失败') + ': ' + (error.message || ''));
    } finally {
      if (id === requestId.current) setLoading(false);
    }
  };

  // 初始加载和依赖变化时重新加载
  useEffect(() => {
    loadMonitorData();
  }, [timeRange]);

  // 时间范围选项
  const timeRangeOptions = [
    { value: 3600, label: t('最近1小时') },
    { value: 21600, label: t('最近6小时') },
    { value: 86400, label: t('最近24小时') },
    { value: 259200, label: t('最近3天') },
    { value: 604800, label: t('最近7天') },
    { value: 2592000, label: t('最近30天') },
  ];

  return (
    <Card
      {...CARD_PROPS}
      title={
        <Space>
          <IconFilter />
          <span>{t('分组监控统计')}</span>
        </Space>
      }
      headerExtraContent={
        <Space>
          <Select
            value={timeRange}
            onChange={setTimeRange}
            optionList={timeRangeOptions}
            style={{ width: 140 }}
            size='small'
          />
          <Button
            icon={<IconRefresh />}
            onClick={loadMonitorData}
            loading={loading}
            size='small'
          >
            {t('刷新')}
          </Button>
        </Space>
      }
    >
      <Spin spinning={loading}>
        <div className='mb-3 flex flex-wrap items-center gap-3'>
          <Text>
            {t('总消耗额度')}：
            {error || !loadedRange
              ? '—'
              : renderQuota(
                  groupStats.reduce((sum, row) => sum + row.total_quota, 0),
                  4,
                )}
          </Text>
          <Link
            to={
              '/console/channel?view=usage&start_time=' +
              (loadedRange?.[0] || Math.floor(Date.now() / 1000) - timeRange) +
              '&end_time=' +
              (loadedRange?.[1] || Math.floor(Date.now() / 1000))
            }
          >
            {t('查看渠道用量')}
          </Link>
        </div>
        <Table
          columns={groupColumns}
          dataSource={error ? [] : groupStats}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
          }}
          rowKey={(record) => `group_${record.channel_group}`}
          size='small'
          empty={error ? t('加载监控数据失败') : t('暂无数据')}
        />
      </Spin>
    </Card>
  );
};

export default ChannelMonitorPanel;
