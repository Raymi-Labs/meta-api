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

import React from 'react';
import { Button, Skeleton } from '@douyinfe/semi-ui';
import { Link } from 'react-router-dom';
import { Gift, RefreshCw } from 'lucide-react';
import { renderQuota } from '../../helpers';

export default function WalletOverview({
  user,
  loading,
  error,
  onRefresh,
  onCreate,
  onHistory,
  onInvoices,
  t,
}) {
  const quota = (value) =>
    typeof value === 'number' && Number.isFinite(value)
      ? renderQuota(value)
      : '—';
  const knownBalance =
    typeof user?.quota === 'number' && Number.isFinite(user.quota);
  const knownTransfer =
    typeof user?.transferable_quota === 'number' &&
    Number.isFinite(user.transferable_quota);
  return (
    <>
      <header className='wallet-heading'>
        <h1>{t('钱包管理')}</h1>
        <div className='wallet-actions'>
          <Button theme='borderless' onClick={onHistory}>
            {t('账单')}
          </Button>
          <Button theme='borderless' onClick={onInvoices}>
            {t('申请发票')}
          </Button>
        </div>
      </header>
      <section className='wallet-balance-card'>
        <div className='wallet-balance-values'>
          <div className='wallet-main-balance'>
            <span>{t('当前余额')}</span>
            {loading ? (
              <Skeleton.Title style={{ width: 120, height: 30 }} />
            ) : (
              <strong>{error ? '—' : quota(user?.quota)}</strong>
            )}
          </div>
          <div>
            <span>{t('可创建兑换码额度')}</span>
            {loading ? (
              <Skeleton.Title style={{ width: 100, height: 24 }} />
            ) : (
              <strong>{error ? '—' : quota(user?.transferable_quota)}</strong>
            )}
          </div>
          <Button
            theme='borderless'
            icon={<RefreshCw size={16} />}
            loading={loading}
            aria-label={t('刷新')}
            onClick={onRefresh}
          >
            {t('刷新')}
          </Button>
        </div>
        {error && (
          <p role='alert' className='wallet-error-text'>
            {t('加载失败，请重试')}
          </p>
        )}
        <div className='wallet-balance-footer'>
          <div className='wallet-history-values'>
            <span>
              {t('历史消耗')}:{' '}
              {error || loading ? '—' : quota(user?.used_quota)}
            </span>
            <span>
              {t('请求次数')}:{' '}
              {error || loading || typeof user?.request_count !== 'number'
                ? '—'
                : user.request_count}
            </span>
            <Link to='/console/log'>{t('使用日志')}</Link>
          </div>
          <Button
            theme='outline'
            icon={<Gift size={16} />}
            onClick={onCreate}
            disabled={loading || error || !knownBalance || !knownTransfer}
          >
            {t('创建兑换码')}
          </Button>
        </div>
        <p className='wallet-hint'>
          {t('将可转赠额度生成兑换码，赠送给其他用户')}
        </p>
      </section>
    </>
  );
}
