import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { buildAuthPathWithNext } from '../../helpers/authRedirect';

export default function DesktopLogin() {
  const { t } = useTranslation();
  const [info, setInfo] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const locked = useRef(false);
  const completed = useRef(false);
  const params = new URLSearchParams(window.location.search);
  const values = params.getAll('request_id');
  const requestId =
    values.length === 1 && /^[A-Za-z0-9_-]{43}$/.test(values[0])
      ? values[0]
      : '';
  const next = `/desktop-login?request_id=${encodeURIComponent(requestId)}`;
  useEffect(() => {
    let alive = true;
    let loading = false;
    // SPA navigation after OAuth does not re-fetch the /desktop-login HTML.
    // Enforce the privacy boundary here as well as on the document response.
    if (window.self !== window.top) {
      setError(t('授权请求无效，请从客户端重新发起'));
      return;
    }
    const meta = document.createElement('meta');
    meta.name = 'referrer';
    meta.content = 'no-referrer';
    document.head.appendChild(meta);
    if (!requestId) {
      setError(t('授权请求无效，请从客户端重新发起'));
      return () => meta.remove();
    }
    const load = async () => {
      if (loading || locked.current || completed.current) return;
      loading = true;
      setConfirmed(false);
      try {
        const response = await API.get('/api/desktop-auth/info', {
          params: { request_id: requestId },
          skipErrorHandler: true,
        });
        if (!response.data.success) throw new Error();
        if (alive) {
          setInfo(response.data.data);
          setConfirmed(false);
          setError('');
        }
      } catch {
        if (alive) {
          setInfo(null);
          setError(t('授权请求无效或已过期，请从客户端重新发起'));
        }
      } finally {
        loading = false;
      }
    };
    void load();
    const refresh = () => {
      if (document.visibilityState === 'visible') void load();
    };
    document.addEventListener('visibilitychange', refresh);
    return () => {
      alive = false;
      meta.remove();
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [requestId, t]);
  const approve = async (deny) => {
    if (
      locked.current ||
      completed.current ||
      !info?.logged_in ||
      (!deny && !confirmed)
    )
      return;
    locked.current = true;
    setBusy(true);
    setError('');
    try {
      const response = await API.post(
        '/api/desktop-auth/approve',
        {
          request_id: requestId,
          csrf: info.csrf,
          display_code: info.display_code,
          user_id: info.user_id,
          deny,
        },
        { skipErrorHandler: true },
      );
      if (!response.data.success) throw new Error();
      completed.current = true;
      setDone(true);
    } catch {
      setError(t('未能确认授权结果，请返回客户端查看；不要反复批准'));
    } finally {
      locked.current = false;
      setBusy(false);
    }
  };
  return (
    <main className='mx-auto max-w-lg px-6 py-16 space-y-5'>
      <h1 className='text-2xl font-semibold'>{t('登录元衡桌面客户端')}</h1>
      <p>
        {t(
          '只批准你刚刚在自己电脑上发起的登录。请核对客户端和网页的授权码一致，不要批准别人发来的链接。',
        )}
      </p>
      {error && (
        <p role='alert' className='text-red-600'>
          {error}
        </p>
      )}
      {done ? (
        <p role='status'>
          {t('已处理。请返回桌面客户端查看结果，可以关闭本页面。')}
        </p>
      ) : (
        info && (
          <>
            <p>
              {t('授权码')}：
              <strong className='text-2xl font-mono tracking-widest'>
                {info.display_code}
              </strong>
            </p>
            {info.logged_in ? (
              <>
                <p>
                  {t('将登录的元衡账号')}：<strong>{info.username}</strong>（ID{' '}
                  {info.user_id}）
                </p>
                <p>
                  {t(
                    '批准后，该客户端将获得此账号的登录会话，可访问账号、额度和工具配置。',
                  )}
                </p>
                <label className='flex gap-2'>
                  <input
                    type='checkbox'
                    checked={confirmed}
                    disabled={busy}
                    onChange={(e) => setConfirmed(e.target.checked)}
                  />
                  {t('这是我发起的登录，授权码和账号均正确')}
                </label>
                <div className='flex gap-4'>
                  <button
                    className='rounded bg-blue-600 text-white px-4 py-2 disabled:opacity-50'
                    disabled={busy || !confirmed}
                    onClick={() => approve(false)}
                  >
                    {t('批准客户端登录')}
                  </button>
                  <button
                    className='rounded border px-4 py-2'
                    disabled={busy}
                    onClick={() => approve(true)}
                  >
                    {t('拒绝')}
                  </button>
                </div>
                <p>
                  {t(
                    '账号不对时，请先在网站退出并登录正确账号，然后重新打开客户端中的授权页面。',
                  )}
                </p>
              </>
            ) : (
              <>
                <p>
                  {t(
                    '请先在网站使用 GitHub、Google 或已有账号登录，再回到这里确认。',
                  )}
                </p>
                <a
                  className='inline-block rounded bg-blue-600 text-white px-4 py-2'
                  href={buildAuthPathWithNext('/login', next)}
                >
                  {t('前往网站登录')}
                </a>
              </>
            )}
          </>
        )
      )}
    </main>
  );
}
