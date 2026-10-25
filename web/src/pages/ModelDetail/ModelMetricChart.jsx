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

import React, { useState } from 'react';
import { chartMetricValue } from '../../helpers/modelPresentation';
import { healthTone } from '../../components/group-health/healthUtils';

export const GROUP_CHART_COLORS = ['#3978f6', '#00a995', '#e5a000', '#9b62d6'];
const number = (value) =>
  value == null
    ? '—'
    : new Intl.NumberFormat(undefined, {
        maximumFractionDigits: 2,
        notation: Math.abs(value) >= 10000 ? 'compact' : 'standard',
      }).format(value);
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
  unit = '',
  bar = false,
  t,
}) {
  const [hover, setHover] = useState(null);
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
      [...map.values()].map((point) => chartMetricValue(point, metric)),
    )
    .filter((value) => value != null);
  const max = metric === 'success_rate' ? 100 : Math.max(1, ...nums) * 1.1;
  const width = 400,
    height = 214,
    left = 50,
    right = 15,
    top = 18,
    bottom = 35;
  const x = (i) =>
    left +
    ((i + 0.5) / Math.max(1, timestamps.length)) * (width - left - right);
  const y = (value) =>
    height - bottom - (value / max) * (height - top - bottom);
  const hoverIndex = timestamps.indexOf(hover);
  const format = (value) => `${number(value)}${value == null ? '' : unit}`;
  return (
    <article className={'model-metric-chart' + (bar ? ' is-usage' : '')}>
      <h3>{title}</h3>
      {nums.length ? (
        <div className='model-chart-plot' onMouseLeave={() => setHover(null)}>
          <svg viewBox={`0 0 ${width} ${height}`} role='img' aria-label={title}>
            {[0, 0.5, 1].map((ratio) => (
              <g key={ratio}>
                <line
                  x1={left}
                  x2={width - right}
                  y1={y(max * ratio)}
                  y2={y(max * ratio)}
                  className='model-chart-grid'
                />
                <text x={left - 8} y={y(max * ratio) + 4} textAnchor='end'>
                  {number(max * ratio)}
                  {metric === 'success_rate' ? '%' : ''}
                </text>
              </g>
            ))}
            {values.map((map, groupIndex) => {
              let path = '';
              let previous = false;
              timestamps.forEach((ts, i) => {
                const v = chartMetricValue(map.get(ts), metric);
                if (v == null) {
                  previous = false;
                  return;
                }
                path += `${previous ? 'L' : 'M'}${x(i)},${y(v)} `;
                previous = true;
              });
              return (
                <g key={groups[groupIndex].group}>
                  {!bar && (
                    <path
                      d={path}
                      fill='none'
                      stroke={GROUP_CHART_COLORS[groupIndex]}
                      strokeWidth='2'
                      vectorEffect='non-scaling-stroke'
                    />
                  )}
                  {timestamps.map((ts, i) => {
                    const point = map.get(ts),
                      value = chartMetricValue(point, metric);
                    if (value == null) return null;
                    const color =
                      metric === 'success_rate' &&
                      ['empty', 'limited'].includes(healthTone(point))
                        ? '#a8aeb9'
                        : GROUP_CHART_COLORS[groupIndex];
                    const slot =
                        (width - left - right) / Math.max(1, timestamps.length),
                      w = Math.max(0.8, (slot * 0.8) / groups.length);
                    return bar ? (
                      <rect
                        key={ts}
                        x={x(i) - slot * 0.4 + groupIndex * w}
                        y={y(value)}
                        width={w}
                        height={Math.max(1, height - bottom - y(value))}
                        fill={color}
                      >
                        <title>
                          {groups[groupIndex].group} · {date(ts)} ·{' '}
                          {format(value)}
                        </title>
                      </rect>
                    ) : (
                      <circle
                        key={ts}
                        cx={x(i)}
                        cy={y(value)}
                        r='2.2'
                        fill={color}
                      >
                        <title>
                          {groups[groupIndex].group} · {date(ts)} ·{' '}
                          {format(value)}
                        </title>
                      </circle>
                    );
                  })}
                </g>
              );
            })}
            {timestamps.map((ts, i) => (
              <rect
                key={ts}
                x={left + (i / timestamps.length) * (width - left - right)}
                y={top}
                width={(width - left - right) / timestamps.length}
                height={height - top - bottom}
                fill='transparent'
                onMouseEnter={() => setHover(ts)}
              >
                <title>
                  {date(ts) +
                    '\n' +
                    values
                      .map(
                        (map, index) =>
                          groups[index].group +
                          ': ' +
                          format(chartMetricValue(map.get(ts), metric)),
                      )
                      .join('\n')}
                </title>
              </rect>
            ))}
            {hoverIndex >= 0 && (
              <line
                className='model-chart-cursor'
                x1={x(hoverIndex)}
                x2={x(hoverIndex)}
                y1={top}
                y2={height - bottom}
                pointerEvents='none'
              />
            )}
            <text x={left} y={height - 10}>
              {timestamps.length ? date(timestamps[0]) : ''}
            </text>
            <text x={width - right} y={height - 10} textAnchor='end'>
              {timestamps.length ? date(timestamps.at(-1)) : ''}
            </text>
          </svg>
          {hoverIndex >= 0 && (
            <div className='model-chart-tooltip'>
              <b>{date(hover)}</b>
              {groups.map((group, i) => (
                <span key={group.group}>
                  <i style={{ background: GROUP_CHART_COLORS[i] }} />
                  {group.group}
                  <strong>
                    {format(chartMetricValue(values[i].get(hover), metric))}
                  </strong>
                </span>
              ))}
            </div>
          )}
        </div>
      ) : (
        <div className='model-chart-empty'>{t('暂无数据')}</div>
      )}
      <div className='model-chart-legend'>
        {groups.map((group, i) => (
          <div key={group.group}>
            <i style={{ background: GROUP_CHART_COLORS[i] }} />
            <span>{group.group}</span>
            <strong>{format(chartMetricValue(group, metric))}</strong>
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
                        {format(chartMetricValue(map.get(ts), metric))}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </article>
  );
}
