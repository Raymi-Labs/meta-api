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

// Filters shown in the overview describe the last applied query, not a modal draft.
export function validateDashboardQuery(inputs, grain, isAdmin) {
  const start = new Date(inputs.start_timestamp).getTime();
  const end = new Date(inputs.end_timestamp).getTime();
  if (
    !inputs.start_timestamp ||
    !inputs.end_timestamp ||
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    end <= start
  )
    return 'invalid-range';
  if (!['hour', 'day', 'week'].includes(grain)) return 'invalid-grain';
  if (!isAdmin && end - start > 30 * 86400000) return 'range-too-long';
  return null;
}

export function dashboardRates(times, tokens, inputs) {
  const minutes =
    (new Date(inputs.end_timestamp).getTime() -
      new Date(inputs.start_timestamp).getTime()) /
    60000;
  const rate = (value) =>
    Number.isFinite(minutes) && minutes > 0 && Number.isFinite(Number(value))
      ? (Number(value) / minutes).toFixed(3)
      : '—';
  return { avgRPM: rate(times), avgTPM: rate(tokens), timeDiff: minutes };
}

export function dashboardQuotaRows(data, now = Date.now()) {
  if (!Array.isArray(data)) throw new Error('Invalid quota response');
  if (!data.length)
    return [
      {
        count: 0,
        quota: 0,
        token_used: 0,
        model_name: '',
        is_empty_placeholder: true,
        created_at: now / 1000,
      },
    ];
  return [...data].sort((a, b) => a.created_at - b.created_at);
}

// Same role threshold as AdminAuth and the existing sidebar (10=admin, 100=root).
export const isDashboardAdmin = (user) => Number(user?.role) >= 10;

// Calendar "today" follows the displayed browser timezone. Rolling ranges use
// exact elapsed seconds, so a DST boundary cannot exceed the self API's 30 days.
export function dashboardPresetRange(preset, now = new Date()) {
  const end = new Date(Math.floor(new Date(now).getTime() / 1000) * 1000);
  if (!Number.isFinite(end.getTime())) return null;
  let start;
  if (preset === 'today') {
    start = new Date(end);
    start.setHours(0, 0, 0, 0);
  } else {
    const days = { '24h': 1, '7d': 7, '30d': 30 }[preset];
    if (!days) return null;
    start = new Date(end.getTime() - days * 86400000);
  }
  if (start >= end) return null;
  return {
    start_timestamp: start.toISOString(),
    end_timestamp: end.toISOString(),
    grain: ['7d', '30d'].includes(preset) ? 'day' : 'hour',
  };
}

export function dashboardQuickActions(isAdmin, isVisible) {
  const actions = [
    ['console', 'token', '/console/token', '令牌管理'],
    ['console', 'log', '/console/log', '使用日志'],
    ...(isAdmin
      ? [
          ['admin', 'channel', '/console/channel', '渠道管理'],
          ['admin', 'user', '/console/user', '用户管理'],
        ]
      : [['personal', 'topup', '/console/topup', '充值']]),
  ];
  return actions.filter(
    ([section, module]) => isVisible(section) && isVisible(section, module),
  );
}
