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

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../helpers/dashboardFormat';
import {
  dashboardMetricValues,
  dashboardCompactTokens,
  dashboardMetricCoverage,
} from '../../helpers/dashboardMetrics';

export const useDashboardStats = (
  consumeQuota,
  consumeTokens,
  times,
  cacheRate,
  t,
  summary,
) => {
  const { i18n } = useTranslation();
  const locale = i18n?.resolvedLanguage || 'zh-CN';
  const groupedStatsData = useMemo(() => {
    const values = summary
      ? {
          quota: summary.quota,
          tokens: summary.tokens,
          requests: summary.requests,
          cacheRate: summary.cache_rate,
          unitPrice: summary.unit_price,
        }
      : dashboardMetricValues(consumeQuota, consumeTokens, times, cacheRate);
    const currency = (value) => (value == null ? '—' : renderQuota(value));
    const number = (value) => (value == null ? '—' : value.toLocaleString());
    return [
      {
        id: 'spend',
        title: t('总消费'),
        value: currency(values.quota),
        description: t(
          '所选时段保留日志的消费减退款，按结算事件时间统计；可为负数。',
        ),
      },
      { id: 'requests', title: t('请求数'), value: number(values.requests) },
      {
        id: 'tokens',
        title: t('Token 用量'),
        value: dashboardCompactTokens(values.tokens, locale),
        fullValue: number(values.tokens),
        description: `${number(values.tokens)} Token · ${t(
          '完整输入（含缓存读取与创建）加输出，已包含的缓存不重复计算。',
        )}`,
      },
      {
        id: 'cache',
        coverage: dashboardMetricCoverage(summary, 'cache', t),
        title: t('缓存命中率'),
        value:
          values.cacheRate == null || values.tokens === 0
            ? '—'
            : `${(values.cacheRate * 100).toFixed(1)}%`,
        description: t(
          '缓存读取 Token ÷ 全部输入 Token，基于当前周期保留的消费日志。',
        ),
      },
      {
        id: 'unit-price',
        coverage: dashboardMetricCoverage(summary, 'price', t),
        title: t('每百万 Token 综合单价'),
        value:
          values.unitPrice == null ? '—' : renderQuota(values.unitPrice, 4),
        description: t(
          '仅按 Token 计费的净消费 ÷ 对应用量 × 1,000,000；排除按次、按时长及无法分类的费用，并非模型报价。',
        ),
      },
    ];
  }, [consumeQuota, consumeTokens, times, cacheRate, t, summary, locale]);
  return { groupedStatsData };
};
