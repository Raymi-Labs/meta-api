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

import { useState, useEffect, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers/apiCore';
import { setUserData } from '../../helpers/data';
import { showError } from '../../helpers/toast';
import { timestamp2string } from '../../helpers/date';
import { getDefaultTime, getInitialTimestamp } from '../../helpers/dashboard';
import {
  dashboardPresetRange,
  isDashboardAdmin,
  dashboardQuotaRows,
  dashboardRates,
  validateDashboardQuery,
} from '../../helpers/dashboardQuery';
import { TIME_OPTIONS } from '../../constants/dashboard.constants';
import { useIsMobile } from '../common/useIsMobile';
import { useLatestRequestGuard } from '../common/useLatestRequestGuard';
import { useMinimumLoadingTime } from '../common/useMinimumLoadingTime';

export const useDashboardData = (userState, userDispatch, statusState) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const quotaRequestGuard = useLatestRequestGuard();
  const refreshRequestGuard = useLatestRequestGuard();
  const uptimeRequestGuard = useLatestRequestGuard();
  const userQuotaRequestGuard = useLatestRequestGuard();
  const perfMetricsRequestGuard = useLatestRequestGuard();
  const modelChannelStatsRequestGuard = useLatestRequestGuard();
  const userBalanceTrendRequestGuard = useLatestRequestGuard();

  // ========== 基础状态 ==========
  const [loading, setLoading] = useState(true);
  const [quotaError, setQuotaError] = useState(false);
  const [accountLoading, setAccountLoading] = useState(true);
  const [accountError, setAccountError] = useState(false);
  const accountRequestGuard = useLatestRequestGuard();
  const [queryVersion, setQueryVersion] = useState(0);
  const [rangePreset, setRangePreset] = useState('custom');
  const [searchModalVisible, setSearchModalVisible] = useState(false);
  const showLoading = useMinimumLoadingTime(loading);

  // ========== 输入状态 ==========
  const [inputs, setInputs] = useState({
    username: '',
    token_name: '',
    model_name: '',
    start_timestamp: getInitialTimestamp(),
    end_timestamp: timestamp2string(new Date().getTime() / 1000),
    channel: '',
    data_export_default_time: '',
  });

  const [dataExportDefaultTime, setDataExportDefaultTime] =
    useState(getDefaultTime());

  // ========== 数据状态 ==========
  const [quotaData, setQuotaData] = useState([]);
  const [consumeQuota, setConsumeQuota] = useState(0);
  const [consumeTokens, setConsumeTokens] = useState(0);
  const [times, setTimes] = useState(0);
  const [modelColors, setModelColors] = useState({});
  const [perfMetricsSummary, setPerfMetricsSummary] = useState([]);
  const [perfMetricsLoading, setPerfMetricsLoading] = useState(false);
  const [modelChannelStats, setModelChannelStats] = useState(null);
  const [modelChannelStatsLoading, setModelChannelStatsLoading] =
    useState(false);
  const [modelChannelStatsDays, setModelChannelStatsDays] = useState(1);
  const [userBalanceTrend, setUserBalanceTrend] = useState(null);
  const [userBalanceTrendLoading, setUserBalanceTrendLoading] = useState(false);
  const [userBalanceTrendDays, setUserBalanceTrendDays] = useState(7);

  // ========== 图表状态 ==========
  const [activeChartTab, setActiveChartTab] = useState('1');

  // ========== 趋势数据 ==========
  const [trendData, setTrendData] = useState({
    balance: [],
    usedQuota: [],
    requestCount: [],
    times: [],
    consumeQuota: [],
    tokens: [],
    rpm: [],
    tpm: [],
  });

  // ========== Uptime 数据 ==========
  const [uptimeData, setUptimeData] = useState([]);
  const [uptimeLoading, setUptimeLoading] = useState(false);
  const [activeUptimeTab, setActiveUptimeTab] = useState('');

  // ========== 常量 ==========
  const isAdminUser = isDashboardAdmin(userState?.user);

  // ========== Panel enable flags ==========
  const apiInfoEnabled = statusState?.status?.api_info_enabled ?? true;
  const announcementsEnabled =
    statusState?.status?.announcements_enabled ?? true;
  const faqEnabled = statusState?.status?.faq_enabled ?? true;
  const uptimeEnabled = statusState?.status?.uptime_kuma_enabled ?? true;

  const hasApiInfoPanel = apiInfoEnabled;
  const hasInfoPanels = announcementsEnabled || faqEnabled || uptimeEnabled;

  // ========== Memoized Values ==========
  const timeOptions = useMemo(
    () =>
      TIME_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  const performanceMetrics = useMemo(
    () => dashboardRates(times, consumeTokens, inputs),
    [times, consumeTokens, inputs.start_timestamp, inputs.end_timestamp],
  );

  const getGreeting = useMemo(() => {
    const hours = new Date().getHours();
    let greeting = '';

    if (hours >= 5 && hours < 12) {
      greeting = t('早上好');
    } else if (hours >= 12 && hours < 14) {
      greeting = t('中午好');
    } else if (hours >= 14 && hours < 18) {
      greeting = t('下午好');
    } else {
      greeting = t('晚上好');
    }

    const username = userState?.user?.username || '';
    return `👋${greeting}，${username}`;
  }, [t, userState?.user?.username]);

  // ========== 回调函数 ==========
  const applyQuery = useCallback(
    (draft, grain, preset = 'custom') => {
      const error = validateDashboardQuery(draft, grain, isAdminUser);
      if (error) {
        showError(
          t(
            error === 'range-too-long'
              ? '请选择不超过30天的时间范围'
              : '请选择有效的起止时间',
          ),
        );
        return false;
      }
      quotaRequestGuard.createRequestId();
      refreshRequestGuard.createRequestId();
      setInputs({
        ...draft,
        username: isAdminUser ? draft.username || '' : '',
      });
      setDataExportDefaultTime(grain);
      setRangePreset(preset);
      try {
        localStorage.setItem('data_export_default_time', grain);
      } catch {
        /* Current query remains usable without storage. */
      }
      setLoading(true);
      setQuotaError(false);
      setQueryVersion((version) => version + 1);
      setSearchModalVisible(false);
      return true;
    },
    [isAdminUser, t, quotaRequestGuard, refreshRequestGuard],
  );

  const showSearchModal = useCallback(() => {
    setSearchModalVisible(true);
  }, []);

  const selectRangePreset = useCallback(
    (preset) => {
      if (preset === 'custom') {
        showSearchModal();
        return;
      }
      const range = dashboardPresetRange(preset);
      if (!range) {
        showError(t('请选择有效的起止时间'));
        return;
      }
      applyQuery(
        {
          ...inputs,
          start_timestamp: range.start_timestamp,
          end_timestamp: range.end_timestamp,
        },
        range.grain,
        preset,
      );
    },
    [inputs, applyQuery, showSearchModal, t],
  );

  const handleCloseModal = useCallback(() => {
    setSearchModalVisible(false);
  }, []);

  // ========== API 调用函数 ==========
  const loadQuotaData = useCallback(async () => {
    const requestId = quotaRequestGuard.createRequestId();
    setLoading(true);
    setQuotaError(false);
    try {
      let url = '';
      const { start_timestamp, end_timestamp, username } = inputs;
      let localStartTimestamp = Date.parse(start_timestamp) / 1000;
      let localEndTimestamp = Date.parse(end_timestamp) / 1000;

      if (isAdminUser) {
        url = `/api/data/?username=${encodeURIComponent(username)}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${dataExportDefaultTime}`;
      } else {
        url = `/api/data/self/?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${dataExportDefaultTime}`;
      }

      const res = await API.get(url);
      if (!quotaRequestGuard.isLatestRequest(requestId)) {
        return null;
      }
      const { success, message, data } = res.data;
      if (success) {
        const rows = dashboardQuotaRows(data);
        setQuotaData(rows);
        return rows;
      } else {
        setQuotaError(true);
        return null;
      }
    } catch {
      if (quotaRequestGuard.isLatestRequest(requestId)) setQuotaError(true);
      return null;
    } finally {
      if (quotaRequestGuard.isLatestRequest(requestId)) {
        setLoading(false);
      }
    }
  }, [dataExportDefaultTime, inputs, isAdminUser, quotaRequestGuard]);

  const loadUptimeData = useCallback(async () => {
    const requestId = uptimeRequestGuard.createRequestId();
    setUptimeLoading(true);
    try {
      const res = await API.get('/api/uptime/status');
      if (!uptimeRequestGuard.isLatestRequest(requestId)) {
        return;
      }
      const { success, message, data } = res.data;
      if (success) {
        setUptimeData(data || []);
        if (data && data.length > 0 && !activeUptimeTab) {
          setActiveUptimeTab(data[0].categoryName);
        }
      } else {
        showError(message);
      }
    } catch (err) {
      console.error(err);
    } finally {
      if (uptimeRequestGuard.isLatestRequest(requestId)) {
        setUptimeLoading(false);
      }
    }
  }, [activeUptimeTab, uptimeRequestGuard]);

  const loadUserQuotaData = useCallback(async () => {
    if (!isAdminUser) return [];
    const requestId = userQuotaRequestGuard.createRequestId();
    try {
      const { start_timestamp, end_timestamp } = inputs;
      const localStartTimestamp = Date.parse(start_timestamp) / 1000;
      const localEndTimestamp = Date.parse(end_timestamp) / 1000;
      const url = `/api/data/users?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;
      const res = await API.get(url);
      if (!userQuotaRequestGuard.isLatestRequest(requestId)) {
        return [];
      }
      const { success, message, data } = res.data;
      if (success) {
        return data || [];
      }
      showError(message);
      return [];
    } catch (err) {
      console.error(err);
      return [];
    }
  }, [inputs, isAdminUser, userQuotaRequestGuard]);

  const loadPerfMetricsSummary = useCallback(async () => {
    const requestId = perfMetricsRequestGuard.createRequestId();
    setPerfMetricsLoading(true);
    try {
      const start = Date.parse(inputs.start_timestamp) / 1000;
      const end = Date.parse(inputs.end_timestamp) / 1000;
      const hours =
        Number.isFinite(start) && Number.isFinite(end) && end > start
          ? Math.max(1, Math.ceil((end - start) / 3600))
          : 24;
      const res = await API.get(
        `/api/perf-metrics/summary?hours=${Math.min(hours, 24 * 30)}`,
      );
      if (!perfMetricsRequestGuard.isLatestRequest(requestId)) {
        return [];
      }
      const { success, message, data } = res.data;
      if (success) {
        const models = data?.models || [];
        setPerfMetricsSummary(models);
        return models;
      }
      showError(message);
      return [];
    } catch (err) {
      console.error(err);
      return [];
    } finally {
      if (perfMetricsRequestGuard.isLatestRequest(requestId)) {
        setPerfMetricsLoading(false);
      }
    }
  }, [inputs.start_timestamp, inputs.end_timestamp, perfMetricsRequestGuard]);

  const loadModelChannelTagStats = useCallback(
    async (daysOverride) => {
      if (!isAdminUser) return null;
      const requestId = modelChannelStatsRequestGuard.createRequestId();
      setModelChannelStatsLoading(true);
      try {
        const days = Number(daysOverride || modelChannelStatsDays || 1);
        const end = Math.floor(Date.now() / 1000);
        const start = end - Math.min(Math.max(days, 1), 30) * 86400;
        const res = await API.get('/api/data/model-channel-tags', {
          params: {
            start_timestamp: start,
            end_timestamp: end,
            username: inputs.username,
            limit: 12,
          },
        });
        if (!modelChannelStatsRequestGuard.isLatestRequest(requestId)) {
          return null;
        }
        const { success, message, data } = res.data;
        if (success) {
          setModelChannelStats(data || null);
          return data || null;
        }
        showError(message);
        return null;
      } catch (err) {
        console.error(err);
        return null;
      } finally {
        if (modelChannelStatsRequestGuard.isLatestRequest(requestId)) {
          setModelChannelStatsLoading(false);
        }
      }
    },
    [
      inputs.username,
      isAdminUser,
      modelChannelStatsDays,
      modelChannelStatsRequestGuard,
    ],
  );

  const handleModelChannelStatsDaysChange = useCallback(
    async (days) => {
      setModelChannelStatsDays(days);
      return loadModelChannelTagStats(days);
    },
    [loadModelChannelTagStats],
  );

  const loadUserBalanceTrend = useCallback(
    async (daysOverride) => {
      if (!isAdminUser) return null;
      const requestId = userBalanceTrendRequestGuard.createRequestId();
      setUserBalanceTrendLoading(true);
      try {
        const days = Number(daysOverride || userBalanceTrendDays || 7);
        const res = await API.get('/api/data/user-balance-trend', {
          params: {
            days,
          },
        });
        if (!userBalanceTrendRequestGuard.isLatestRequest(requestId)) {
          return null;
        }
        const { success, message, data } = res.data;
        if (success) {
          setUserBalanceTrend(data || null);
          return data || null;
        }
        showError(message);
        return null;
      } catch (err) {
        console.error(err);
        return null;
      } finally {
        if (userBalanceTrendRequestGuard.isLatestRequest(requestId)) {
          setUserBalanceTrendLoading(false);
        }
      }
    },
    [isAdminUser, userBalanceTrendDays, userBalanceTrendRequestGuard],
  );

  const handleUserBalanceTrendDaysChange = useCallback(
    async (days) => {
      setUserBalanceTrendDays(days);
      return loadUserBalanceTrend(days);
    },
    [loadUserBalanceTrend],
  );

  const getUserData = useCallback(async () => {
    const requestId = accountRequestGuard.createRequestId();
    setAccountLoading(true);
    setAccountError(false);
    try {
      const { data: response } = await API.get('/api/user/self');
      if (!accountRequestGuard.isLatestRequest(requestId)) return;
      if (!response.success || !response.data)
        throw new Error('Invalid account response');
      userDispatch({ type: 'login', payload: response.data });
      setUserData(response.data);
    } catch {
      if (accountRequestGuard.isLatestRequest(requestId)) setAccountError(true);
    } finally {
      if (accountRequestGuard.isLatestRequest(requestId))
        setAccountLoading(false);
    }
  }, [userDispatch, accountRequestGuard]);

  useEffect(
    () => () => {
      quotaRequestGuard.createRequestId();
      refreshRequestGuard.createRequestId();
      accountRequestGuard.createRequestId();
      uptimeRequestGuard.createRequestId();
      userQuotaRequestGuard.createRequestId();
      perfMetricsRequestGuard.createRequestId();
      modelChannelStatsRequestGuard.createRequestId();
      userBalanceTrendRequestGuard.createRequestId();
    },
    [
      quotaRequestGuard,
      refreshRequestGuard,
      accountRequestGuard,
      uptimeRequestGuard,
      userQuotaRequestGuard,
      perfMetricsRequestGuard,
      modelChannelStatsRequestGuard,
      userBalanceTrendRequestGuard,
    ],
  );

  const refresh = useCallback(
    async (options = {}) => {
      const requestId = refreshRequestGuard.createRequestId();
      const {
        includeUptime = uptimeEnabled,
        includePerfMetrics = false,
        includeUserBalanceTrend = false,
      } = options;
      const data = await loadQuotaData();
      if (!refreshRequestGuard.isLatestRequest(requestId)) return null;
      const tasks = [getUserData()];
      if (includeUptime) {
        tasks.push(loadUptimeData());
      }
      if (includePerfMetrics) {
        tasks.push(loadPerfMetricsSummary());
      }
      if (options.includeModelChannelStats) {
        tasks.push(loadModelChannelTagStats());
      }
      if (includeUserBalanceTrend) {
        tasks.push(loadUserBalanceTrend());
      }
      if (tasks.length > 0) {
        await Promise.allSettled(tasks);
      }
      return refreshRequestGuard.isLatestRequest(requestId) ? data : null;
    },
    [
      refreshRequestGuard,
      loadQuotaData,
      getUserData,
      loadUptimeData,
      loadPerfMetricsSummary,
      loadModelChannelTagStats,
      loadUserBalanceTrend,
      uptimeEnabled,
      isAdminUser,
    ],
  );

  return {
    // 基础状态
    loading: showLoading,
    quotaError,
    accountLoading,
    accountError,
    queryVersion,
    rangePreset,
    selectRangePreset,
    applyQuery,
    searchModalVisible,

    // 输入状态
    inputs,
    dataExportDefaultTime,

    // 数据状态
    quotaData,
    consumeQuota,
    setConsumeQuota,
    consumeTokens,
    setConsumeTokens,
    times,
    setTimes,
    modelColors,
    setModelColors,
    perfMetricsSummary,
    perfMetricsLoading,
    modelChannelStats,
    modelChannelStatsLoading,
    modelChannelStatsDays,
    userBalanceTrend,
    userBalanceTrendLoading,
    userBalanceTrendDays,

    // 图表状态
    activeChartTab,
    setActiveChartTab,

    // 趋势数据
    trendData,
    setTrendData,

    // Uptime 数据
    uptimeData,
    uptimeLoading,
    activeUptimeTab,
    setActiveUptimeTab,

    // 计算值
    timeOptions,
    performanceMetrics,
    getGreeting,
    isAdminUser,
    hasApiInfoPanel,
    hasInfoPanels,
    apiInfoEnabled,
    announcementsEnabled,
    faqEnabled,
    uptimeEnabled,

    // 函数
    showSearchModal,
    handleCloseModal,
    loadQuotaData,
    loadUserQuotaData,
    loadUptimeData,
    loadPerfMetricsSummary,
    loadModelChannelTagStats,
    handleModelChannelStatsDaysChange,
    loadUserBalanceTrend,
    handleUserBalanceTrendDaysChange,
    getUserData,
    refresh,

    // 导航和翻译
    navigate,
    t,
    isMobile,
  };
};
