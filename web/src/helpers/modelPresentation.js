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

export function formatModelContext(value) {
  return Number.isSafeInteger(Number(value)) && Number(value) > 0
    ? Number(value).toLocaleString() + ' tokens'
    : '—';
}

export function formatModelThroughput(value) {
  return value != null && Number.isFinite(Number(value)) && Number(value) >= 0
    ? `${Number(value).toFixed(1)} tok/s`
    : '—';
}

// Nested controls and text selection retain their own interactions.
export function shouldNavigateModelCard(event, selection = '') {
  return (
    !event.defaultPrevented &&
    event.button === 0 &&
    !selection &&
    !event.target.closest(
      'a,button,input,label,select,textarea,[role="checkbox"]',
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
  if (metric === 'request_count' && !point?.observed_count) return null;
  return value != null && Number.isFinite(Number(value)) && Number(value) >= 0
    ? Number(value)
    : null;
}
