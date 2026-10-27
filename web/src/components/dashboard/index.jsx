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

import React, { useContext, useEffect, useRef } from 'react';
import { getRelativeTime } from '../../helpers/date';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

import DashboardHeader from './DashboardHeader';
import './dashboardOverview.css';
import StatsCards from './StatsCards';
import ChartsPanel from './ChartsPanel';
import ApiInfoPanel from './ApiInfoPanel';
import AnnouncementsPanel from './AnnouncementsPanel';
import FaqPanel from './FaqPanel';
import UptimePanel from './UptimePanel';
import ChannelMonitorPanel from './ChannelMonitorPanel';
import SearchModal from './modals/SearchModal';

import { useDashboardData } from '../../hooks/dashboard/useDashboardData';
import { useDashboardStats } from '../../hooks/dashboard/useDashboardStats';
import { useDashboardCharts } from '../../hooks/dashboard/useDashboardCharts';

import {
  CHART_CONFIG,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  ANNOUNCEMENT_LEGEND_DATA,
  UPTIME_STATUS_MAP,
} from '../../constants/dashboard.constants';
import {
  handleCopyUrl,
  handleSpeedTest,
  getUptimeStatusColor,
  getUptimeStatusText,
  renderMonitorList,
} from '../../helpers/dashboard';

