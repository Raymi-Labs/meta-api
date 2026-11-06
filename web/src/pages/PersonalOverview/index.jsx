import React, { useCallback, useContext, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowUpRight,
  KeyRound,
  Settings,
  Wallet,
  FileText,
  RotateCw,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';
import { renderQuota } from '../../helpers/render';
import UsageCalendar from '../../components/dashboard/UsageCalendar';
import './personalOverview.css';
import { maskedEmail } from '../../helpers/personalOverview';

function usePersonalResource(url, userId, params) {
  const [version, setVersion] = useState(0);
  const [state, setState] = useState({
    loading: true,
    data: null,
    error: false,
  });
  const query = JSON.stringify(params || {});
  useEffect(() => {
    const cancel = new AbortController();
    let active = true;
    setState({ loading: true, data: null, error: false });
    API.get(url, {
      params: JSON.parse(query),
      headers: { 'New-API-User': userId },
      signal: cancel.signal,
      disableDuplicate: true,
      skipGlobalLoading: true,
    })
      .then((response) => {
        if (!active) return;
        if (
          !response.data?.success ||
          !response.data.data ||
          (url === '/api/user/self' && response.data.data.id !== userId)
        )
          throw new Error('invalid self response');
        setState({ loading: false, error: false, data: response.data.data });
      })
      .catch(() => {
        if (active && !cancel.signal.aborted)
          setState({ loading: false, error: true, data: null });
      });
    return () => {
      active = false;
      cancel.abort();
    };
  }, [url, userId, query, version]);
  return { ...state, retry: useCallback(() => setVersion((v) => v + 1), []) };
}
function Retry({ resource, t }) {
  return (
    <div className='personal-overview-error' role='alert'>
      <span>{t('加载失败，请重试')}</span>
      <button type='button' onClick={resource.retry}>
        <RotateCw size={14} />
        {t('重试')}
      </button>
    </div>
  );
}
const stateLabels = {
  enabled: '已启用',
  disabled: '已停用',
  expired: '已过期',
  exhausted: '额度耗尽',
};
function PersonalOverviewContent({ userId }) {
  const { t, i18n } = useTranslation();
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const account = usePersonalResource('/api/user/self', userId);
  const month = usePersonalResource('/api/data/self/month', userId, {
    timezone,
  });
  const tokens = usePersonalResource('/api/data/self/tokens', userId);
  const summary = month.data?.summary;
  const monthValid =
    summary?.source === 'retained_settlement_logs_v1' &&
    Number.isSafeInteger(summary.quota) &&
    Number.isSafeInteger(summary.requests) &&
    summary.requests >= 0 &&
    Number.isSafeInteger(summary.tokens) &&
    summary.tokens >= 0 &&
    month.data?.timezone === timezone &&
    Number.isInteger(month.data.active_days) &&
    month.data.active_days >= 0 &&
    month.data.active_days <= 31;
  const tokenData = tokens.data;
  const tokenValid =
    tokenData &&
    ['total', 'enabled', 'other'].every(
      (k) => Number.isSafeInteger(tokenData[k]) && tokenData[k] >= 0,
    ) &&
    tokenData.enabled + tokenData.other === tokenData.total &&
    Array.isArray(tokenData.recent) &&
    tokenData.recent.length <= 5 &&
    tokenData.recent.every(
      (r) =>
        typeof r.name === 'string' &&
        /^••••[^\r\n]{0,4}$/.test(r.masked_key) &&
        Object.hasOwn(stateLabels, r.status) &&
        Number.isFinite(r.created_time),
    );
  const name = account.data?.display_name || account.data?.username || '';
  const number = (value) => Number(value).toLocaleString(i18n.language);
  const date = (value) =>
    value > 0
      ? new Intl.DateTimeFormat(i18n.language, {
          year: 'numeric',
          month: 'short',
          day: 'numeric',
        }).format(new Date(value * 1000))
      : '—';
  return (
    <main
      className='personal-overview'
      aria-labelledby='personal-overview-title'
    >
      <header className='personal-overview-header'>
        <div className='personal-overview-identity'>
          <div className='personal-overview-avatar' aria-hidden='true'>
            {name ? Array.from(name)[0].toUpperCase() : '·'}
          </div>
          <div>
            <h1 id='personal-overview-title'>{t('个人总览')}</h1>
            <p>
              {account.loading ? t('加载中...') : name}
              {maskedEmail(account.data?.email) && (
                <span> · {maskedEmail(account.data.email)}</span>
              )}
            </p>
          </div>
        </div>
        <Link className='personal-overview-settings' to='/console/personal'>
          <Settings size={16} />
          {t('个人设置')}
        </Link>
      </header>
      {account.error && <Retry resource={account} t={t} />}
      <section
        className='personal-overview-month'
        aria-labelledby='personal-month-title'
      >
        <div className='personal-overview-section-heading'>
          <h2 id='personal-month-title'>{t('本月用量')}</h2>
          <span>
            {month.data?.month || ''} · {timezone}
          </span>
        </div>
        {month.error || (!month.loading && !monthValid) ? (
          <Retry resource={month} t={t} />
        ) : (
          <div className='personal-overview-metrics'>
            {[
              ['本月消费', monthValid ? renderQuota(summary.quota) : '—'],
              ['调用次数', monthValid ? number(summary.requests) : '—'],
              ['Token 用量', monthValid ? number(summary.tokens) : '—'],
              ['活跃天数', monthValid ? number(month.data.active_days) : '—'],
            ].map(([label, value]) => (
              <div key={label}>
                <span>{t(label)}</span>
                <strong aria-busy={month.loading}>{value}</strong>
              </div>
            ))}
          </div>
        )}
        <p className='personal-overview-note'>
          {t(
            '统计依据留存的消费与退款记录；退款冲减消费，补差不重复计入调用。',
          )}
        </p>
      </section>
      <UsageCalendar t={t} userId={userId} defaultOpen />
      <section
        className='personal-overview-tokens'
        aria-labelledby='personal-tokens-title'
      >
        <div className='personal-overview-section-heading'>
          <h2 id='personal-tokens-title'>
            <KeyRound size={18} />
            {t('令牌概况')}
            {tokenValid && (
              <span className='personal-overview-count'>{tokenData.total}</span>
            )}
          </h2>
          <Link to='/console/token'>
            {t('管理令牌')}
            <ArrowUpRight size={16} />
          </Link>
        </div>
        {tokens.error || (!tokens.loading && !tokenValid) ? (
          <Retry resource={tokens} t={t} />
        ) : (
          <>
            <div className='personal-overview-token-stats'>
              <div>
                <span>{t('已启用')}</span>
                <strong>{tokenValid ? number(tokenData.enabled) : '—'}</strong>
              </div>
              <div>
                <span>{t('其他状态')}</span>
                <strong>{tokenValid ? number(tokenData.other) : '—'}</strong>
              </div>
              <div>
                <span>{t('最近创建')}</span>
                <strong>
                  {tokenValid && tokenData.recent.length
                    ? date(tokenData.recent[0].created_time)
                    : '—'}
                </strong>
              </div>
            </div>
            {tokens.loading ? (
              <p role='status'>{t('加载中...')}</p>
            ) : tokenData.recent.length === 0 ? (
              <div className='personal-overview-empty'>
                {t('还没有令牌，前往令牌管理创建第一枚。')}
                <Link to='/console/token'>
                  {t('管理令牌')}
                  <ArrowUpRight size={16} />
                </Link>
              </div>
            ) : (
              <div className='personal-overview-table-scroll'>
                <table>
                  <thead>
                    <tr>
                      {['名称', '令牌标识', '状态', '创建时间'].map((k) => (
                        <th key={k}>{t(k)}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {tokenData.recent.map((row) => (
                      <tr key={row.id}>
                        <td>{row.name || t('未命名')}</td>
                        <td>
                          <code>{row.masked_key}</code>
                        </td>
                        <td>
                          <span
                            className={`personal-token-status ${row.status}`}
                          >
                            {t(stateLabels[row.status])}
                          </span>
                        </td>
                        <td>{date(row.created_time)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
        <p className='personal-overview-note'>
          {t('显示最近 5 枚令牌，不含系统生图临时令牌；已启用不代表额度充足。')}
        </p>
      </section>
      <nav className='personal-overview-shortcuts' aria-label={t('快捷入口')}>
        {[
          ['/console/topup', '钱包管理', Wallet],
          ['/console/log', '使用日志', FileText],
          ['/console/personal', '个人设置', Settings],
        ].map(([to, label, Icon]) => (
          <Link key={to} to={to}>
            <Icon size={18} />
            <span>{t(label)}</span>
            <ArrowUpRight size={16} />
          </Link>
        ))}
      </nav>
    </main>
  );
}
export default function PersonalOverview() {
  const [state] = useContext(UserContext);
  const id = state?.user?.id;
  return id ? <PersonalOverviewContent key={id} userId={id} /> : null;
}
