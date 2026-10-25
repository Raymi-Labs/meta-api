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

const modes = new Set(['inherit', 'auto', 'zhipu-glm53-v1']);
const efforts = new Set(['minimal', 'low', 'medium', 'high', 'xhigh', 'max']);

export function readReasoningAdaptation(setting) {
  let parsed = {};
  try {
    parsed =
      typeof setting === 'string' ? JSON.parse(setting || '{}') : setting || {};
  } catch {
    /* Existing settings errors are handled by the channel editor. */
  }
  const policy = parsed.reasoning_adaptation || {};
  return {
    reasoning_adaptation_mode: policy.mode || 'inherit',
    reasoning_adaptation_rules: JSON.stringify(policy.rules || [], null, 2),
  };
}

export function buildReasoningAdaptation(mode, rawRules) {
  if (!modes.has(mode)) throw new Error('Invalid adaptation mode');
  const rules = JSON.parse(rawRules || '[]');
  if (!Array.isArray(rules) || rules.length > 32)
    throw new Error('Use an array of at most 32 rules');
  const seen = new Set();
  for (const rule of rules) {
    if (
      !rule ||
      typeof rule.model !== 'string' ||
      !rule.model ||
      rule.model.length > 255 ||
      rule.model !== rule.model.trim() ||
      /[*?\r\n\t]/.test(rule.model)
    )
      throw new Error('Use an exact upstream model name');
    if (rule.protocol !== 'chat')
      throw new Error('Only chat protocol is supported');
    if (seen.has(rule.model)) throw new Error('Duplicate model rule');
    seen.add(rule.model);
    if (
      !rule.mapping ||
      typeof rule.mapping !== 'object' ||
      Array.isArray(rule.mapping)
    )
      throw new Error('Each rule needs an effort mapping');
    const entries = Object.entries(rule.mapping);
    if (
      !entries.length ||
      entries.length > 6 ||
      entries.some(([from, to]) => !efforts.has(from) || !efforts.has(to))
    )
      throw new Error('Invalid effort mapping');
  }
  return { mode, rules };
}
