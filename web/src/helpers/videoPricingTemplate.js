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

import { isWanImageModel } from './videoTest.js';

// This is a configuration helper for existing protocol families, not a model
// availability claim. Do not offer it for unsupported vendors detected by the
// broader safety-only video test matcher.
export const canUseVideoTemplate = (name = '') =>
  !isWanImageModel(name) &&
  /^(?:grok-imagine-video(?:[-.]|$)|sora-2(?:[-.]|$)|veo(?:[-.\d]|$)|wanx?\d|kling(?:[-.]|$)|vidu|MiniMax-H3(?:-|$)|MiniMax-Hailuo(?:-|$)|T2V-01|I2V-01|S2V-01|doubao-seedance(?:-|$)|seedance(?:-|$))/i.test(
    name,
  );
export function buildVideoPricingTemplate(rows, mode = 'seconds') {
  if (!['seconds', 'fixed'].includes(mode) || !rows.length) return null;
  let expr = 'tier("unsupported_video_parameters", -1)';
  const seen = [];
  for (const [index, row] of rows.entries()) {
    const resolution = String(row.resolution || '')
      .trim()
      .toUpperCase();
    const min = Number(row.min),
      max = mode === 'fixed' ? min : Number(row.max),
      rate = Number(row.rate);
    if (
      !/^[A-Z0-9*X:_-]{1,40}$/.test(resolution) ||
      row.min === '' ||
      row.min == null ||
      (mode === 'seconds' && (row.max === '' || row.max == null)) ||
      !Number.isInteger(min) ||
      !Number.isInteger(max) ||
      min < 1 ||
      max < min ||
      max > 3600 ||
      row.rate === '' ||
      row.rate == null ||
      !Number.isFinite(rate) ||
      rate < 0 ||
      rate > 1000000
    )
      return null;
    if (
      seen.some(
        (r) => r.resolution === resolution && min <= r.max && max >= r.min,
      )
    )
      return null;
    seen.push({ resolution, min, max });
    const coefficient = Number((rate * 1000000).toPrecision(15));
    const cost =
      (mode === 'seconds' ? 'param("duration") * ' : '') +
      coefficient +
      ' * (param("sampleCount") ?? 1)';
    expr =
      'param("resolution") == ' +
      JSON.stringify(resolution) +
      ' && param("duration") >= ' +
      min +
      ' && param("duration") <= ' +
      max +
      ' ? tier("video_' +
      index +
      '", ' +
      cost +
      ') : (' +
      expr +
      ')';
  }
  return (
    'param("duration") == nil ? tier("video_config_preview", 0) : (' +
    expr +
    ')'
  );
}

export const isVideoPriceExpression = (expr = '') =>
  /tier\("(?:video_config_preview|configuration_preview)"/.test(expr);