const DashboardContent = () => {
  // ========== Context ==========
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const userChartLoadedRef = useRef(false);
  const perfMetricsLoadedRef = useRef(false);
  const modelChannelStatsLoadedRef = useRef(false);
  const userBalanceTrendLoadedRef = useRef(false);

  // ========== 主要数据管理 ==========
  const dashboardData = useDashboardData(userState, userDispatch, statusState);

  // ========== 图表管理 ==========
  const dashboardCharts = useDashboardCharts(
    dashboardData.dataExportDefaultTime,
    dashboardData.setTrendData,
    dashboardData.setConsumeQuota,
    dashboardData.setTimes,
    dashboardData.setConsumeTokens,
    dashboardData.setModelColors,
    dashboardData.t,
  );

  // ========== 统计数据 ==========
  const { groupedStatsData } = useDashboardStats(
    userState,
    dashboardData.consumeQuota,
    dashboardData.consumeTokens,
    dashboardData.times,
    dashboardData.performanceMetrics,
    dashboardData.t,
  );

  // ========== 数据处理 ==========
  const loadUserData = async () => {
    if (dashboardData.isAdminUser) {
      const userData = await dashboardData.loadUserQuotaData();
      if (userData && userData.length > 0) {
        dashboardCharts.updateUserChartData(userData);
      }
      userChartLoadedRef.current = true;
    }
  };

  const loadModelChannelTagStats = async () => {
    if (dashboardData.isAdminUser) {
      await dashboardData.loadModelChannelTagStats();
      modelChannelStatsLoadedRef.current = true;
    }
  };

  const initChart = async () => {
    const quotaTask = dashboardData.loadQuotaData().then((data) => {
      if (data && data.length > 0) {
        dashboardCharts.updateChartData(data);
      }
    });

    const optionalTasks = [quotaTask, dashboardData.getUserData()];
    if (dashboardData.uptimeEnabled) {
      optionalTasks.push(dashboardData.loadUptimeData());
    }
    await Promise.allSettled(optionalTasks);
  };

  const handleRefresh = async () => {
    if (dashboardData.rangePreset !== 'custom') {
      dashboardData.selectRangePreset(dashboardData.rangePreset);
      return;
    }
    const isUserChartTab = dashboardData.activeChartTab === '5';
    const isModelChannelStatsTab = dashboardData.activeChartTab === '6';
    const isUserBalanceTrendTab = dashboardData.activeChartTab === '8';
    const data = await dashboardData.refresh({
      includePerfMetrics: dashboardData.activeChartTab === '7',
      includeModelChannelStats: isModelChannelStatsTab,
      includeUserBalanceTrend: isUserBalanceTrendTab,
    });
    if (!data) return;
    if (data.length > 0) {
      dashboardCharts.updateChartData(data);
    }
    if (isUserChartTab) {
      await loadUserData();
    }
  };

  const handleSearchConfirm = (draft, grain) =>
    dashboardData.applyQuery(draft, grain);

  // ========== 数据准备 ==========
  const apiInfoData = statusState?.status?.api_info || [];
  const announcementData = (statusState?.status?.announcements || []).map(
    (item) => {
      const pubDate = item?.publishDate ? new Date(item.publishDate) : null;
      const absoluteTime =
        pubDate && !isNaN(pubDate.getTime())
          ? `${pubDate.getFullYear()}-${String(pubDate.getMonth() + 1).padStart(2, '0')}-${String(pubDate.getDate()).padStart(2, '0')} ${String(pubDate.getHours()).padStart(2, '0')}:${String(pubDate.getMinutes()).padStart(2, '0')}`
          : item?.publishDate || '';
      const relativeTime = getRelativeTime(item.publishDate);
      return {
        ...item,
        time: absoluteTime,
        relative: relativeTime,
      };
    },
  );
  const faqData = statusState?.status?.faq || [];

  const uptimeLegendData = Object.entries(UPTIME_STATUS_MAP).map(
    ([status, info]) => ({
      status: Number(status),
      color: info.color,
      label: dashboardData.t(info.label),
    }),
  );

  // ========== Effects ==========
  useEffect(() => {
    userChartLoadedRef.current = false;
    perfMetricsLoadedRef.current = false;
    modelChannelStatsLoadedRef.current = false;
    userBalanceTrendLoadedRef.current = false;
    initChart();
  }, [dashboardData.queryVersion]);

  useEffect(() => {
    if (
      dashboardData.isAdminUser &&
      dashboardData.activeChartTab === '5' &&
      !userChartLoadedRef.current
    ) {
      loadUserData();
    }
  }, [
    dashboardData.activeChartTab,
    dashboardData.isAdminUser,
    dashboardData.queryVersion,
  ]);

  useEffect(() => {
    if (
      dashboardData.isAdminUser &&
      dashboardData.activeChartTab === '6' &&
      !modelChannelStatsLoadedRef.current
    ) {
      loadModelChannelTagStats();
    }
  }, [
    dashboardData.activeChartTab,
    dashboardData.isAdminUser,
    dashboardData.queryVersion,
  ]);

  useEffect(() => {
    if (dashboardData.activeChartTab === '7' && !perfMetricsLoadedRef.current) {
      perfMetricsLoadedRef.current = true;
      dashboardData.loadPerfMetricsSummary();
    }
  }, [
    dashboardData.activeChartTab,
    dashboardData.loadPerfMetricsSummary,
    dashboardData.queryVersion,
  ]);

  useEffect(() => {
    if (
      dashboardData.isAdminUser &&
      dashboardData.activeChartTab === '8' &&
      !userBalanceTrendLoadedRef.current
    ) {
      userBalanceTrendLoadedRef.current = true;
      dashboardData.loadUserBalanceTrend();
    }
  }, [
    dashboardData.activeChartTab,
    dashboardData.isAdminUser,
    dashboardData.queryVersion,
  ]);

  return (
    <div className='dashboard-overview'>
      <DashboardHeader
        getGreeting={dashboardData.getGreeting}
        rangePreset={dashboardData.rangePreset}
        onRangeChange={dashboardData.selectRangePreset}
        inputs={dashboardData.inputs}
        dataExportDefaultTime={dashboardData.dataExportDefaultTime}
        isAdminUser={dashboardData.isAdminUser}
        showSearchModal={dashboardData.showSearchModal}
        refresh={handleRefresh}
        loading={dashboardData.loading}
        t={dashboardData.t}
      />

      <SearchModal
        searchModalVisible={dashboardData.searchModalVisible}
        handleSearchConfirm={handleSearchConfirm}
        handleCloseModal={dashboardData.handleCloseModal}
        isMobile={dashboardData.isMobile}
        isAdminUser={dashboardData.isAdminUser}
        inputs={dashboardData.inputs}
        dataExportDefaultTime={dashboardData.dataExportDefaultTime}
        timeOptions={dashboardData.timeOptions}
        t={dashboardData.t}
      />

      {(dashboardData.quotaError || dashboardData.accountError) && (
        <div className='dashboard-load-error' role='alert'>
          <span>
            {dashboardData.t('加载失败，请重试')} ·{' '}
            {dashboardData.t(
              dashboardData.quotaError ? '使用统计' : '账户数据',
            )}
          </span>
          <button type='button' onClick={handleRefresh}>
            {dashboardData.t('重试')}
          </button>
        </div>
      )}
      <StatsCards
        groupedStatsData={groupedStatsData}
        loading={dashboardData.loading}
        quotaError={dashboardData.quotaError}
        accountLoading={dashboardData.accountLoading}
        accountError={dashboardData.accountError}
      />

      {/* API信息和图表面板 */}
      <div className='mb-4'>
        <div
          className={`grid grid-cols-1 gap-4 ${dashboardData.hasApiInfoPanel ? 'lg:grid-cols-4' : ''}`}
        >
          <ChartsPanel
            quotaLoading={dashboardData.loading}
            quotaError={dashboardData.quotaError}
            activeChartTab={dashboardData.activeChartTab}
            setActiveChartTab={dashboardData.setActiveChartTab}
            spec_line={dashboardCharts.spec_line}
            spec_rank_bar={dashboardCharts.spec_rank_bar}
            spec_user_rank={dashboardCharts.spec_user_rank}
            perfMetricsSummary={dashboardData.perfMetricsSummary}
            perfMetricsLoading={dashboardData.perfMetricsLoading}
            modelChannelStats={dashboardData.modelChannelStats}
            modelChannelStatsLoading={dashboardData.modelChannelStatsLoading}
            modelChannelStatsDays={dashboardData.modelChannelStatsDays}
            onModelChannelStatsDaysChange={
              dashboardData.handleModelChannelStatsDaysChange
            }
            userBalanceTrend={dashboardData.userBalanceTrend}
            userBalanceTrendLoading={dashboardData.userBalanceTrendLoading}
            userBalanceTrendDays={dashboardData.userBalanceTrendDays}
            onUserBalanceTrendDaysChange={
              dashboardData.handleUserBalanceTrendDaysChange
            }
            onUserBalanceTrendUsersChanged={dashboardData.loadUserBalanceTrend}
            isAdminUser={dashboardData.isAdminUser}
            CARD_PROPS={CARD_PROPS}
            CHART_CONFIG={CHART_CONFIG}
            FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
            hasApiInfoPanel={dashboardData.hasApiInfoPanel}
            t={dashboardData.t}
          />

          {dashboardData.hasApiInfoPanel && (
            <ApiInfoPanel
              apiInfoData={apiInfoData}
              handleCopyUrl={(url) => handleCopyUrl(url, dashboardData.t)}
              handleSpeedTest={handleSpeedTest}
              CARD_PROPS={CARD_PROPS}
              FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
              ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
              t={dashboardData.t}
            />
          )}
        </div>
      </div>

      {/* 系统公告和常见问答卡片 */}
      {dashboardData.hasInfoPanels && (
        <div className='mb-4'>
          <div className='grid grid-cols-1 lg:grid-cols-4 gap-4'>
            {/* 公告卡片 */}
            {dashboardData.announcementsEnabled && (
              <AnnouncementsPanel
                announcementData={announcementData}
                announcementLegendData={ANNOUNCEMENT_LEGEND_DATA.map(
                  (item) => ({
                    ...item,
                    label: dashboardData.t(item.label),
                  }),
                )}
                CARD_PROPS={CARD_PROPS}
                ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
                t={dashboardData.t}
              />
            )}

            {/* 常见问答卡片 */}
            {dashboardData.faqEnabled && (
              <FaqPanel
                faqData={faqData}
                CARD_PROPS={CARD_PROPS}
                FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
                ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
                t={dashboardData.t}
              />
            )}

            {/* 服务可用性卡片 */}
            {dashboardData.uptimeEnabled && (
              <UptimePanel
                uptimeData={dashboardData.uptimeData}
                uptimeLoading={dashboardData.uptimeLoading}
                activeUptimeTab={dashboardData.activeUptimeTab}
                setActiveUptimeTab={dashboardData.setActiveUptimeTab}
                loadUptimeData={dashboardData.loadUptimeData}
                uptimeLegendData={uptimeLegendData}
                renderMonitorList={(monitors) =>
                  renderMonitorList(
                    monitors,
                    (status) => getUptimeStatusColor(status, UPTIME_STATUS_MAP),
                    (status) =>
                      getUptimeStatusText(
                        status,
                        UPTIME_STATUS_MAP,
                        dashboardData.t,
                      ),
                    dashboardData.t,
                  )
                }
                CARD_PROPS={CARD_PROPS}
                ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
                t={dashboardData.t}
              />
            )}
          </div>
        </div>
      )}

      {/* 渠道监控面板 */}
      {dashboardData.isAdminUser && (
        <div className='mb-4'>
          <ChannelMonitorPanel
            CARD_PROPS={CARD_PROPS}
            ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
          />
        </div>
      )}
    </div>
  );
};

// Remount account-owned data on identity/role changes before exposing any statistics.
const Dashboard = () => {
  const [userState] = useContext(UserContext);
  const user = userState?.user;
  if (!user?.id) return null;
  return (
    <DashboardContent key={`${user.id}:${user.role}:${user.group || ''}`} />
  );
};
export default Dashboard;
