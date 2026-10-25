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

import { useState, useCallback, useEffect, useRef } from 'react';
import {
  modelColorMap,
  renderNumber,
  renderQuota,
  modelToColor,
  getQuotaWithUnit,
} from '../../helpers/dashboardFormat';
import {
  processRawData,
  calculateTrendData,
  aggregateDataByTimeAndModel,
  generateChartTimePoints,
  updateChartSpec,
  updateMapValue,
  initializeMaps,
} from '../../helpers/dashboard';

const USER_COLORS = [
  '#3b82f6',
  '#ef4444',
  '#10b981',
  '#f59e0b',
  '#8b5cf6',
  '#ec4899',
  '#06b6d4',
  '#f97316',
  '#6366f1',
  '#14b8a6',
];

export const useDashboardCharts = (
  dataExportDefaultTime,
  setTrendData,
  setConsumeQuota,
  setTimes,
  setConsumeTokens,
  setModelColors,
  t,
) => {
  const lastChartData = useRef(null);
  const lastUserData = useRef(null);
  // ========== 图表规格状态 ==========
  const [spec_line, setSpecLine] = useState({
    type: 'bar',
    data: [
      {
        id: 'barData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Usage',
    seriesField: 'Model',
    stack: true,
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型消耗分布'),
      subtext: `${t('总计')}：${renderQuota(0, 2)}`,
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => datum['rawQuota'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            if (array[i].key == '其他') {
              continue;
            }
            let value = parseFloat(array[i].value);
            if (isNaN(value)) {
              value = 0;
            }
            if (array[i].datum && array[i].datum.TimeSum) {
              sum = array[i].datum.TimeSum;
            }
            array[i].value = renderQuota(value, 4);
          }
          array.unshift({
            key: t('总计'),
            value: renderQuota(sum, 4),
          });
          return array;
        },
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_rank_bar, setSpecRankBar] = useState({
    type: 'bar',
    data: [
      {
        id: 'rankData',
        values: [],
      },
    ],
    xField: 'Model',
    yField: 'Count',
    seriesField: 'Model',
    legends: {
      visible: true,
      selectMode: 'single',
    },
    title: {
      visible: true,
      text: t('模型调用次数排行'),
      subtext: '',
    },
    bar: {
      state: {
        hover: {
          stroke: '#000',
          lineWidth: 1,
        },
      },
    },
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    },
    color: {
      specified: modelColorMap,
    },
  });

  const [spec_user_rank, setSpecUserRank] = useState({
    type: 'bar',
    data: [{ id: 'userRankData', values: [] }],
    xField: 'rawQuota',
    yField: 'User',
    seriesField: 'User',
    direction: 'horizontal',
    legends: { visible: false },
    title: {
      visible: true,
      text: t('用户消耗排行'),
      subtext: '',
    },
    bar: {
      state: { hover: { stroke: '#000', lineWidth: 1 } },
    },
    label: {
      visible: true,
      position: 'outside',
      formatMethod: (value, datum) => renderQuota(datum['rawQuota'] || 0, 2),
    },
    axes: [
      {
        orient: 'left',
        type: 'band',
        label: { visible: true },
      },
      {
        orient: 'bottom',
        type: 'linear',
        visible: false,
      },
    ],
    tooltip: {
      mark: {
        content: [
          {
            key: (datum) => datum['User'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
    },
    color: { type: 'ordinal', range: USER_COLORS },
  });

  // ========== 数据处理函数 ==========
  const generateModelColors = useCallback((uniqueModels, modelColors) => {
    const newModelColors = {};
    Array.from(uniqueModels).forEach((modelName) => {
      newModelColors[modelName] =
        modelColorMap[modelName] ||
        modelColors[modelName] ||
        modelToColor(modelName);
    });
    return newModelColors;
  }, []);

  const updateChartData = useCallback(
    (source) => {
      lastChartData.current = source;
      const data = source.map((row) =>
        row.is_empty_placeholder ? { ...row, model_name: t('无数据') } : row,
      );
      const processedData = processRawData(
        data,
        dataExportDefaultTime,
        initializeMaps,
        updateMapValue,
      );

      const {
        totalQuota,
        totalTimes,
        totalTokens,
        uniqueModels,
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
      } = processedData;

      const trendDataResult = calculateTrendData(
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
        dataExportDefaultTime,
      );
      setTrendData(trendDataResult);

      const newModelColors = generateModelColors(uniqueModels, {});
      setModelColors(newModelColors);

      const aggregatedData = aggregateDataByTimeAndModel(
        data,
        dataExportDefaultTime,
      );

      const modelTotals = new Map();
      for (let [_, value] of aggregatedData) {
        updateMapValue(modelTotals, value.model, value.count);
      }

      const chartTimePoints = generateChartTimePoints(
        aggregatedData,
        data,
        dataExportDefaultTime,
      );

      let newLineData = [];

      chartTimePoints.forEach((time) => {
        let timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            rawQuota: aggregated?.quota || 0,
            Usage: aggregated?.quota
              ? getQuotaWithUnit(aggregated.quota, 4)
              : 0,
          };
        });

        const timeSum = timeData.reduce((sum, item) => sum + item.rawQuota, 0);
        timeData.sort((a, b) => b.rawQuota - a.rawQuota);
        timeData = timeData.map((item) => ({ ...item, TimeSum: timeSum }));
        newLineData.push(...timeData);
      });

      newLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      updateChartSpec(
        setSpecLine,
        newLineData,
        `${t('总计')}：${renderQuota(totalQuota, 2)}`,
        newModelColors,
        'barData',
      );

      // ===== 模型调用次数排行柱状图 =====
      const rankData = Array.from(modelTotals)
        .map(([model, count]) => ({
          Model: model,
          Count: count,
        }))
        .sort((a, b) => b.Count - a.Count);

      updateChartSpec(
        setSpecRankBar,
        rankData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'rankData',
      );

      setConsumeQuota(totalQuota);
      setTimes(totalTimes);
      setConsumeTokens(totalTokens);
    },
    [
      dataExportDefaultTime,
      setTrendData,
      generateModelColors,
      setModelColors,
      setConsumeQuota,
      setTimes,
      setConsumeTokens,
      t,
    ],
  );

  const updateUserChartData = useCallback(
    (data) => {
      lastUserData.current = data;
      const userQuotaTotal = new Map();
      data.forEach((item) => {
        const previousQuota = userQuotaTotal.get(item.username) || 0;
        userQuotaTotal.set(item.username, previousQuota + item.quota);
      });
      const rankingData = Array.from(userQuotaTotal.entries())
        .sort((a, b) => b[1] - a[1])
        .slice(0, 10)
        .map(([username, quota]) => ({
          User: username,
          Quota: quota,
        }));

      const userRankValues = rankingData
        .map((item) => ({
          User: item.User,
          rawQuota: item.Quota,
          Quota: getQuotaWithUnit(item.Quota, 4),
        }))
        .sort((a, b) => b.rawQuota - a.rawQuota);

      const totalUserQuota = rankingData.reduce(
        (sum, item) => sum + item.Quota,
        0,
      );

      setSpecUserRank((prev) => ({
        ...prev,
        data: [{ id: 'userRankData', values: userRankValues }],
        title: {
          ...prev.title,
          subtext: `${t('总计')}：${renderQuota(totalUserQuota, 2)}`,
        },
      }));
    },
    [t],
  );

  // Rebuild presentation from cached data when the language changes. Model and
  // account names stay untouched; the synthetic empty state alone is translated.
  useEffect(() => {
    setSpecLine((previous) => ({
      ...previous,
      title: {
        ...previous.title,
        text: t('模型消耗分布'),
        subtext: `${t('总计')}：${renderQuota(0, 2)}`,
      },
    }));
    setSpecRankBar((previous) => ({
      ...previous,
      title: {
        ...previous.title,
        text: t('模型调用次数排行'),
        subtext: `${t('总计')}：${renderNumber(0)}`,
      },
    }));
    setSpecUserRank((previous) => ({
      ...previous,
      title: { ...previous.title, text: t('用户消耗排行') },
    }));
    if (lastChartData.current) updateChartData(lastChartData.current);
    if (lastUserData.current) updateUserChartData(lastUserData.current);
  }, [t, updateChartData, updateUserChartData]);

  // ========== 初始化图表主题 ==========
  useEffect(() => {
    let mounted = true;
    import('@visactor/vchart-semi-theme').then(({ initVChartSemiTheme }) => {
      if (mounted) {
        initVChartSemiTheme({
          isWatchingThemeSwitch: true,
        });
      }
    });
    return () => {
      mounted = false;
    };
  }, []);

  return {
    spec_line,
    spec_rank_bar,
    spec_user_rank,
    updateChartData,
    updateUserChartData,
    generateModelColors,
  };
};
