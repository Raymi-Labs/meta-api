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

export const MODEL_MODALITY_LABELS = {
  text: '文本',
  image: '图像',
  audio: '音频',
  video: '视频',
  file: '文件',
};

export function formatModelContextExact(value) {
  return Number.isSafeInteger(Number(value)) && Number(value) > 0
    ? Number(value).toLocaleString() + ' tokens'
    : undefined;
}

export function formatModelContext(value) {
  if (!formatModelContextExact(value)) return '—';
  const tokens = Number(value);
  // Keep the compact limit conservative; the tooltip retains the exact value.
  if (tokens >= 1000000) return `${Math.floor(tokens / 100000) / 10}M`;
  if (tokens >= 1000) return `${Math.floor(tokens / 1000)}K`;
  return `${tokens} tokens`;
}

export function formatModelThroughput(value) {
  return value != null && Number.isFinite(Number(value)) && Number(value) >= 0
    ? `${Number(value).toFixed(1)} tok/s`
    : '—';
}

// Round the original converted amount, not an already rounded price string.
export function formatModelCardPrice(amount, symbol = '$') {
  if (amount == null || !Number.isFinite(amount) || amount < 0) return null;
  const value = new Intl.NumberFormat('en-US', {
    useGrouping: false,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount);
  return amount > 0 && value === '0.00'
    ? `<${symbol}0.01`
    : `${symbol}${value}`;
}

// Nested controls and text selection retain their own interactions.
export function shouldNavigateModelCard(event, selection = '') {
  return (
    !event.defaultPrevented &&
    event.button === 0 &&
    !selection &&
    !event.target.closest(
      'a,button,input,label,select,textarea,[role="checkbox"],.semi-checkbox',
    )
  );
}

export function selectComparisonGroups(available, selected, limit = 4) {
  return [...new Set(selected)]
    .filter((name) => available.includes(name))
    .slice(0, limit);
}

export function chartMetricValue(point, metric) {
  const value = point?.[metric];
  if (
    metric === 'request_count' &&
    !point?.observed_count &&
    !point?.model_usage_reference
  )
    return null;
  return value != null && Number.isFinite(Number(value)) && Number(value) >= 0
    ? Number(value)
    : null;
}
