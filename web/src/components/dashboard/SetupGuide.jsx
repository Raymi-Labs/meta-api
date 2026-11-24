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

// Adapted from QuantumNous/new-api's overview setup guide (Copyright (C) 2023-2026 QuantumNous).
import React, { useEffect, useId, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Check,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  ListChecks,
  Terminal,
} from 'lucide-react';
import { API } from '../../helpers/apiCore';
import { copy } from '../../helpers/clipboard';
import { renderQuota } from '../../helpers/render';
import { showError, showSuccess } from '../../helpers/utils';
import {
  buildPlaygroundCode,
  getPlaygroundEndpoint,
  getPlaygroundSetup,
} from '../../helpers/playgroundCode';
import { canOfferTextProtocolExamples } from '../../helpers/modelProtocols';
import {
  getTopNavigationLinks,
  parseHeaderModules,
} from '../../helpers/topNavigation';
import { useSidebar } from '../../hooks/common/useSidebar';

const readOptions = {
  timeout: 15000,
  skipErrorHandler: true,
  skipGlobalLoading: true,
  disableDuplicate: true,
};

async function readData(url, signal) {
  const { data } = await API.get(url, { ...readOptions, signal });
  if (!data?.success) throw new Error('Setup data unavailable');
  return data.data;
}

async function loadKeyCount(signal) {
  const page = await readData('/api/token/?p=1&page_size=1', signal);
  if (!Number.isSafeInteger(page?.total) || page.total < 0)
    throw new Error('Invalid key count');
  return page.total;
}

async function loadChatModel(signal) {
  const [names, catalog] = await Promise.all([
    readData('/api/user/models', signal),
    readData('/api/pricing', signal),
  ]);
  if (!Array.isArray(names) || !Array.isArray(catalog))
    throw new Error('Invalid model data');
  const allowed = new Set(names);
  return (
    catalog.find(
      (entry) =>
        allowed.has(entry.model_name) &&
        canOfferTextProtocolExamples(entry) &&
        entry.supported_endpoint_types.includes('openai'),
    )?.model_name || ''
  );
}

function useGuideData(load, revision) {
  const [result, setResult] = useState({});
  useEffect(() => {
    const controller = new AbortController();
    setResult({});
    load(controller.signal).then(
      (value) => {
        if (!controller.signal.aborted) setResult({ value });
      },
      () => {
        if (!controller.signal.aborted) setResult({ error: true });
      },
    );
    return () => controller.abort();
  }, [load, revision]);
  return result;
}

function FirstRequest({
  address,
  hasKey,
  uptimeEnabled,
  uptimeLoading,
  uptimeData,
  t,
}) {
  const [retry, setRetry] = useState(0);
  const model = useGuideData(loadChatModel, retry);
  const endpoint = getPlaygroundEndpoint(address);
  const code =
    model.value &&
    buildPlaygroundCode(
      {
        model: model.value,
        messages: [{ role: 'user', content: 'Say hello in one sentence.' }],
      },
      'curl',
      address,
    );
  const monitors = uptimeData.flatMap((group) => group.monitors || []);
  const healthy = monitors.filter((monitor) => monitor.status === 1).length;
  const handleCopy = async () => {
    if (await copy(code)) showSuccess(t('复制成功'));
    else showError(t('复制失败，请手动选择文本复制'));
  };
  return (
    <section className='dashboard-setup-preview'>
      <div className='dashboard-setup-line'>
        <h3>
          <Terminal size={17} />
          {t('首个 API 请求')}
        </h3>
        {code && (
          <button type='button' onClick={handleCopy}>
            {t('复制')}
          </button>
        )}
      </div>
      {hasKey === false && <p>{t('创建 API 密钥以解锁真实请求')}</p>}
      {code && (
        <>
          <pre className='dashboard-setup-env'>
            {getPlaygroundSetup('curl')}
          </pre>
          <pre tabIndex={0}>
            <code>{code}</code>
          </pre>
        </>
      )}
      <dl>
        <div>
          <dt>{t('API 地址')}</dt>
          <dd>{endpoint || t('暂不可用')}</dd>
        </div>
        <div>
          <dt>{t('API 密钥')}</dt>
          <dd>
            {hasKey === null ? '—' : hasKey ? t('已创建') : t('需要 API 密钥')}
          </dd>
        </div>
        <div>
          <dt>{t('已选择模型')}</dt>
          <dd>
            {model.error ? (
              <button onClick={() => setRetry(retry + 1)}>
                {t('加载失败，请重试')}
              </button>
            ) : model.value === undefined ? (
              t('加载中')
            ) : (
              model.value || t('暂无可用模型')
            )}
          </dd>
        </div>
        {uptimeEnabled && (
          <div>
            <dt>{t('服务可用性')}</dt>
            <dd>
              {uptimeLoading
                ? t('加载中')
                : monitors.length
                  ? `${t('正常')} ${healthy}/${monitors.length}`
                  : t('暂无监控数据')}
            </dd>
          </div>
        )}
      </dl>
    </section>
  );
}

