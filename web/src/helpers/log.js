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

export function getLogOther(otherStr) {
  if (otherStr === undefined || otherStr === null || otherStr === '') {
    return {};
  }
  if (typeof otherStr === 'object') {
    return otherStr;
  }
  try {
    return JSON.parse(otherStr);
  } catch (e) {
    console.error(`Failed to parse record.other: "${otherStr}".`, e);
    return null;
  }
}

function formatRatioValue(value) {
  const ratio = Number(value);
  if (!Number.isFinite(ratio) || ratio <= 0) {
    return '1';
  }
  return ratio.toFixed(4).replace(/\.?0+$/, '');
}

export function formatTimeRatioLogDetail(other, t = (value) => value) {
  if (!other || typeof other !== 'object') {
    return '';
  }
  if (other.no_discount === true) return t('不参与折扣，按模型基础价计费');
  const ratio = Number(other.time_ratio);
  const hasRule = Boolean(other.time_ratio_rule);
  if ((!Number.isFinite(ratio) || ratio === 1) && !hasRule) {
    return '';
  }

  const parts = [
    `${t('当前时间倍率')}：${formatRatioValue(ratio)}x`,
    hasRule ? `${t('命中规则')}：${other.time_ratio_rule}` : '',
    other.time_ratio_timezone
      ? `${t('规则时区')}：${other.time_ratio_timezone}`
      : '',
    other.time_ratio_matched_at
      ? `${t('匹配时间')}：${other.time_ratio_matched_at}`
      : '',
  ].filter(Boolean);

  return parts.join('，');
}

export function formatTimeRatioLogSummary(other, t = (value) => value) {
  if (!other || typeof other !== 'object') {
    return '';
  }
  if (other.no_discount === true) return t('不参与折扣，按模型基础价计费');
  const ratio = Number(other.time_ratio);
  const hasRule = Boolean(other.time_ratio_rule);
  if ((!Number.isFinite(ratio) || ratio === 1) && !hasRule) {
    return '';
  }
  const rule = hasRule ? ` · ${other.time_ratio_rule}` : '';
  return `${t('时间倍率')} ${formatRatioValue(ratio)}x${rule}`;
}
