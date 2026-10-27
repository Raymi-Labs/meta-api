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

export const h3VideoProfiles = {
  'MiniMax-H3': { resolutions: ['768P', '2K'], min: 4, max: 15 },
  'MiniMax-H3-Highspeed': { resolutions: ['768P'], min: 4, max: 15 },
  'MiniMax-H3-Lite': { resolutions: ['480P', '768P'], min: 4, max: 15 },
  'MiniMax-H3-Max': { resolutions: ['480P', '768P'], min: 5, max: 15 },
};

// Prices are USD per requested output second, before group/time multipliers.
// Unknown/missing parameters fail billing rather than silently matching a price.
export function buildH3SecondsExpression(model, rates) {
  const profile = h3VideoProfiles[model];
  if (!profile) return null;
  let expr = 'tier("unsupported_video_parameters", -1)';
  for (const resolution of [...profile.resolutions].reverse()) {
    const value = rates[resolution];
    if (
      value === '' ||
      value == null ||
      !Number.isFinite(Number(value)) ||
      Number(value) < 0 ||
      Number(value) > 1000000
    )
      return null;
    const coefficient = Number((Number(value) * 1000000).toPrecision(15));
    expr =
      'param("resolution") == "' +
      resolution +
      '" ? tier("video_' +
      resolution +
      '", param("duration") * ' +
      coefficient +
      ') : (' +
      expr +
      ')';
  }
  // The settings validator evaluates an empty request. H3 submission rejects
  // missing duration before billing; this branch is only a configuration preview.
  return (
    'param("duration") == nil ? tier("configuration_preview", 0) : (param("duration") >= ' +
    profile.min +
    ' && param("duration") <= ' +
    profile.max +
    ' ? (' +
    expr +
    ') : tier("invalid_video_duration", -1))'
  );
}
