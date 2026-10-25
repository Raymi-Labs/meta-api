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

import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Banner,
  Button,
  Card,
  Collapsible,
  Empty,
  Input,
  Progress,
  TabPane,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoAccess,
  IllustrationNoAccessDark,
} from '@douyinfe/semi-illustrations';
import { IconSearch } from '@douyinfe/semi-icons';
import {
  Infinity,
  KeyRound,
  Layers,
  MessageSquareText,
  Package,
  Shield,
  TableProperties,
  Wallet,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, renderQuota, showError, showWarning } from '../../helpers';
import {
  formatConcurrencyLabel,
  formatWindowLimitShort,
} from '../../helpers/render';
import { StatusContext } from '../../context/Status';
import { useSiteLanguage } from '../../hooks/common/useSiteLanguage';
import { getSiteLanguageSnapshot } from '../../helpers/siteLanguage';
import { useSetTheme, useTheme } from '../../context/Theme';
import ThemeToggle from '../../components/layout/headerbar/ThemeToggle';
import LanguageSelector from '../../components/layout/headerbar/LanguageSelector';
import CardTable from '../../components/common/ui/CardTable';
import { useLatestRequestGuard } from '../../hooks/common/useLatestRequestGuard';
import { getLogo, getSystemName } from '../../helpers/storage';
import './usage.css';

const { Title, Text } = Typography;

const QUERY_MODES = [
  {
    key: 'single',
    label: '单一',
    title: '查询单个 Key 的完整详情',
    placeholder: '请输入单个 API Key',
    note: '适合排查某一个 Key 的额度、套餐和模型消耗。',
  },
  {
    key: 'aggregate',
    label: '聚合',
    title: '同时查询多个 Key 的总览',
    placeholder: '请输入多个 API Key，使用英文逗号分隔',
    note: '适合做整体总览，结果会同时展示聚合数据和每 Key 明细。',
  },
];

const PERIOD_OPTIONS = [
  { key: 'today', label: '今日' },
  { key: 'week', label: '本周' },
  { key: 'month', label: '本月' },
];

const PERIOD_LABELS = {
  hourly: '每小时',
  daily: '每天',
  weekly: '每周',
  monthly: '每月',
  yearly: '每年',
  custom: '自定义',
  none: '无',
};

const TOKEN_STATUS_MAP = {
  1: { text: '已启用', color: 'green' },
  2: { text: '已禁用', color: 'red' },
  3: { text: '已过期', color: 'grey' },
  4: { text: '已耗尽', color: 'orange' },
};

const SHARE_TONES = ['blue', 'teal', 'gold', 'violet'];

function formatNumber(value) {
  const num = Number(value || 0);
  if (!Number.isFinite(num)) return '0';
  if (num >= 1e9) return `${(num / 1e9).toFixed(1)}B`;
  if (num >= 1e6) return `${(num / 1e6).toFixed(1)}M`;
  if (num >= 1e3) return `${(num / 1e3).toFixed(1)}K`;
  return String(num);
}

function formatTimestamp(ts) {
  if (!ts || ts === 0) return null;
  return new Date(ts * 1000).toLocaleString(getSiteLanguageSnapshot().language);
}

function formatExactNumber(value) {
  const num = Number(value || 0);
  if (!Number.isFinite(num)) return '0';
  return num.toLocaleString(getSiteLanguageSnapshot().language);
}

function getCacheSummaryText(record, t) {
  const cacheReadTokens = Number(record?.cache_read_tokens || 0);
  const cacheWriteTokens = Number(record?.cache_write_tokens || 0);

  if (cacheReadTokens > 0 && cacheWriteTokens > 0) {
    return `${t('缓存读')} ${formatExactNumber(cacheReadTokens)} · ${t(
      '写',
    )} ${formatExactNumber(cacheWriteTokens)}`;
  }
  if (cacheReadTokens > 0) {
    return `${t('缓存读')} ${formatExactNumber(cacheReadTokens)}`;
  }
  if (cacheWriteTokens > 0) {
    return `${t('缓存写')} ${formatExactNumber(cacheWriteTokens)}`;
  }
  return '';
}

function getRuntimeLimitText(source, t) {
  const maxConcurrency = Number(source?.max_concurrency || 0);
  const windowRequestLimit = Number(source?.window_request_limit || 0);
  const windowSeconds = Number(source?.window_seconds || 0);
  const parts = [];

  if (maxConcurrency > 0) {
    parts.push(formatConcurrencyLabel(maxConcurrency, t));
  }
  if (windowRequestLimit > 0 && windowSeconds > 0) {
    parts.push(formatWindowLimitShort(windowSeconds, windowRequestLimit, t));
  }

  return parts.length ? parts.join(' · ') : t('无限制');
}

function getRuntimeProgressColor(pct) {
  if (pct > 90) return 'var(--semi-color-danger)';
  if (pct >= 70) return 'var(--semi-color-warning)';
  return 'var(--semi-color-primary)';
}

function formatCountdown(remainMs) {
  if (remainMs <= 0) return null;
  const totalSec = Math.ceil(remainMs / 1000);
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  return `${String(min).padStart(2, '0')}:${String(sec).padStart(2, '0')}`;
}

