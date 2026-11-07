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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Empty,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { RefreshCw, Ticket } from 'lucide-react';
import { API } from '../../helpers/apiCore';

const { Text, Title } = Typography;

const formatInteger = (value) =>
  new Intl.NumberFormat().format(Number.isFinite(Number(value)) ? value : 0);

const formatDateTime = (value) => {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
};

const statusColor = (status) => {
  if (status === 'settled' || status === 'completed') return 'green';
  if (status === 'failed' || status === 'dead' || status === 'reversed') {
    return 'red';
  }
  return 'amber';
};

const StatCard = ({ icon, label, value, hint }) => (
  <Card className='!rounded-2xl border-0 shadow-sm h-full'>
    <div className='flex items-start justify-between gap-4'>
      <div>
        <Text type='tertiary' size='small'>
          {label}
        </Text>
        <div className='mt-2 text-2xl font-semibold text-[var(--semi-color-text-0)]'>
          {value}
        </div>
        {hint ? (
          <div className='mt-1 text-xs text-[var(--semi-color-text-2)]'>
            {hint}
          </div>
        ) : null}
      </div>
      <div className='rounded-xl bg-[var(--semi-color-primary-light-default)] p-2 text-[var(--semi-color-primary)]'>
        {icon}
      </div>
    </div>
  </Card>
);

const Pulse = () => {
  const { t } = useTranslation();
  const [summary, setSummary] = useState(null);
  const [rewards, setRewards] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadKey, setReloadKey] = useState(0);

  const retry = useCallback(() => setReloadKey((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    const load = async () => {
      setLoading(true);
      setError('');
      try {
        const config = {
          signal: controller.signal,
          disableDuplicate: true,
          skipErrorHandler: true,
          skipGlobalLoading: true,
        };
        const [summaryResponse, rewardsResponse] = await Promise.all([
          API.get('/api/pulse/summary', config),
          API.get('/api/pulse/rewards', {
            ...config,
            params: { limit: 50 },
          }),
        ]);
        if (!active) return;
        setSummary(summaryResponse.data || null);
        setRewards(
          Array.isArray(rewardsResponse.data?.rewards)
            ? rewardsResponse.data.rewards
            : [],
        );
      } catch (requestError) {
        if (!active || requestError?.code === 'ERR_CANCELED') return;
        const unavailable = requestError?.response?.status === 503;
        setError(
          unavailable
            ? t('Pulse 服务暂不可用，请稍后重试')
            : t('无法加载 Pulse 数据，请稍后重试'),
        );
      } finally {
        if (active) setLoading(false);
      }
    };

    load();
    return () => {
      active = false;
      controller.abort();
    };
  }, [reloadKey, t]);

  const rewardColumns = useMemo(
    () => [
      { title: t('奖励类型'), dataIndex: 'reward_type' },
      {
        title: t('奖励值'),
        dataIndex: 'amount',
        render: formatInteger,
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (value) => <Tag color={statusColor(value)}>{value || '-'}</Tag>,
      },
      {
        title: t('时间'),
        dataIndex: 'created_at',
        render: formatDateTime,
      },
    ],
    [t],
  );

  return (
    <div className='px-2 pb-8'>
      <div className='mx-auto w-full max-w-[1440px]'>
        <div className='mb-5 flex flex-wrap items-center justify-between gap-3'>
          <div>
            <Title heading={3} className='!mb-1'>
              {t('Meta Pulse')}
            </Title>
          </div>
        </div>

        {loading ? (
          <Card className='!rounded-2xl border-0 shadow-sm'>
            <div className='flex min-h-[360px] items-center justify-center'>
              <Spin size='large' tip={t('正在加载 Pulse 数据')} />
            </div>
          </Card>
        ) : error ? (
          <Card className='!rounded-2xl border-0 shadow-sm'>
            <Empty
              title={t('暂时无法显示 Pulse')}
              description={error}
              style={{ padding: '64px 0' }}
            >
              <Button
                type='primary'
                icon={<RefreshCw size={16} />}
                onClick={retry}
              >
                {t('重新加载')}
              </Button>
            </Empty>
          </Card>
        ) : (
          <>
            <div className='mb-5 grid grid-cols-1 gap-4 sm:grid-cols-3'>
              <StatCard
                icon={<Ticket size={20} />}
                label={t('可用奖励券')}
                value={formatInteger(summary?.available_tickets)}
              />
            </div>

            <Card
              className='!rounded-2xl border-0 shadow-sm'
              title={t('奖励历史')}
            >
              <Table
                rowKey='grant_id'
                columns={rewardColumns}
                dataSource={rewards}
                pagination={false}
                empty={<Empty description={t('暂无奖励记录')} />}
                scroll={{ x: 620 }}
              />
            </Card>
          </>
        )}
      </div>
    </div>
  );
};

export default Pulse;