export default function SetupGuide({
  user,
  status,
  accountLoading,
  accountError,
  onRetryAccount,
  revision,
  uptimeEnabled,
  uptimeLoading,
  uptimeData,
  t,
}) {
  const panelId = useId();
  const storageKey = `dashboard_overview_setup_guide_expanded:${user.id}`;
  const [preference, setPreference] = useState(() => {
    try {
      return localStorage.getItem(storageKey);
    } catch {
      return null;
    }
  });
  const { isModuleVisible } = useSidebar();
  const [retry, setRetry] = useState(0);
  const keys = useGuideData(loadKeyCount, revision + retry);
  const hasKey = keys.value === undefined ? null : keys.value > 0;
  const accountReady = !accountLoading && !accountError;
  const hasFunds =
    accountReady &&
    Number.isFinite(user.quota) &&
    Number.isFinite(user.used_quota)
      ? user.quota > 0 || user.used_quota > 0
      : null;
  const hasUsage =
    accountReady && Number.isFinite(user.request_count)
      ? user.request_count > 0
      : null;
  const topLinks = getTopNavigationLinks(
    t,
    '',
    parseHeaderModules(status?.HeaderNavModules),
  );
  const hasTopLink = (name) => topLinks.some((link) => link.itemKey === name);
  const steps = [
    {
      title: t('创建 API 密钥'),
      description: t('为你的应用或服务创建密钥'),
      to: '/console/token',
      done: hasKey,
      visible: isModuleVisible('console', 'token'),
    },
    {
      title: t('添加额度'),
      description: t('生产流量前保持充足余额'),
      to: '/console/topup',
      done: hasFunds,
      visible: isModuleVisible('personal', 'topup'),
    },
    {
      title: t('发送请求'),
      description: t('使用 Playground 或你的客户端验证路由'),
      to: '/playground',
      done: hasUsage,
      visible: isModuleVisible('chat', 'chat') || hasTopLink('playground'),
    },
  ].filter((step) => step.visible);
  const completed = steps.filter((step) => step.done).length;
  const known = steps.every((step) => step.done !== null);
  const incomplete = steps.some((step) => step.done === false);
  const expanded =
    preference === 'expanded' ||
    (preference !== 'collapsed' &&
      (incomplete || Boolean(keys.error) || accountError));
  const toggle = () => {
    const next = expanded ? 'collapsed' : 'expanded';
    setPreference(next);
    try {
      localStorage.setItem(storageKey, next);
    } catch {
      /* Session state still applies. */
    }
  };
  const actions = [
    [t('令牌管理'), '/console/token', isModuleVisible('console', 'token')],
    [
      t('渠道管理'),
      '/console/channel',
      user.role >= 10 && isModuleVisible('admin', 'channel'),
    ],
    [t('使用日志'), '/console/log', isModuleVisible('console', 'log')],
    [t('模型广场'), '/pricing', hasTopLink('pricing')],
  ].filter((action) => action[2]);

  if (!steps.length) return null;
  return (
    <section className={`dashboard-setup ${expanded ? 'is-open' : ''}`}>
      <div className='dashboard-setup-line'>
        <button
          type='button'
          className='dashboard-setup-toggle'
          aria-expanded={Boolean(expanded)}
          aria-controls={panelId}
          onClick={toggle}
        >
          <ListChecks size={17} />
          {t('开始使用')}
          {expanded ? <ChevronUp size={15} /> : <ChevronDown size={15} />}
        </button>
        {known && incomplete && (
          <span>
            {completed} / {steps.length}
          </span>
        )}
      </div>
      <div id={panelId} hidden={!expanded}>
        {expanded && (
          <>
            <div className='dashboard-setup-grid'>
              <div>
                <h2>{t('几分钟内开始使用你的 API 网关')}</h2>
                <p>{t('集中展示密钥、余额、路由和服务健康状态。')}</p>
                <ol>
                  {steps.map(({ title, description, to, done }, i) => (
                    <li key={to}>
                      <Link to={to}>
                        <span
                          className={
                            'dashboard-setup-step' + (done ? ' done' : '')
                          }
                        >
                          {done ? <Check size={15} /> : i + 1}
                        </span>
                        <span>
                          <strong>{title}</strong>
                          <small>{description}</small>
                        </span>
                        <span className='sr-only'>
                          {done === null
                            ? t('未知')
                            : done
                              ? t('已完成')
                              : t('未完成')}
                        </span>
                        <ChevronRight size={15} />
                      </Link>
                    </li>
                  ))}
                </ol>
                <div className='dashboard-setup-balance'>
                  {t('账户余额')}{' '}
                  <strong>
                    {accountReady && Number.isFinite(user.quota)
                      ? renderQuota(user.quota)
                      : '—'}
                  </strong>
                </div>
                {(keys.error || accountError) && (
                  <button
                    onClick={() => {
                      if (keys.error) setRetry(retry + 1);
                      if (accountError) onRetryAccount();
                    }}
                  >
                    {t('加载失败，请重试')}
                  </button>
                )}
              </div>
              <FirstRequest
                key={revision}
                address={status?.server_address || window.location.origin}
                hasKey={hasKey}
                uptimeEnabled={uptimeEnabled}
                uptimeLoading={uptimeLoading}
                uptimeData={uptimeData}
                t={t}
              />
            </div>
            {actions.length > 0 && (
              <div className='dashboard-setup-actions'>
                <div>
                  <small>{t('推荐操作')}</small>
                  <h3>{t('保持平台就绪')}</h3>
                </div>
                {actions.map(([label, to]) => (
                  <Link key={to} to={to}>
                    {label}
                    <ChevronRight size={13} />
                  </Link>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </section>
  );
}