function RuntimeLimitPanel({ data, t }) {
  const maxConcurrency = Number(data?.max_concurrency || 0);
  const windowRequestLimit = Number(data?.window_request_limit || 0);
  const windowSeconds = Number(data?.window_seconds || 0);
  const rs = data?.runtime_status || {};

  const currentConcurrency = Number(rs.current_concurrency || 0);
  const windowUsed = Number(rs.window_used || 0);
  const windowEndMs = Number(rs.window_end_ms || 0);
  const serverNowMs = Number(rs.server_now_ms || 0);

  const hasConcurrency = maxConcurrency > 0;
  const hasWindow = windowRequestLimit > 0 && windowSeconds > 0;

  const [countdown, setCountdown] = useState('');
  const offsetRef = useRef(0);

  useEffect(() => {
    if (!hasWindow || !windowEndMs || !serverNowMs) {
      setCountdown('');
      return;
    }
    offsetRef.current = Date.now() - serverNowMs;
    const tick = () => {
      const adjusted = Date.now() - offsetRef.current;
      const remain = windowEndMs - adjusted;
      if (remain <= 0) {
        setCountdown(t('窗口已重置'));
      } else {
        setCountdown(formatCountdown(remain) + ' ' + t('后重置'));
      }
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [hasWindow, windowEndMs, serverNowMs, t]);

  if (!hasConcurrency && !hasWindow) {
    return (
      <div className='usage-meta-item' style={{ gridColumn: 'span 1' }}>
        <span className='usage-meta-item__label'>{t('运行限制')}</span>
        <span className='usage-meta-item__value'>
          <Tag color='green'>{t('无限制')}</Tag>
        </span>
      </div>
    );
  }

  const concurrencyPct = hasConcurrency
    ? Math.min((currentConcurrency / maxConcurrency) * 100, 100)
    : 0;
  const windowPct = hasWindow
    ? Math.min((windowUsed / windowRequestLimit) * 100, 100)
    : 0;

  return (
    <div className='usage-meta-item' style={{ gridColumn: 'span 1' }}>
      <span className='usage-meta-item__label'>{t('运行限制')}</span>
      <div className='usage-runtime-panel'>
        {hasConcurrency && (
          <div className='usage-runtime-item'>
            <div className='usage-runtime-item__header'>
              <span>{t('当前并发')}</span>
              <span className='usage-runtime-item__value'>
                {currentConcurrency}/{maxConcurrency}
              </span>
            </div>
            <Progress
              percent={concurrencyPct}
              size='small'
              stroke={getRuntimeProgressColor(concurrencyPct)}
              showInfo={false}
              style={{ height: 6 }}
            />
          </div>
        )}
        {hasWindow && (
          <div className='usage-runtime-item'>
            <div className='usage-runtime-item__header'>
              <span>{t('窗口请求')}</span>
              <span className='usage-runtime-item__value'>
                {windowUsed}/{windowRequestLimit}
              </span>
            </div>
            <Progress
              percent={windowPct}
              size='small'
              stroke={getRuntimeProgressColor(windowPct)}
              showInfo={false}
              style={{ height: 6 }}
            />
            {countdown && (
              <span className='usage-runtime-countdown'>{countdown}</span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function parseKeys(raw) {
  return [
    ...new Set(
      String(raw || '')
        .split(/[,\n，]/)
        .map((item) => item.trim())
        .filter(Boolean)
        .map((item) => (item.startsWith('sk-') ? item.slice(3) : item)),
    ),
  ];
}

function getDistribution(source) {
  const distribution = source?.token_distribution || {};
  const input = Number(
    distribution.input_tokens ??
      source?.input_tokens ??
      source?.prompt_tokens ??
      0,
  );
  const completion = Number(
    distribution.completion_tokens ?? source?.completion_tokens ?? 0,
  );
  const cacheRead = Number(
    distribution.cache_read_tokens ?? source?.cache_read_tokens ?? 0,
  );
  const cacheCreation = Number(
    distribution.cache_creation_tokens ?? source?.cache_creation_tokens ?? 0,
  );
  const total = Number(
    distribution.total_tokens ??
      source?.total_tokens ??
      source?.period_token_count ??
      input + completion + cacheRead + cacheCreation,
  );
  const cacheCreationSupported = Boolean(
    distribution.cache_creation_supported ??
      source?.cache_creation_supported ??
      cacheCreation > 0,
  );
  return {
    input,
    completion,
    cacheRead,
    cacheCreation,
    total,
    cacheCreationSupported,
  };
}

function getPercent(used, total) {
  if (!total || total <= 0) return 0;
  return Math.min(100, Math.max(0, (Number(used || 0) / Number(total)) * 100));
}

function getIdentity(item) {
  const id = Number(item?.token_id ?? item?.id ?? 0);
  if (id > 0) return `id:${id}`;
  return (
    item?.masked_key ||
    item?.raw_key ||
    item?.normalized_key ||
    item?.name ||
    ''
  );
}

function mergeAggregateDetails(usageData, statsData) {
  const merged = new Map();
  const usageItems =
    usageData?.tokens || usageData?.items || usageData?.key_details || [];
  const statItems = statsData?.key_stats || [];

  usageItems.forEach((item) => {
    const key = getIdentity(item);
    if (key) merged.set(key, { ...item });
  });

  statItems.forEach((item) => {
    const key = getIdentity(item);
    if (!key) return;
    merged.set(key, { ...(merged.get(key) || {}), ...item });
  });

  return [...merged.values()].sort(
    (a, b) => Number(b.period_quota || 0) - Number(a.period_quota || 0),
  );
}

function OverviewCard({ label, value, sub }) {
  return (
    <div className='usage-overview-card'>
      <div className='usage-overview-card__label'>{label}</div>
      <div className='usage-overview-card__value'>{value}</div>
      {sub ? <div className='usage-overview-card__sub'>{sub}</div> : null}
    </div>
  );
}

function ProgressBlock({ label, value, percent, helper, tone = 'blue' }) {
  return (
    <div className={`usage-progress usage-progress--${tone}`}>
      <div className='usage-progress__top'>
        <span>{label}</span>
        <strong>{value}</strong>
      </div>
      <div className='usage-progress__track'>
        <div
          className='usage-progress__fill'
          style={{ width: `${percent}%` }}
        />
      </div>
      {helper ? <div className='usage-progress__helper'>{helper}</div> : null}
    </div>
  );
}

function ShareBreakdown({ items, emptyText }) {
  const validItems = items.filter(
    (item) =>
      item.alwaysVisible ||
      item.unavailable ||
      Number(item.rawValue || 0) > 0 ||
      Number(item.percent || 0) > 0,
  );

  if (!validItems.length) {
    return <div className='usage-placeholder'>{emptyText}</div>;
  }

  return (
    <div className='usage-share-breakdown'>
      <div className='usage-share-breakdown__list'>
        {validItems.map((item) => (
          <div
            className={`usage-share-breakdown__row ${item.unavailable ? 'is-unavailable' : ''}`}
            key={item.key}
          >
            <div className='usage-share-breakdown__top'>
              <div className='usage-share-breakdown__label'>
                <span
                  className={`usage-share-breakdown__dot usage-share-breakdown__dot--${item.tone || 'blue'} ${item.unavailable ? 'is-unavailable' : ''}`}
                />
                <div className='usage-share-breakdown__label-text'>
                  <strong>{item.label}</strong>
                  {item.meta ? <span>{item.meta}</span> : null}
                </div>
              </div>
              <div className='usage-share-breakdown__value'>
                <strong>{item.value}</strong>
                {!item.unavailable ? (
                  <span>{item.percent.toFixed(1)}%</span>
                ) : null}
              </div>
            </div>
            {!item.unavailable ? (
              <div className='usage-share-breakdown__track'>
                <div
                  className={`usage-share-breakdown__fill usage-share-breakdown__fill--${item.tone || 'blue'}`}
                  style={{ width: `${item.percent}%` }}
                />
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}

function MetaItem({ label, value }) {
  return (
    <div className='usage-meta-item'>
      <span className='usage-meta-item__label'>{label}</span>
      <strong className='usage-meta-item__value'>{value}</strong>
    </div>
  );
}

export default function Usage() {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const { language: currentLang, changeLanguage: handleLanguageChange } =
    useSiteLanguage();
  const theme = useTheme();
  const setTheme = useSetTheme();
  const [queryMode, setQueryMode] = useState('single');
  const [queryValue, setQueryValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [statsLoading, setStatsLoading] = useState(false);
  const [usageData, setUsageData] = useState(null);
  const [statsData, setStatsData] = useState(null);
  const [period, setPeriod] = useState('month');
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailRows, setDetailRows] = useState([]);
  const [detailTotal, setDetailTotal] = useState(0);
  const [detailPage, setDetailPage] = useState(1);
  const [detailPageSize, setDetailPageSize] = useState(10);
  const usageRequestGuard = useLatestRequestGuard();
  const statsRequestGuard = useLatestRequestGuard();
  const detailRequestGuard = useLatestRequestGuard();

  const enabled = useMemo(() => {
    try {
      const modules = JSON.parse(statusState?.status?.HeaderNavModules || '{}');
      return modules.usage === true;
    } catch {
      return false;
    }
  }, [statusState?.status?.HeaderNavModules]);

  const systemName = getSystemName();
  const logo = getLogo();
  const queryModeConfig = useMemo(
    () => QUERY_MODES.find((item) => item.key === queryMode) || QUERY_MODES[0],
    [queryMode],
  );
  const periodLabel = useMemo(
    () =>
      t(PERIOD_OPTIONS.find((item) => item.key === period)?.label || '本月'),
    [period, t],
  );
  const activeSingleKey = useMemo(
    () => (usageData?.mode === 'single' ? usageData?.queryKeys?.[0] || '' : ''),
    [usageData],
  );

  const invalidKeys = useMemo(() => {
    const keys = new Set();
    [
      usageData?.invalid_keys,
      statsData?.invalid_keys,
      usageData?.missing_keys,
      statsData?.missing_keys,
    ].forEach((list) => {
      (list || []).forEach((item) =>
        keys.add(typeof item === 'string' ? item : item?.key),
      );
    });
    return [...keys].filter(Boolean);
  }, [statsData, usageData]);

  const handleThemeToggle = useCallback(
    (nextTheme) => {
      if (['light', 'dark', 'auto'].includes(nextTheme)) setTheme(nextTheme);
    },
    [setTheme],
  );

  const fetchStats = useCallback(async (mode, keys, nextPeriod) => {
    const endpoint =
      mode === 'single'
        ? '/api/usage/public_token/stats'
        : '/api/usage/public_token/stats/batch';
    const payload =
      mode === 'single'
        ? { key: keys[0], period: nextPeriod }
        : { keys, period: nextPeriod };
    const res = await API.post(endpoint, payload);
    return res.data?.success ? res.data.data : null;
  }, []);

  const resetDetailLogs = useCallback(() => {
    detailRequestGuard.createRequestId();
    setDetailOpen(false);
    setDetailLoading(false);
    setDetailRows([]);
    setDetailTotal(0);
    setDetailPage(1);
    setDetailPageSize(10);
  }, [detailRequestGuard]);

  const fetchDetailLogs = useCallback(
    async (
      page = 1,
      pageSize = detailPageSize,
      nextPeriod = period,
      keyOverride = activeSingleKey,
    ) => {
      const nextKey = keyOverride || activeSingleKey;
      if (!nextKey) return;
      const requestId = detailRequestGuard.createRequestId();
      setDetailLoading(true);
      try {
        const res = await API.post('/api/usage/public_token/logs', {
          key: nextKey,
          period: nextPeriod,
          page,
          page_size: pageSize,
        });
        if (!res.data?.success) {
          throw new Error(res.data?.message || t('加载调用明细失败'));
        }
        if (!detailRequestGuard.isLatestRequest(requestId)) {
          return;
        }
        const payload = res.data?.data || {};
        setDetailRows(payload.items || []);
        setDetailTotal(Number(payload.total || 0));
        setDetailPage(Number(payload.page || page));
        setDetailPageSize(Number(payload.page_size || pageSize));
      } catch {
        if (detailRequestGuard.isLatestRequest(requestId)) {
          showError(t('加载调用明细失败'));
        }
      } finally {
        if (detailRequestGuard.isLatestRequest(requestId)) {
          setDetailLoading(false);
        }
      }
    },
    [activeSingleKey, detailPageSize, detailRequestGuard, period, t],
  );

  const runQuery = useCallback(
    async (mode, nextPeriod = period) => {
      const keys = parseKeys(queryValue);
      if (!keys.length) return showError(t('请输入 API Key'));
      if (mode === 'single' && keys.length !== 1)
        return showWarning(t('单一模式仅支持一个 Key'));

      const requestId = usageRequestGuard.createRequestId();
      setLoading(true);
      setUsageData(null);
      setStatsData(null);
      resetDetailLogs();
      try {
        const usageEndpoint =
          mode === 'single'
            ? '/api/usage/public_token'
            : '/api/usage/public_token/batch';
        const usagePayload = mode === 'single' ? { key: keys[0] } : { keys };
        const [usageRes, nextStats] = await Promise.all([
          API.post(usageEndpoint, usagePayload),
          fetchStats(mode, keys, nextPeriod).catch(() => null),
        ]);
        if (!usageRequestGuard.isLatestRequest(requestId)) {
          return;
        }
        if (!usageRes.data?.success)
          return showError(usageRes.data?.message || t('查询失败'));
        setUsageData({ mode, queryKeys: keys, ...usageRes.data.data });
        setStatsData(nextStats);
      } catch {
        if (usageRequestGuard.isLatestRequest(requestId)) {
          showError(t('查询失败，请稍后重试'));
        }
      } finally {
        if (usageRequestGuard.isLatestRequest(requestId)) {
          setLoading(false);
        }
      }
    },
    [fetchStats, period, queryValue, resetDetailLogs, t, usageRequestGuard],
  );

  const handlePeriodChange = useCallback(
    async (nextPeriod) => {
      setPeriod(nextPeriod);
      detailRequestGuard.createRequestId();
      if (!usageData?.queryKeys?.length) return;
      const requestId = statsRequestGuard.createRequestId();
      setStatsLoading(true);
      try {
        const nextStats = await fetchStats(
          usageData.mode,
          usageData.queryKeys,
          nextPeriod,
        );
        if (!statsRequestGuard.isLatestRequest(requestId)) {
          return;
        }
        setStatsData(nextStats);
        if (usageData.mode === 'single' && detailOpen) {
          await fetchDetailLogs(
            1,
            detailPageSize,
            nextPeriod,
            usageData.queryKeys[0],
          );
        }
      } catch {
        if (statsRequestGuard.isLatestRequest(requestId)) {
          showWarning(t('统计数据暂时不可用，已保留基础额度信息'));
        }
      } finally {
        if (statsRequestGuard.isLatestRequest(requestId)) {
          setStatsLoading(false);
        }
      }
    },
    [
      detailOpen,
      detailPageSize,
      fetchDetailLogs,
      fetchStats,
      detailRequestGuard,
      statsRequestGuard,
      t,
      usageData,
    ],
  );

  const singleStatus =
    usageData?.mode === 'single'
      ? TOKEN_STATUS_MAP[usageData.status] || { text: '未知', color: 'grey' }
      : null;
  const singleDistribution = getDistribution(statsData);
  const aggregateDistribution = getDistribution(
    statsData?.summary || statsData,
  );
  const aggregateDetails = useMemo(
    () =>
      usageData?.mode === 'aggregate'
        ? mergeAggregateDetails(usageData, statsData)
        : [],
    [statsData, usageData],
  );

  const aggregateSummary = useMemo(() => {
    const summary = {
      ...(usageData?.summary || {}),
      ...(statsData?.summary || {}),
    };
    if (usageData?.mode !== 'aggregate') return summary;
    return {
      total_granted:
        Number(summary.total_granted || 0) ||
        aggregateDetails.reduce(
          (sum, item) => sum + Number(item.total_granted || 0),
          0,
        ),
      total_used:
        Number(summary.total_used || 0) ||
        aggregateDetails.reduce(
          (sum, item) => sum + Number(item.total_used || 0),
          0,
        ),
      total_available:
        Number(summary.total_available || 0) ||
        aggregateDetails.reduce(
          (sum, item) => sum + Number(item.total_available || 0),
          0,
        ),
      period_quota: Number(summary.period_quota || 0),
      period_request_count: Number(summary.period_request_count || 0),
      rpm: Number(summary.rpm || 0),
      tpm: Number(summary.tpm || 0),
      valid_key_count: Number(
        summary.valid_key_count || summary.key_count || aggregateDetails.length,
      ),
    };
  }, [aggregateDetails, statsData, usageData]);

  const modelStats = useMemo(() => {
    const rows = [...(statsData?.model_stats || [])]
      .map((item) => ({
        ...item,
        quota: Number(item.quota || 0),
        count: Number(item.count || 0),
        prompt_tokens: Number(item.prompt_tokens || 0),
        completion_tokens: Number(item.completion_tokens || 0),
      }))
      .sort((a, b) => b.quota - a.quota);
    const totalQuota = rows.reduce((sum, row) => sum + row.quota, 0);
    return rows.map((item, index) => ({
      ...item,
      share: getPercent(item.quota, totalQuota),
      tone: SHARE_TONES[index % SHARE_TONES.length],
    }));
  }, [statsData]);

  const singleTokenItems = useMemo(() => {
    if (!statsData) return [];
    return [
      {
        key: 'input',
        label: t('输入 Token'),
        value: formatNumber(singleDistribution.input),
        rawValue: singleDistribution.input,
        percent: getPercent(singleDistribution.input, singleDistribution.total),
        tone: 'blue',
        alwaysVisible: true,
      },
      {
        key: 'completion',
        label: t('输出 Token'),
        value: formatNumber(singleDistribution.completion),
        rawValue: singleDistribution.completion,
        percent: getPercent(
          singleDistribution.completion,
          singleDistribution.total,
        ),
        tone: 'teal',
        alwaysVisible: true,
      },
      {
        key: 'cacheCreation',
        label: t('缓存创建 Token'),
        value: singleDistribution.cacheCreationSupported
          ? formatNumber(singleDistribution.cacheCreation)
          : t('未上报'),
        rawValue: singleDistribution.cacheCreation,
        percent: getPercent(
          singleDistribution.cacheCreation,
          singleDistribution.total,
        ),
        tone: 'gold',
        alwaysVisible: true,
        unavailable: !singleDistribution.cacheCreationSupported,
        meta: !singleDistribution.cacheCreationSupported
          ? t('当前模型或供应商未返回缓存创建统计')
          : null,
      },
      {
        key: 'cacheRead',
        label: t('缓存读取 Token'),
        value: formatNumber(singleDistribution.cacheRead),
        rawValue: singleDistribution.cacheRead,
        percent: getPercent(
          singleDistribution.cacheRead,
          singleDistribution.total,
        ),
        tone: 'violet',
        alwaysVisible: true,
      },
    ];
  }, [singleDistribution, statsData, t]);

  const aggregateTokenItems = useMemo(() => {
    if (!statsData) return [];
    return [
      {
        key: 'input',
        label: t('输入 Token'),
        value: formatNumber(aggregateDistribution.input),
        rawValue: aggregateDistribution.input,
        percent: getPercent(
          aggregateDistribution.input,
          aggregateDistribution.total,
        ),
        tone: 'blue',
        alwaysVisible: true,
      },
      {
        key: 'completion',
        label: t('输出 Token'),
        value: formatNumber(aggregateDistribution.completion),
        rawValue: aggregateDistribution.completion,
        percent: getPercent(
          aggregateDistribution.completion,
          aggregateDistribution.total,
        ),
        tone: 'teal',
        alwaysVisible: true,
      },
      {
        key: 'cacheCreation',
        label: t('缓存创建 Token'),
        value: aggregateDistribution.cacheCreationSupported
          ? formatNumber(aggregateDistribution.cacheCreation)
          : t('未上报'),
        rawValue: aggregateDistribution.cacheCreation,
        percent: getPercent(
          aggregateDistribution.cacheCreation,
          aggregateDistribution.total,
        ),
        tone: 'gold',
        alwaysVisible: true,
        unavailable: !aggregateDistribution.cacheCreationSupported,
        meta: !aggregateDistribution.cacheCreationSupported
          ? t('当前模型或供应商未返回缓存创建统计')
          : null,
      },
      {
        key: 'cacheRead',
        label: t('缓存读取 Token'),
        value: formatNumber(aggregateDistribution.cacheRead),
        rawValue: aggregateDistribution.cacheRead,
        percent: getPercent(
          aggregateDistribution.cacheRead,
          aggregateDistribution.total,
        ),
        tone: 'violet',
        alwaysVisible: true,
      },
    ];
  }, [aggregateDistribution, statsData, t]);

  const modelShareItems = useMemo(
    () =>
      modelStats.map((item) => ({
        key: item.model,
        label: item.model,
        value: renderQuota(item.quota || 0),
        rawValue: item.quota,
        percent: item.share,
        tone: item.tone,
        meta: `${formatNumber(item.count)} ${t('次请求')} · ${formatNumber(item.prompt_tokens)} / ${formatNumber(item.completion_tokens)} Token`,
      })),
    [modelStats, t],
  );

  const singlePackageText = useMemo(() => {
    if (!usageData?.package_enabled) return t('当前 Key 未启用套餐限制');
    return `${t(PERIOD_LABELS[usageData.package_period] || usageData.package_period || '无')} · ${usageData.package_next_reset_time ? `${t('下次重置')} ${formatTimestamp(usageData.package_next_reset_time)}` : t('暂无重置时间')}`;
  }, [t, usageData]);

  const detailColumns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'created_at',
        key: 'created_at',
        width: 176,
        render: (value) => formatTimestamp(value) || '-',
      },
      {
        title: t('模型'),
        dataIndex: 'model_name',
        key: 'model_name',
        width: 170,
        render: (value, record) => (
          <div className='usage-log-primary-cell'>
            <strong>{value || '-'}</strong>
            <span>{record?.is_stream ? t('流式') : t('非流式')}</span>
          </div>
        ),
      },
      {
        title: t('耗时'),
        dataIndex: 'use_time',
        key: 'use_time',
        width: 90,
        render: (value) =>
          Number(value || 0) > 0 ? `${value}${t('秒')}` : '-',
      },
      {
        title: t('输入 Token'),
        dataIndex: 'prompt_tokens',
        key: 'prompt_tokens',
        width: 180,
        render: (value, record) => {
          const cacheSummary = getCacheSummaryText(record, t);
          return (
            <div className='usage-log-token-cell'>
              <strong>{formatExactNumber(value)}</strong>
              {cacheSummary ? (
                <span className='usage-log-cache'>{cacheSummary}</span>
              ) : null}
            </div>
          );
        },
      },
      {
        title: t('输出 Token'),
        dataIndex: 'completion_tokens',
        key: 'completion_tokens',
        width: 120,
        render: (value) => formatExactNumber(value),
      },
      {
        title: t('费用'),
        dataIndex: 'quota',
        key: 'quota',
        width: 110,
        render: (value) => renderQuota(value || 0),
      },
      {
        title: t('详情'),
        dataIndex: 'content',
        key: 'content',
        render: (value) => (
          <div className='usage-log-detail'>{value || '-'}</div>
        ),
      },
    ],
    [t],
  );

  const handleToggleDetails = useCallback(async () => {
    const nextOpen = !detailOpen;
    setDetailOpen(nextOpen);
    if (nextOpen && !detailRows.length && activeSingleKey) {
      await fetchDetailLogs(1, detailPageSize, period, activeSingleKey);
    }
  }, [
    activeSingleKey,
    detailOpen,
    detailPageSize,
    detailRows.length,
    fetchDetailLogs,
    period,
  ]);

  const handleDetailPageChange = useCallback(
    (page) => {
      fetchDetailLogs(page, detailPageSize);
    },
    [detailPageSize, fetchDetailLogs],
  );

  const handleDetailPageSizeChange = useCallback(
    (pageSize) => {
      fetchDetailLogs(1, pageSize);
    },
    [fetchDetailLogs],
  );

  const renderPeriodTabs = () => (
    <Tabs
      className='usage-period-tabs'
      type='button'
      size='small'
      activeKey={period}
      onChange={handlePeriodChange}
    >
      {PERIOD_OPTIONS.map((option) => (
        <TabPane key={option.key} tab={t(option.label)} itemKey={option.key} />
      ))}
    </Tabs>
  );

  return (
    <div className='usage-page'>
      <div className='usage-shell'>
        <section className='usage-panel usage-console-panel'>
          <div className='usage-console-head'>
            <div className='usage-console-brand'>
              <div className='usage-brand'>
                {logo ? (
                  <img
                    className='usage-brand__logo'
                    src={logo}
                    alt={systemName}
                  />
                ) : null}
                <span className='usage-brand__name'>{systemName}</span>
              </div>
              <div className='usage-console-copy'>
                <Text className='usage-eyebrow'>{t('公开额度查询')}</Text>
                <Title heading={2} className='usage-hero__title'>
                  {t('快速查看 API Key 的额度、周期与模型消耗')}
                </Title>
                <Text className='usage-console-description'>
                  {t(
                    '现在支持单一查询和聚合查询，聚合模式会把多个 Key 的总览和每 Key 明细一起展示。',
                  )}
                </Text>
              </div>
            </div>
            <div className='usage-toolbar__inner'>
              <LanguageSelector
                currentLang={currentLang}
                onLanguageChange={handleLanguageChange}
                t={t}
              />
              <ThemeToggle
                theme={theme}
                onThemeToggle={handleThemeToggle}
                t={t}
              />
            </div>
          </div>
          {enabled ? (
            <div className='usage-query-panel'>
              <div className='usage-query-panel__head'>
                <Text className='usage-query-panel__eyebrow'>
                  {t('查询模式')}
                </Text>
                <div className='usage-query-panel__title'>
                  {t(queryModeConfig.title)}
                </div>
              </div>
              <div className='usage-mode-switch'>
                {QUERY_MODES.map((mode) => (
                  <button
                    key={mode.key}
                    type='button'
                    className={`usage-mode-switch__button ${queryMode === mode.key ? 'is-active' : ''}`}
                    onClick={() => {
                      setQueryMode(mode.key);
                      resetDetailLogs();
                    }}
                  >
                    <span>{t(mode.label)}</span>
                    <small>{t(mode.note)}</small>
                  </button>
                ))}
              </div>
              <div className='usage-query-field'>
                <Text className='usage-query-label'>{t('API Key')}</Text>
                <div className='usage-query-row'>
                  <Input
                    className='usage-query-input'
                    placeholder={t(queryModeConfig.placeholder)}
                    value={queryValue}
                    onChange={setQueryValue}
                    onEnterPress={() => runQuery(queryMode)}
                    mode={queryMode === 'single' ? 'password' : undefined}
                    size='large'
                  />
                  <Button
                    className='usage-query-button'
                    theme='solid'
                    icon={<IconSearch />}
                    loading={loading}
                    onClick={() => runQuery(queryMode)}
                    size='large'
                  >
                    {t('查询')}
                  </Button>
                </div>
              </div>
              <div className='usage-inline-note'>
                <Shield size={14} />
                <Text type='tertiary' size='small'>
                  {queryMode === 'single'
                    ? t('您的 Key 仅用于查询，不会被存储或记录')
                    : t(
                        '聚合模式请用英文逗号分隔多个 Key，系统会自动去重并汇总',
                      )}
                </Text>
              </div>
              {invalidKeys.length ? (
                <Banner
                  className='usage-alert'
                  type='warning'
                  description={t('以下 Key 未查询到：{{keys}}', {
                    keys: invalidKeys.join(', '),
                  })}
                />
              ) : null}
            </div>
          ) : (
            <div className='usage-empty-panel'>
              <Empty
                image={
                  <IllustrationNoAccess style={{ width: 140, height: 140 }} />
                }
                darkModeImage={
                  <IllustrationNoAccessDark
                    style={{ width: 140, height: 140 }}
                  />
                }
                title={t('功能未启用')}
                description={t('管理员未开启公开令牌额度查询功能')}
              />
            </div>
          )}
        </section>

        {enabled && usageData?.mode === 'single' ? (
          <section className='usage-section'>
            <div className='usage-section-header usage-section-header--compact'>
              <div>
                <Text className='usage-section-eyebrow'>{t('查询结果')}</Text>
                <Title heading={5} className='usage-section-title'>
                  {t('API 信息')}
                </Title>
              </div>
              {renderPeriodTabs()}
            </div>

            <div className='usage-results-stack'>
              <Card className='usage-card usage-card--hero'>
                <div className='usage-card-shell'>
                  <div className='usage-card-head'>
                    <div className='usage-card-title-wrap'>
                      <span className='usage-card-title'>
                        <KeyRound size={16} />
                        {t('API 信息')}
                      </span>
                      <Tag color={singleStatus?.color || 'grey'} size='small'>
                        {t(singleStatus?.text || '未知')}
                      </Tag>
                    </div>
                  </div>
                  <div className='usage-card-body usage-card-body--hero'>
                    <div className='usage-primary-stat'>
                      <div className='usage-primary-stat__main'>
                        <div className='usage-primary-stat__eyebrow'>
                          {usageData.group || t('默认分组')}
                        </div>
                        <div className='usage-primary-stat__title'>
                          {usageData.name || '-'}
                        </div>
                      </div>
                      <div className='usage-primary-stat__value'>
                        <span>
                          {usageData.unlimited_quota
                            ? t('无限')
                            : renderQuota(usageData.total_available)}
                        </span>
                        <small>{t('剩余额度')}</small>
                      </div>
                    </div>

                    <div className='usage-overview-grid usage-overview-grid--compact'>
                      <OverviewCard
                        label={t('总额度')}
                        value={
                          usageData.unlimited_quota
                            ? t('无限')
                            : renderQuota(usageData.total_granted || 0)
                        }
                        sub={`${t('已使用')} ${renderQuota(
                          usageData.total_used || 0,
                        )}`}
                      />
                      <OverviewCard
                        label={t('允许所有模型')}
                        value={
                          usageData.model_limits_enabled ? t('否') : t('是')
                        }
                        sub={usageData.group || t('默认分组')}
                      />
                    </div>

                    {usageData.unlimited_quota ? (
                      <div className='usage-unlimited-inline'>
                        <Infinity size={18} />
                        <span>
                          {t('已使用')} {renderQuota(usageData.total_used || 0)}
                        </span>
                      </div>
                    ) : (
                      <ProgressBlock
                        label={t('总额度使用进度')}
                        value={`${renderQuota(
                          usageData.total_used || 0,
                        )} / ${renderQuota(usageData.total_granted || 0)}`}
                        percent={getPercent(
                          usageData.total_used,
                          usageData.total_granted,
                        )}
                        helper={`${t('已使用')} ${renderQuota(
                          usageData.total_used || 0,
                        )} · ${t('剩余')} ${renderQuota(
                          usageData.total_available || 0,
                        )}`}
                      />
                    )}

                    <div className='usage-secondary-panel'>
                      <div className='usage-secondary-panel__head'>
                        <span className='usage-card-title'>
                          <Package size={15} />
                          {t('套餐信息')}
                        </span>
                      </div>
                      {usageData.package_enabled ? (
                        <ProgressBlock
                          label={t('套餐用量')}
                          value={`${renderQuota(
                            usageData.package_used_quota || 0,
                          )} / ${renderQuota(
                            usageData.package_limit_quota || 0,
                          )}`}
                          percent={getPercent(
                            usageData.package_used_quota,
                            usageData.package_limit_quota,
                          )}
                          helper={singlePackageText}
                          tone='teal'
                        />
                      ) : (
                        <div className='usage-inline-placeholder'>
                          {singlePackageText}
                        </div>
                      )}
                    </div>

                    <div className='usage-meta-grid usage-meta-grid--wide'>
                      <MetaItem
                        label={t('创建时间')}
                        value={formatTimestamp(usageData.created_time) || '-'}
                      />
                      <MetaItem
                        label={t('过期时间')}
                        value={
                          usageData.expires_at
                            ? formatTimestamp(usageData.expires_at)
                            : t('永不过期')
                        }
                      />
                      <MetaItem
                        label={t('最近访问')}
                        value={formatTimestamp(usageData.accessed_time) || '-'}
                      />
                      <MetaItem
                        label={t('总额度')}
                        value={
                          usageData.unlimited_quota
                            ? t('无限')
                            : renderQuota(usageData.total_granted || 0)
                        }
                      />
                      <RuntimeLimitPanel data={usageData} t={t} />
                    </div>
                  </div>
                </div>
              </Card>

              <div className='usage-kpi-grid'>
                <OverviewCard
                  label={`${periodLabel}${t('请求')}`}
                  value={
                    statsData
                      ? formatNumber(statsData.period_request_count)
                      : '--'
                  }
                  sub={
                    statsData
                      ? `${formatNumber(singleDistribution.total)} Token`
                      : t('统计返回后显示')
                  }
                />
                <OverviewCard
                  label={`${periodLabel}${t('消费')}`}
                  value={
                    statsData ? renderQuota(statsData.period_quota || 0) : '--'
                  }
                  sub={
                    statsData && (statsData.rpm > 0 || statsData.tpm > 0)
                      ? `${statsData.rpm || 0} RPM / ${formatNumber(
                          statsData.tpm || 0,
                        )} TPM`
                      : t('统计返回后显示')
                  }
                />
              </div>

              <div className='usage-distribution-grid'>
                <Card className='usage-card'>
                  <div className='usage-card-shell'>
                    <div className='usage-card-head'>
                      <div className='usage-card-title-wrap'>
                        <span className='usage-card-title'>
                          <MessageSquareText size={16} />
                          {t('Token 使用分布')}
                        </span>
                        <span className='usage-card-title__aside'>
                          {formatNumber(singleDistribution.total)} Token
                        </span>
                      </div>
                      <Text className='usage-card-head__description'>
                        {t('总计包含非缓存输入、输出、缓存创建与缓存读取。')}
                      </Text>
                    </div>
                    <div className='usage-card-body'>
                      <ShareBreakdown
                        items={singleTokenItems}
                        emptyText={t('暂无统计数据')}
                      />
                    </div>
                  </div>
                </Card>

                <Card className='usage-card usage-card--list'>
                  <div className='usage-card-shell'>
                    <div className='usage-card-head'>
                      <div className='usage-card-title-wrap'>
                        <span className='usage-card-title'>
                          <Layers size={16} />
                          {t('模型使用统计')}
                        </span>
                        <span className='usage-card-title__aside'>
                          {statsData
                            ? renderQuota(statsData.period_quota || 0)
                            : ''}
                        </span>
                      </div>
                    </div>
                    <div className='usage-card-body usage-card-body--list'>
                      {statsLoading ? (
                        <div className='usage-placeholder'>{t('加载中')}</div>
                      ) : modelShareItems.length ? (
                        <div className='usage-model-board'>
                          <div className='usage-model-board__summary'>
                            <div className='usage-model-board__item'>
                              <span>{`${periodLabel}${t('消费')}`}</span>
                              <strong>
                                {renderQuota(statsData?.period_quota || 0)}
                              </strong>
                            </div>
                            <div className='usage-model-board__item'>
                              <span>{`${periodLabel} Token`}</span>
                              <strong>
                                {formatNumber(singleDistribution.total)}
                              </strong>
                            </div>
                          </div>
                          <ShareBreakdown
                            items={modelShareItems}
                            emptyText={t('暂无统计数据')}
                          />
                        </div>
                      ) : (
                        <div className='usage-placeholder'>
                          {t('暂无统计数据')}
                        </div>
                      )}
                    </div>
                  </div>
                </Card>
              </div>

              <Card className='usage-card'>
                <div className='usage-card-shell'>
                  <div className='usage-card-head usage-card-head--action'>
                    <div className='usage-card-head__main'>
                      <div className='usage-card-title-wrap'>
                        <span className='usage-card-title'>
                          <TableProperties size={16} />
                          {t('调用明细')}
                        </span>
                        {detailTotal > 0 ? (
                          <span className='usage-card-title__aside'>
                            {formatExactNumber(detailTotal)} {t('次请求')}
                          </span>
                        ) : null}
                      </div>
                      <Text className='usage-card-head__description'>
                        {t('默认收起，需要时再展开查看当前周期内的调用记录。')}
                      </Text>
                    </div>
                    <Button
                      className='usage-detail-toggle'
                      theme='light'
                      onClick={handleToggleDetails}
                    >
                      {detailOpen ? t('收起') : t('展开')}
                    </Button>
                  </div>
                  <Collapsible isOpen={detailOpen} keepDOM>
                    <div className='usage-card-body usage-card-body--details'>
                      <CardTable
                        columns={detailColumns}
                        dataSource={detailRows}
                        rowKey='id'
                        loading={detailLoading}
                        className='usage-log-table'
                        pagination={{
                          currentPage: detailPage,
                          pageSize: detailPageSize,
                          total: detailTotal,
                          pageSizeOpts: [10, 20, 50],
                          showSizeChanger: true,
                          onPageSizeChange: handleDetailPageSizeChange,
                          onPageChange: handleDetailPageChange,
                        }}
                        empty={
                          <div className='usage-placeholder'>
                            {t('暂无调用明细')}
                          </div>
                        }
                      />
                    </div>
                  </Collapsible>
                </div>
              </Card>
            </div>
          </section>
        ) : null}

        {enabled && usageData?.mode === 'aggregate' ? (
          <>
            <section className='usage-section'>
              <div className='usage-section-header usage-section-header--compact'>
                <div>
                  <Text className='usage-section-eyebrow'>{t('聚合结果')}</Text>
                  <Title heading={5} className='usage-section-title'>
                    {t('聚合结果')}
                  </Title>
                </div>
                {renderPeriodTabs()}
              </div>

              <div className='usage-results-stack'>
                <Card className='usage-card usage-card--hero'>
                  <div className='usage-card-shell'>
                    <div className='usage-card-head'>
                      <div className='usage-card-title-wrap'>
                        <span className='usage-card-title'>
                          <Wallet size={16} />
                          {t('聚合额度概览')}
                        </span>
                        <Tag color='blue' size='small'>
                          {formatNumber(aggregateSummary.valid_key_count)} Key
                        </Tag>
                      </div>
                    </div>
                    <div className='usage-card-body usage-card-body--hero'>
                      <div className='usage-primary-stat'>
                        <div className='usage-primary-stat__main'>
                          <div className='usage-primary-stat__eyebrow'>
                            {invalidKeys.length
                              ? `${t('无效 Key')} ${formatNumber(
                                  invalidKeys.length,
                                )}`
                              : t('全部命中')}
                          </div>
                          <div className='usage-primary-stat__title'>
                            {t('总剩余额度')}
                          </div>
                        </div>
                        <div className='usage-primary-stat__value'>
                          <span>
                            {renderQuota(aggregateSummary.total_available || 0)}
                          </span>
                          <small>{t('已聚合 Key')}</small>
                        </div>
                      </div>

                      <div className='usage-overview-grid usage-overview-grid--compact'>
                        <OverviewCard
                          label={t('已聚合 Key')}
                          value={formatNumber(aggregateSummary.valid_key_count)}
                          sub={
                            invalidKeys.length
                              ? `${t('无效 Key')} ${formatNumber(
                                  invalidKeys.length,
                                )}`
                              : t('全部命中')
                          }
                        />
                        <OverviewCard
                          label={t('总额度')}
                          value={renderQuota(
                            aggregateSummary.total_granted || 0,
                          )}
                          sub={`${t('已使用')} ${renderQuota(
                            aggregateSummary.total_used || 0,
                          )}`}
                        />
                      </div>

                      <ProgressBlock
                        label={t('总额度使用进度')}
                        value={`${renderQuota(
                          aggregateSummary.total_used || 0,
                        )} / ${renderQuota(
                          aggregateSummary.total_granted || 0,
                        )}`}
                        percent={getPercent(
                          aggregateSummary.total_used,
                          aggregateSummary.total_granted,
                        )}
                        helper={`${t('已使用')} ${renderQuota(
                          aggregateSummary.total_used || 0,
                        )} · ${t('剩余')} ${renderQuota(
                          aggregateSummary.total_available || 0,
                        )}`}
                      />

                      <div className='usage-meta-grid usage-meta-grid--wide'>
                        <MetaItem
                          label={t('已聚合 Key')}
                          value={formatNumber(aggregateSummary.valid_key_count)}
                        />
                        <MetaItem
                          label={t('无效 Key')}
                          value={formatNumber(invalidKeys.length)}
                        />
                        <MetaItem
                          label={t('总额度')}
                          value={renderQuota(
                            aggregateSummary.total_granted || 0,
                          )}
                        />
                        <MetaItem
                          label={t('已使用')}
                          value={renderQuota(aggregateSummary.total_used || 0)}
                        />
                      </div>
                    </div>
                  </div>
                </Card>

                <div className='usage-kpi-grid'>
                  <OverviewCard
                    label={`${periodLabel}${t('请求')}`}
                    value={formatNumber(aggregateSummary.period_request_count)}
                    sub={`${formatNumber(aggregateDistribution.total)} Token`}
                  />
                  <OverviewCard
                    label={`${periodLabel}${t('消费')}`}
                    value={renderQuota(aggregateSummary.period_quota || 0)}
                    sub={
                      aggregateSummary.rpm > 0 || aggregateSummary.tpm > 0
                        ? `${aggregateSummary.rpm || 0} RPM / ${formatNumber(
                            aggregateSummary.tpm || 0,
                          )} TPM`
                        : t('统计返回后显示')
                    }
                  />
                </div>

                <div className='usage-distribution-grid'>
                  <Card className='usage-card'>
                    <div className='usage-card-shell'>
                      <div className='usage-card-head'>
                        <div className='usage-card-title-wrap'>
                          <span className='usage-card-title'>
                            <MessageSquareText size={16} />
                            {t('Token 使用分布')}
                          </span>
                          <span className='usage-card-title__aside'>
                            {formatNumber(aggregateDistribution.total)} Token
                          </span>
                        </div>
                        <Text className='usage-card-head__description'>
                          {t('总计包含非缓存输入、输出、缓存创建与缓存读取。')}
                        </Text>
                      </div>
                      <div className='usage-card-body'>
                        <ShareBreakdown
                          items={aggregateTokenItems}
                          emptyText={t('暂无统计数据')}
                        />
                      </div>
                    </div>
                  </Card>

                  <Card className='usage-card usage-card--list'>
                    <div className='usage-card-shell'>
                      <div className='usage-card-head'>
                        <div className='usage-card-title-wrap'>
                          <span className='usage-card-title'>
                            <Layers size={16} />
                            {t('模型使用统计')}
                          </span>
                          <span className='usage-card-title__aside'>
                            {renderQuota(aggregateSummary.period_quota || 0)}
                          </span>
                        </div>
                      </div>
                      <div className='usage-card-body usage-card-body--list'>
                        {statsLoading ? (
                          <div className='usage-placeholder'>{t('加载中')}</div>
                        ) : modelShareItems.length ? (
                          <div className='usage-model-board'>
                            <div className='usage-model-board__summary'>
                              <div className='usage-model-board__item'>
                                <span>{`${periodLabel}${t('消费')}`}</span>
                                <strong>
                                  {renderQuota(
                                    aggregateSummary.period_quota || 0,
                                  )}
                                </strong>
                              </div>
                              <div className='usage-model-board__item'>
                                <span>{`${periodLabel} Token`}</span>
                                <strong>
                                  {formatNumber(aggregateDistribution.total)}
                                </strong>
                              </div>
                            </div>
                            <ShareBreakdown
                              items={modelShareItems}
                              emptyText={t('暂无统计数据')}
                            />
                          </div>
                        ) : (
                          <div className='usage-placeholder'>
                            {t('暂无统计数据')}
                          </div>
                        )}
                      </div>
                    </div>
                  </Card>
                </div>
              </div>
            </section>

            <section className='usage-section'>
              <div className='usage-section-header'>
                <Text className='usage-section-eyebrow'>
                  {t('每 Key 明细')}
                </Text>
              </div>
              <div className='usage-key-list'>
                {aggregateDetails.length ? (
                  aggregateDetails.map((item) => {
                    const itemStatus = TOKEN_STATUS_MAP[item.status] || {
                      text: '未知',
                      color: 'grey',
                    };
                    const itemDistribution = getDistribution(item);
                    return (
                      <Card className='usage-card' key={getIdentity(item)}>
                        <div className='usage-card-shell'>
                          <div className='usage-card-head'>
                            <div className='usage-card-title-wrap'>
                              <span className='usage-card-title'>
                                <KeyRound size={16} />
                                {item.name ||
                                  item.masked_key ||
                                  item.raw_key ||
                                  '-'}
                              </span>
                              <Tag color={itemStatus.color} size='small'>
                                {t(itemStatus.text)}
                              </Tag>
                            </div>
                          </div>
                          <div className='usage-card-body'>
                            <div className='usage-key-detail__meta'>
                              <span>{item.group || t('默认分组')}</span>
                              <span>{`${periodLabel}${t('请求')} ${formatNumber(item.period_request_count)}`}</span>
                              <span>{`${periodLabel}${t('消费')} ${renderQuota(item.period_quota || 0)}`}</span>
                              <span>{`${formatNumber(itemDistribution.total)} Token`}</span>
                              <span>{getRuntimeLimitText(item, t)}</span>
                            </div>

                            {!item.unlimited_quota ? (
                              <ProgressBlock
                                label={t('额度概览')}
                                value={`${renderQuota(item.total_used || 0)} / ${renderQuota(item.total_granted || 0)}`}
                                percent={getPercent(
                                  item.total_used,
                                  item.total_granted,
                                )}
                                helper={`${t('剩余')} ${renderQuota(item.total_available || 0)}${item.package_enabled ? ` · ${t('套餐用量')} ${renderQuota(item.package_used_quota || 0)} / ${renderQuota(item.package_limit_quota || 0)}` : ''}`}
                              />
                            ) : (
                              <div className='usage-inline-placeholder'>
                                {t('该 Key 不受总额度限制')}
                              </div>
                            )}
                          </div>
                        </div>
                      </Card>
                    );
                  })
                ) : (
                  <div className='usage-placeholder'>{t('暂无统计数据')}</div>
                )}
              </div>
            </section>
          </>
        ) : null}

        <footer className='usage-footer'>
          <Text type='tertiary' size='small'>
            {t('由 {{systemName}} 提供支持', { systemName })}
          </Text>
        </footer>
      </div>
    </div>
  );
}
