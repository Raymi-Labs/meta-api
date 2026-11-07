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
import React, { useContext, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Wallet, ChevronDown, ArrowUpRight, RefreshCw } from 'lucide-react';
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';
import { renderQuota } from '../../helpers/render';
import {
  validWalletSummary,
  walletEstimate,
} from '../../helpers/walletSummary';
import './walletSummary.css';

function WalletSummaryContent({ userId, refreshVersion }) {
  const { t, i18n } = useTranslation();
  const [version, setVersion] = useState(0);
  const [state, setState] = useState({
    loading: true,
    data: null,
    error: false,
  });
  const [open, setOpen] = useState(false);
  const root = useRef(null);
  const trigger = useRef(null);
  useEffect(() => {
    const abort = new AbortController();
    let active = true;
    setState({ loading: true, data: null, error: false });
    API.get('/api/data/self/wallet', {
      headers: { 'New-API-User': userId },
      disableDuplicate: true,
      skipGlobalLoading: true,
      skipErrorHandler: true,
      signal: abort.signal,
    })
      .then(({ data }) => {
        if (!active) return;
        if (!data?.success || !validWalletSummary(data.data, userId))
          throw new Error('invalid wallet summary');
        setState({ loading: false, data: data.data, error: false });
      })
      .catch(() => {
        if (active && !abort.signal.aborted)
          setState({ loading: false, data: null, error: true });
      });
    return () => {
      active = false;
      abort.abort();
    };
  }, [userId, refreshVersion, version]);
  useEffect(() => {
    if (!open) return;
    const outside = (event) => {
      if (!root.current?.contains(event.target)) setOpen(false);
    };
    const escape = (event) => {
      if (event.key === 'Escape') {
        setOpen(false);
        trigger.current?.focus();
      }
    };
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [open]);
  const data = state.data;
  const estimate = walletEstimate(data);
  const days = estimate.days
    ? new Intl.NumberFormat(i18n.language, {
        maximumFractionDigits: 1,
        roundingMode: 'floor',
      }).format(Math.floor(estimate.days * 10) / 10)
    : '';
  const status = state.loading
    ? t('加载中...')
    : state.error
      ? t('加载失败，请重试')
      : data?.balance <= 0
        ? t('待充值')
        : estimate.kind === 'urgent'
          ? t('预计不足1天')
          : ['normal', 'low'].includes(estimate.kind)
            ? t('预计可用约 {{days}} 天', { days })
            : estimate.kind === 'idle'
              ? t('暂无净消耗')
              : t('暂不估算');
  const amount = data ? renderQuota(data.balance) : '—';
  const reason =
    data?.usage.status === 'not_recorded'
      ? t('消费日志未启用，暂不估算。')
      : data?.usage.status === 'unavailable' || state.error
        ? t('加载失败，请重试')
        : null;
  return (
    <div className='dashboard-wallet' ref={root}>
      <div className='dashboard-wallet-strip'>
        <button
          ref={trigger}
          type='button'
          className='dashboard-wallet-trigger'
          aria-expanded={open}
          aria-controls='dashboard-wallet-details'
          onClick={() => setOpen((v) => !v)}
        >
          <Wallet size={15} aria-hidden='true' />
          <span>{t('余额')}</span>
          <strong aria-busy={state.loading}>{amount}</strong>
          <ChevronDown size={12} aria-hidden='true' />
        </button>
        <span
          className={`dashboard-wallet-status is-${data?.balance <= 0 ? 'empty' : estimate.kind}`}
        >
          <i aria-hidden='true' />
          {status}
        </span>
        <Link className='dashboard-wallet-topup' to='/console/topup'>
          {t('充值')}
          <ArrowUpRight size={13} aria-hidden='true' />
        </Link>
      </div>
      {open && (
        <section
          id='dashboard-wallet-details'
          className='dashboard-wallet-popover'
          aria-label={t('钱包余额与预估')}
        >
          <div className='dashboard-wallet-heading'>
            <span>{t('钱包余额')}</span>
            <button
              type='button'
              disabled={state.loading}
              aria-label={t('刷新')}
              onClick={() => setVersion((v) => v + 1)}
            >
              <RefreshCw size={15} />
            </button>
          </div>
          <div className='dashboard-wallet-amount'>{amount}</div>
          <div className='dashboard-wallet-metrics'>
            <div>
              <span>{t('近24小时消耗')}</span>
              <strong>
                {data?.usage.status === 'ok'
                  ? renderQuota(data.usage.net_quota)
                  : '—'}
              </strong>
            </div>
            <div>
              <span>{t('预计可用')}</span>
              <strong>{status}</strong>
            </div>
          </div>
          {reason && (
            <p role='status' className='dashboard-wallet-reason'>
              {reason}{' '}
              {(state.error || data?.usage.status === 'unavailable') && (
                <button
                  type='button'
                  disabled={state.loading}
                  onClick={() => setVersion((v) => v + 1)}
                >
                  {t('重试')}
                </button>
              )}
            </p>
          )}
          <p className='dashboard-wallet-note'>
            {t(
              '按近24小时用量估算，退款冲减消耗，仅供参考；不随看板筛选变化。',
            )}
          </p>
          {data?.balance <= 0 && (
            <p className='dashboard-wallet-note'>
              {t('钱包余额为零不影响仍有额度的套餐。')}
            </p>
          )}
          {data && (
            <time
              className='dashboard-wallet-time'
              dateTime={new Date(data.as_of * 1000).toISOString()}
            >
              {new Date(data.as_of * 1000).toLocaleString(i18n.language)}
            </time>
          )}
        </section>
      )}
    </div>
  );
}

export default function WalletSummary(props) {
  const [state] = useContext(UserContext);
  const userId = state?.user?.id;
  return userId ? (
    <WalletSummaryContent key={userId} userId={userId} {...props} />
  ) : null;
}
