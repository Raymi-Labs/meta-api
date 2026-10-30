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
import { healthTone } from '../../components/group-health/healthUtils';
import {
  chartDisplayValue,
  formatChartValue,
  formatChartDetailValue,
  chartDomain,
  contiguousChartPoints,
} from '../../helpers/modelChartDisplay';

export const GROUP_CHART_COLORS = ['#16b8a6', '#f5a623', '#5285ed', '#9b72cf'];
const date = (ts) =>
  new Date(ts * 1000).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });

export default function ModelMetricChart({
  title,
  groups,
  metric,
  bar = false,
  t,
  expandedView = false,
}) {
  const [hover, setHover] = useState(null);
  const [expanded, setExpanded] = useState(false);
  const [width, setWidth] = useState(400);
  const plotRef = useRef(null);
  useEffect(() => {
    const element = plotRef.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(Math.max(240, entry.contentRect.width)),
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
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
  const { min, max, ticks } = chartDomain(nums, metric);
  const tickLabels = ticks.map((value) =>
    formatChartValue(value, metric, metric === 'success_rate'),
  );
  const height = expandedView ? 320 : bar ? 260 : 220,
    left = bar
      ? Math.max(58, ...tickLabels.map((label) => label.length * 7 + 12))
      : 58,
    right = 12,
    top = 20,
    bottom = 36;
  const start = timestamps[0] || 0,
    span = Math.max(3600, (timestamps.at(-1) || start) - start + 3600);
  const slot = ((width - left - right) * 3600) / span;
  const x = (ts) =>
    left + ((ts - start + 1800) / span) * (width - left - right);
  const y = (value) =>
    height - bottom - ((value - min) / (max - min)) * (height - top - bottom);
  const hoverIndex = timestamps.indexOf(hover);
  const format = (value) => formatChartValue(value, metric);
  const detail = (value) => formatChartDetailValue(value, metric);
  const limited = (point) =>
    metric === 'success_rate' &&
    ['empty', 'limited'].includes(healthTone(point));
  const activeColor = (point, i) =>
    limited(point) ? '#a8aeb9' : GROUP_CHART_COLORS[i];
  const timeLabels = [
    ...new Set([
      0,
      ...(width > 500 ? [Math.floor((timestamps.length - 1) / 2)] : []),
      timestamps.length - 1,
    ]),
  ].filter((i) => i >= 0);
  return (
    <article
      className={
        'model-metric-chart' +
        (bar ? ' is-usage' : '') +
        (expandedView ? ' is-expanded' : '')
      }
    >
      <div className='model-chart-heading'>
        <h3>{title}</h3>
        {!expandedView && (
          <Button
            theme='borderless'
            type='tertiary'
            size='small'
            icon={<Maximize2 size={15} />}
            aria-label={t('展开') + ' · ' + title}
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
              aria-label={title}
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
                      const joined = contiguousChartPoints(prior?.ts, ts);
                      const next = chartDisplayValue(
                        map.get(ts + 3600),
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
                    {date(ts) +
                      '\n' +
                      values
                        .map(
                          (map, i) =>
                            groups[i].group +
                            ': ' +
                            detail(chartDisplayValue(map.get(ts), metric)),
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
                  {date(timestamps[i])}
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
                <b>{date(hover)}</b>
                {groups.map((group, i) => (
                  <span key={group.group}>
                    <i
                      style={{
                        background: activeColor(values[i].get(hover), i),
                      }}
                    />
                    <span>{group.group}</span>
                    <strong>
                      {detail(chartDisplayValue(values[i].get(hover), metric))}
                    </strong>
                  </span>
                ))}
              </div>
            )}
          </>
        ) : (
          <div className='model-chart-empty'>{t('暂无数据')}</div>
        )}
      </div>
      <div className='model-chart-legend'>
        {groups.map((group, i) => (
          <div key={group.group}>
            <i style={{ background: activeColor(group, i) }} />
            <span title={group.group}>{group.group}</span>
            <strong title={detail(chartDisplayValue(group, metric))}>
              {format(chartDisplayValue(group, metric))}
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
                    <th>{date(ts)}</th>
                    {values.map((map, i) => (
                      <td key={i}>
                        {detail(chartDisplayValue(map.get(ts), metric))}
                      </td>
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
          title={title}
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
            />
          )}
        </Modal>
      )}
    </article>
  );
}
