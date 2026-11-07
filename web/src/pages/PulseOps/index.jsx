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

import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Empty,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { RefreshCw } from 'lucide-react';
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

const PulseOps = () => {
  const { t } = useTranslation();
  const [overview, setOverview] = useState(null);
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
        const response = await API.get('/api/pulse/ops/overview', {
          signal: controller.signal,
          disableDuplicate: true,
          skipErrorHandler: true,
          skipGlobalLoading: true,
        });
        if (active) setOverview(response.data || null);
      } catch (requestError) {
        if (!active || requestError?.code === 'ERR_CANCELED') return;
        const status = requestError?.response?.status;
        setError(
          status === 503
            ? t('Pulse 服务暂不可用，请稍后重试')
            : status === 403
              ? t('当前账号无权查看运营概览')
              : t('无法加载运营概览，请稍后重试'),
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

  const snapshot = overview?.snapshot;

  return (
    <div className='px-2 pb-8'>
      <div className='mx-auto w-full max-w-[1440px]'>
        <div className='mb-5 flex flex-wrap items-center justify-between gap-3'>
          <div>
            <Title heading={3} className='!mb-1'>
              {t('Pulse 运营概览')}
            </Title>
          </div>
          <Button icon={<RefreshCw size={16} />} onClick={retry}>
            {t('刷新')}
          </Button>
        </div>

        {loading ? (
          <Card className='!rounded-2xl border-0 shadow-sm'>
            <div className='flex min-h-[360px] items-center justify-center'>
              <Spin size='large' tip={t('正在加载运营概览')} />
            </div>
          </Card>
        ) : error ? (
          <Card className='!rounded-2xl border-0 shadow-sm'>
            <Empty
              title={t('暂时无法显示运营概览')}
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
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              {[
                {
                  key: 'settlement_retry_count',
                  label: t('结算重试'),
                },
                {
                  key: 'settlement_dead_count',
                  label: t('结算死信'),
                },
              ].map((item) => (
                <Card
                  key={item.key}
                  className='!rounded-2xl border-0 shadow-sm'
                >
                  <Text type='tertiary' size='small'>
                    {item.label}
                  </Text>
                  <div className='mt-2 text-2xl font-semibold'>
                    {formatInteger(snapshot?.[item.key])}
                  </div>
                </Card>
              ))}
            </div>

            <Text type='tertiary' size='small' className='mt-4 block'>
              {t('观测时间')} {formatDateTime(overview?.observed_at)}
            </Text>
          </>
        )}
      </div>
    </div>
  );
};

export default PulseOps;
