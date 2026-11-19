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

import React, { useEffect, useRef, useState } from 'react';
import { Button, Modal } from '@douyinfe/semi-ui';
import { Maximize2 } from 'lucide-react';
import {
  healthTone,
  uptimeTone,
} from '../../components/group-health/healthUtils';
import {
  chartDisplayValue,
  formatChartValue,
  formatChartDetailValue,
  chartDomain,
  contiguousChartPoints,
  chartSummary,
  hasChartData,
} from '../../helpers/modelChartDisplay';

export const GROUP_CHART_COLORS = ['#16b8a6', '#f5a623', '#5285ed', '#9b72cf'];
const date = (ts, daily = false) =>
  new Date(ts * 1000).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    ...(daily ? { timeZone: 'UTC' } : { hour: '2-digit', minute: '2-digit' }),
  });

export default function ModelMetricChart({
  title,
  groups: candidates,
  metric,
  bar = false,
  t,
  expandedView = false,
  bucketSeconds = 3600,
}) {
  const groups = candidates.filter((group) => hasChartData([group], metric));
  const [hover, setHover] = useState(null);
  const [expanded, setExpanded] = useState(false);
  const [width, setWidth] = useState(400);
  const plotRef = useRef(null);
  const resizePlot = () => {
    const element = plotRef.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(240, entry.contentRect.width)),
    );
    observer.observe(element);
    return () => observer.disconnect();
  };
  const timestamps = [
    ...new Set(
      groups.flatMap((group) => (group.series || []).map((point) => point.ts)),
    ),
  ].sort((a, b) => a - b);
  const values = groups.map(
    (group) => new Map((group.series || []).map((point) => [point.ts, point])),
  );
  const nums = values
    .flatMap((map) =>
      [...map.values()].map((point) => chartDisplayValue(point, metric)),
    )
    .filter((value) => value != null);
  const summaries = groups.map((group) => chartSummary(group, metric));
  const hasReference = summaries.some((summary) => summary.kind === 'external');
  const hasSnapshot = summaries.some((summary) => summary.value != null);
  const showPlot = Boolean(nums.length || hasSnapshot);
  useEffect(resizePlot, [showPlot]);
  const externalWindow = (summary) =>
    metric === 'uptime_rate'
      ? t('最近 {{hours}} 小时', { hours: 24 })
      : (summary.windowMinutes === 1440
          ? t('最近 {{hours}} 小时', { hours: 24 })
          : t('近30分钟')) +
        ' · ' +
        t('参考 P50');
  const summaryLabel = (summary) =>
    summary.kind === 'external'
      ? externalWindow(summary)
      : summary.kind === 'history'
        ? t('最近有效统计')
        : summary.kind === 'measured'
          ? t('当前汇总')
          : '';
  const displayTitle =
    hasReference && metric === 'avg_ttft_ms'
      ? t('首字时间') + ' · s'
      : hasReference && metric === 'uptime_rate' && !nums.length
        ? 'Uptime · ' + t('参考可用率')
        : title;
  const { min, max, ticks } = chartDomain(nums, metric);
  const tickLabels = ticks.map((value) =>
    formatChartValue(
      value,
      metric,
      ['success_rate', 'uptime_rate'].includes(metric),
    ),
  );
  const height = expandedView ? 320 : bar ? 260 : 220,
    left = bar
      ? Math.max(58, ...tickLabels.map((label) => label.length * 7 + 12))
      : 58,
    right = 12,
    top = 20,
    bottom = 36;
  const start = timestamps[0] || 0,
    span = Math.max(
      bucketSeconds,
      (timestamps.at(-1) || start) - start + bucketSeconds,
    );
  const slot = ((width - left - right) * bucketSeconds) / span;
  const x = (ts) =>
    left + ((ts - start + bucketSeconds / 2) / span) * (width - left - right);
  const y = (value) =>
    height - bottom - ((value - min) / (max - min)) * (height - top - bottom);
  const hoverIndex = timestamps.indexOf(hover);
  const format = (value) => formatChartValue(value, metric);
  const detail = (value) => formatChartDetailValue(value, metric);
  const pointDetail = (point) => detail(chartDisplayValue(point, metric));
  const limited = (point) =>
    ['success_rate', 'uptime_rate'].includes(metric) &&
    ['empty', 'limited'].includes(
      metric === 'uptime_rate' ? uptimeTone(point) : healthTone(point),
    );
  const activeColor = (point, i) =>
    limited(point) ? '#a8aeb9' : GROUP_CHART_COLORS[i];
  const timeLabels = [
    ...new Set([
      0,
      ...(width > 500 ? [Math.floor((timestamps.length - 1) / 2)] : []),
      timestamps.length - 1,
    ]),
  ].filter((i) => i >= 0);
  if (!showPlot) return null;
  return (
    <article
      className={
        'model-metric-chart' +
        (bar ? ' is-usage' : '') +
        (expandedView ? ' is-expanded' : '')
      }
    >
      <div className='model-chart-heading'>
        <h3>{displayTitle}</h3>
        {!expandedView && (
          <Button
            theme='borderless'
            type='tertiary'
            size='small'
            icon={<Maximize2 size={15} />}
            aria-label={t('展开') + ' · ' + displayTitle}
            onClick={() => setExpanded(true)}
          />
        )}
      </div>
      <div
        ref={plotRef}
        className='model-chart-plot'
        onMouseLeave={() => setHover(null)}
      >
        {nums.length ? (
          <>
            <svg
              viewBox={`0 0 ${width} ${height}`}
              role='img'
              aria-label={displayTitle}
            >
              {ticks.map((value, tickIndex) => (
                <g key={value}>
                  <line
                    x1={left}
                    x2={width - right}
                    y1={y(value)}
                    y2={y(value)}
                    className='model-chart-grid'
                  />
                  <text x={left - 10} y={y(value) + 4} textAnchor='end'>
                    {tickLabels[tickIndex]}
                  </text>
                </g>
              ))}
              {values.map((map, groupIndex) => {
                let previous = null;
                return (
                  <g key={groups[groupIndex].group}>
                    {timestamps.map((ts) => {
                      const point = map.get(ts),
                        value = chartDisplayValue(point, metric);
                      if (value == null) {
                        previous = null;
                        return null;
                      }
                      const prior = previous;
                      previous = { ts, point, value };
                      const joined = contiguousChartPoints(
                        prior?.ts,
                        ts,
                        bucketSeconds,
                      );
                      const next = chartDisplayValue(
                        map.get(ts + bucketSeconds),
                        metric,
                      );
                      const color = activeColor(point, groupIndex);
                      const w = Math.min(
                        36,
                        (slot * 0.72) / Math.max(1, groups.length),
                      );
                      return bar ? (
                        <rect
                          key={ts}
                          x={x(ts) - (w * groups.length) / 2 + groupIndex * w}
                          y={y(value)}
                          width={Math.max(0.5, w - 1)}
                          height={Math.max(0, height - bottom - y(value))}
                          rx='2'
                          fill={color}
                          opacity='0.88'
                        />
                      ) : (
                        <g key={ts}>
                          {joined && (
                            <path
                              d={`M${x(prior.ts)},${y(prior.value)} L${x(ts)},${y(value)}`}
                              fill='none'
                              stroke={limited(prior.point) ? '#a8aeb9' : color}
                              strokeWidth='1.8'
                              strokeLinecap='round'
                              strokeLinejoin='round'
                            />
                          )}
                          {((!joined && next == null) ||
                            limited(point) ||
                            hover === ts) && (
                            <circle
                              cx={x(ts)}
                              cy={y(value)}
                              r={hover === ts ? 3.5 : 2.3}
                              fill={color}
                              stroke='var(--semi-color-bg-0)'
                              strokeWidth='1.5'
                            />
                          )}
                        </g>
                      );
                    })}
                  </g>
                );
              })}
              {hoverIndex >= 0 && (
                <line
                  className='model-chart-cursor'
                  x1={x(hover)}
                  x2={x(hover)}
                  y1={top}
                  y2={height - bottom}
                  pointerEvents='none'
                />
              )}
              {timestamps.map((ts) => (
                <rect
                  key={ts}
                  x={x(ts) - slot / 2}
                  y={top}
                  width={slot}
                  height={height - top - bottom}
                  fill='transparent'
                  onMouseEnter={() => setHover(ts)}
                  onClick={() => setHover(ts)}
                >
                  <title>
                    {date(ts, bucketSeconds === 86400) +
                      '\n' +
                      values
                        .map(
                          (map, i) =>
                            groups[i].group + ': ' + pointDetail(map.get(ts)),
                        )
                        .join('\n')}
                  </title>
                </rect>
              ))}
              {timeLabels.map((i, index) => (
                <text
                  key={i}
                  x={
                    index === 0
                      ? left
                      : index === timeLabels.length - 1
                        ? width - right
                        : x(timestamps[i])
                  }
                  y={height - 10}
                  textAnchor={
                    index === 0
                      ? 'start'
                      : index === timeLabels.length - 1
                        ? 'end'
                        : 'middle'
                  }
                >
                  {date(timestamps[i], bucketSeconds === 86400)}
                </text>
              ))}
            </svg>
            {hoverIndex >= 0 && (
              <div
                className='model-chart-tooltip'
                style={
                  hoverIndex > timestamps.length / 2
                    ? { left: 8, right: 'auto' }
                    : { right: 8, left: 'auto' }
                }
              >
                <b>{date(hover, bucketSeconds === 86400)}</b>
                {groups.map((group, i) => (
                  <span key={group.group}>
                    <i
                      style={{
                        background: activeColor(values[i].get(hover), i),
                      }}
                    />
                    <span>{group.group}</span>
                    <strong>{pointDetail(values[i].get(hover))}</strong>
                  </span>
                ))}
              </div>
            )}
          </>
        ) : hasSnapshot ? (
          <div className='model-chart-reference-values'>
            {groups.map((group, i) => (
              <div key={group.group}>
                <span>{group.group}</span>
                <strong>
                  {summaries[i].value == null
                    ? t('暂无统计数据')
                    : format(summaries[i].value)}
                </strong>
                {summaryLabel(summaries[i]) && (
                  <small>{summaryLabel(summaries[i])}</small>
                )}
              </div>
            ))}
          </div>
        ) : (
          <div className='model-chart-empty'>{t('暂无统计数据')}</div>
        )}
      </div>
      <div className='model-chart-legend'>
        {groups.map((group, i) => (
          <div key={group.group}>
            <i style={{ background: activeColor(group, i) }} />
            <span title={group.group}>{group.group}</span>
            <strong
              title={
                summaries[i].value == null
                  ? t('暂无统计数据')
                  : detail(summaries[i].value)
              }
            >
              {summaries[i].value == null
                ? t('暂无统计数据')
                : format(summaries[i].value)}
              {summaries[i].kind === 'external' && nums.length > 0 && (
                <small> · {externalWindow(summaries[i])}</small>
              )}
            </strong>
          </div>
        ))}
      </div>
      {nums.length > 0 && (
        <details className='model-chart-data'>
          <summary>{t('统计明细')}</summary>
          <div>
            <table>
              <thead>
                <tr>
                  <th>{t('时间')}</th>
                  {groups.map((group) => (
                    <th key={group.group}>{group.group}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {timestamps.map((ts) => (
                  <tr key={ts}>
                    <th>{date(ts, bucketSeconds === 86400)}</th>
                    {values.map((map, i) => (
                      <td key={i}>{pointDetail(map.get(ts))}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
      {!expandedView && (
        <Modal
          visible={expanded}
          title={displayTitle}
          footer={null}
          width='min(1040px, calc(100vw - 32px))'
          onCancel={() => setExpanded(false)}
          className='model-chart-modal'
        >
          {expanded && (
            <ModelMetricChart
              title={title}
              groups={groups}
              metric={metric}
              bar={bar}
              t={t}
              expandedView
              bucketSeconds={bucketSeconds}
            />
          )}
        </Modal>
      )}
    </article>
  );
}
