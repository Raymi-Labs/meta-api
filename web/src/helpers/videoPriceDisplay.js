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

import { formatModelPricingUnitPrice } from './modelPricingCurrency.js';
import { buildH3SecondsExpression, h3VideoProfiles } from './h3VideoPricing.js';
import { buildVideoPricingTemplate } from './videoPricingTemplate.js';

// Keep quoted values intact. Only known, complete generated templates may be
// labelled by resolution: partial matches could omit charges or request rules.
const compactExpression = (expression) =>
  expression.replace(/"(?:[^"\\]|\\.)*"|\s+/g, (part) =>
    part.startsWith('"') ? part : '',
  );

export function videoResolutionRates(expression = '') {
  if (typeof expression !== 'string' || expression.length > 100000) return [];
  const source = compactExpression(expression.replace(/^v1:/, ''));
  const number = '(\\d+(?:\\.\\d+)?(?:[eE][+-]?\\d+)?)';
  const h3Pattern = new RegExp(
    'param\\("resolution"\\)=="([^"]+)"\\?tier\\("video_[^"]+",param\\("duration"\\)\\*' +
      number +
      '\\)',
    'g',
  );
  const rates = Object.fromEntries(
    [...source.matchAll(h3Pattern)].map((m) => [m[1], Number(m[2]) / 1e6]),
  );
  for (const [model, profile] of Object.entries(h3VideoProfiles)) {
    const rebuilt = buildH3SecondsExpression(model, rates);
    if (rebuilt && compactExpression(rebuilt) === source)
      return profile.resolutions.map((resolution) => ({
        resolution,
        min: profile.min,
        max: profile.max,
        rate: rates[resolution],
        multiple: false,
      }));
  }
  const rowPattern = new RegExp(
    'param\\("resolution"\\)=="([^"]+)"&&param\\("duration"\\)>=(\\d+)&&param\\("duration"\\)<=(\\d+)\\?tier\\("video_\\d+",param\\("duration"\\)\\*' +
      number +
      '\\*\\(param\\("sampleCount"\\)\\?\\?1\\)\\)',
    'g',
  );
  // The generator nests the last configured row first.
  const rows = [...source.matchAll(rowPattern)].reverse().map((m) => ({
    resolution: m[1],
    min: Number(m[2]),
    max: Number(m[3]),
    rate: Number(m[4]) / 1e6,
    multiple: true,
  }));
  const rebuilt = rows.length <= 100 && buildVideoPricingTemplate(rows);
  return rebuilt && compactExpression(rebuilt) === source ? rows : [];
}

// Do not evaluate arbitrary billing expressions in the browser. Recognize only
// linear duration tiers and retain the existing fallback for everything else.
export function videoSecondRates(expression = '') {
  if (typeof expression !== 'string' || expression.length > 100000) return [];
  const rows = videoResolutionRates(expression);
  if (rows.length)
    return [...new Set(rows.map((row) => row.rate))].sort((a, b) => a - b);
  const rates = [];
  function scan(text) {
    let depth = 0,
      quoted = false,
      escaped = false;
    const top = [];
    for (let i = 0; i < text.length; i++) {
      const c = text[i];
      if (quoted) {
        if (escaped) escaped = false;
        else if (c === '\\') escaped = true;
        else if (c === '"') quoted = false;
        continue;
      }
      if (c === '"') quoted = true;
      else if (c === '(') depth++;
      else if (c === ')') {
        depth--;
        if (depth < 0) return null;
      }
      if (!quoted && depth === 0) top.push(i);
    }
    return depth || quoted ? null : top;
  }
  function visit(source, nesting = 0) {
    if (nesting > 64) return false;
    let text = source.trim(),
      positions = scan(text);
    if (!positions) return false;
    while (text.startsWith('(') && positions[0] === text.length - 1) {
      text = text.slice(1, -1).trim();
      positions = scan(text);
      if (!positions) return false;
    }
    const question = positions.find(
      (i) => text[i] === '?' && text[i - 1] !== '?' && text[i + 1] !== '?',
    );
    if (question !== undefined) {
      let branches = 0,
        colon;
      for (const i of positions.filter((i) => i > question)) {
        if (text[i] === '?' && text[i - 1] !== '?' && text[i + 1] !== '?')
          branches++;
        if (text[i] === ':') {
          if (branches === 0) {
            colon = i;
            break;
          }
          branches--;
        }
      }
      return (
        colon !== undefined &&
        visit(text.slice(question + 1, colon), nesting + 1) &&
        visit(text.slice(colon + 1), nesting + 1)
      );
    }
    if (
      /^tier\(\s*"(?:video_config_preview|configuration_preview)"\s*,\s*0\s*\)$/.test(
        text,
      ) ||
      /^tier\(\s*"unsupported_video_parameters"\s*,\s*-1\s*\)$/.test(text)
    )
      return true;
    const match = text.match(
      /^tier\(\s*"[^"\n]+"\s*,\s*param\(\s*"duration"\s*\)\s*\*\s*(\d+(?:\.\d+)?)\s*(?:\*\s*\(param\("sampleCount"\)\s*\?\?\s*1\))?\s*\)$/,
    );
    if (!match) return false;
    const price = Number(match[1]) / 1e6;
    if (!Number.isFinite(price) || price < 0) return false;
    rates.push(price);
    return true;
  }
  return visit(expression) ? [...new Set(rates)].sort((a, b) => a - b) : [];
}

export function videoPriceItems(expression, ratio, displayPrice, t = (s) => s) {
  if (ratio == null || !Number.isFinite(Number(ratio)) || Number(ratio) < 0)
    return [];
  const rows = videoResolutionRates(expression);
  if (rows.length)
    return rows.map((row, index) => {
      const unitPriceUSD = row.rate * Number(ratio);
      const value = formatModelPricingUnitPrice(unitPriceUSD, displayPrice, 4);
      const amount = displayPrice?.toAmount?.(unitPriceUSD) ?? unitPriceUSD;
      const duration = `${row.min === row.max ? row.min : `${row.min}–${row.max}`} ${t('秒')}`;
      const repeated =
        rows.filter((other) => other.resolution === row.resolution).length > 1;
      return {
        key: `video-resolution-${index}`,
        label: row.resolution + (repeated ? ` · ${duration}` : ''),
        value:
          amount > 0 && Number(amount.toFixed(4)) === 0
            ? `<${displayPrice?.currencySymbol || '$'}0.0001`
            : value,
        suffix: ` / ${t('秒')}`,
        duration,
        multiple: row.multiple,
        unitPriceUSD,
        isDynamic: false,
      };
    });
  const rates = videoSecondRates(expression);
  if (!rates.length) return [];
  const values = rates.map((rate) =>
    formatModelPricingUnitPrice(rate * Number(ratio), displayPrice, 4),
  );
  return [
    {
      key: 'video-second',
      label: t('视频按秒 / 表达式'),
      value:
        values.length === 1
          ? values[0]
          : `${values[0]} – ${values[values.length - 1]}`,
      suffix: ' / s',
      isDynamic: false,
    },
  ];
}

export function isVideoConsumption(record, other) {
  if (
    Number(record?.type) !== 2 ||
    other?.violation_fee === true ||
    other?.violation_fee_code ||
    other?.violation_fee_marker
  )
    return false;
  if (other?.task_billing?.kind === 'video_expression') return true;
  return /^\/v1\/(?:videos(?:\/|$)|video\/generations(?:\/|$))/.test(
    other?.request_path || '',
  );
}

export function videoLogSegments(record, other, quota, t = (s) => s) {
  const result = [
    {
      text: `${t('扣费')}：${quota(Number(record.quota), 6)}`,
      tone: 'primary',
    },
  ];
  const snapshot = other?.task_billing;
  if (
    snapshot &&
    Number.isFinite(snapshot.base_quota) &&
    Number.isFinite(snapshot.group_ratio) &&
    Number.isFinite(snapshot.time_ratio)
  ) {
    result.push({
      text: `${quota(snapshot.base_quota, 6)} × ${snapshot.group_ratio} × ${snapshot.time_ratio} = ${quota(Number(record.quota), 6)}`,
      tone: 'secondary',
    });
  }
  const seconds = Number(other?.task_billing?.requested_seconds);
  if (
    Number.isFinite(seconds) &&
    seconds > 0 &&
    Number.isFinite(Number(record.quota))
  ) {
    result.push({
      text: `${seconds} s · ${quota(Number(record.quota) / seconds, 6)} / s`,
      tone: 'secondary',
    });
  }
  if (other?.task_billing?.matched_tier)
    result.push({
      text: String(other.task_billing.matched_tier),
      tone: 'secondary',
    });
  return result;
}
