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

import { visibleOfficialTiers } from './modelOfficialPricing';
import { officialComparisonUnit } from './modelGroupDiscount';

const inverse = { '<=': '>', '<': '>=', '>': '<=', '>=': '<' };
export function tierConditions(tiers, index) {
  const conditions = tiers[index]?.conditions || [];
  if (
    index === 1 &&
    tiers.length === 2 &&
    !conditions.length &&
    tiers[0].conditions?.length === 1
  ) {
    const first = tiers[0].conditions[0];
    return [{ ...first, op: inverse[first.op] }];
  }
  return conditions;
}

// Only a proven input-length boundary can match a multi-tier official rate.
function matchingOfficialTier(tiers, index, official) {
  if (
    tiers.length === 1 &&
    !tiers[0].conditions.length &&
    official.length === 1
  )
    return official[0];
  if (tiers.length !== 2) return undefined;
  if (index === 1 && tiers[1].conditions.length) return undefined;
  const conditions = tierConditions(tiers, index);
  if (conditions.length !== 1 || conditions[0].var !== 'len') return undefined;
  const condition = conditions[0];
  const matches = official.filter((tier) => {
    const match = tier.label.match(
      /^(?:Standard · )?(?:Input\s+)?(≤|<=|<|≥|>=|>)\s*([\d.]+)([KM]?)(?: input tokens)?$/i,
    );
    if (!match) return false;
    const op = { '≤': '<=', '≥': '>=' }[match[1]] || match[1];
    const value =
      Number(match[2]) * ({ K: 1000, M: 1000000 }[match[3].toUpperCase()] || 1);
    return condition.op === op && condition.value === value;
  });
  return matches.length === 1 ? matches[0] : undefined;
}

export function groupTierRows(model, tiers, getPricing, t) {
  const official = visibleOfficialTiers(model.official_pricing).filter(
    (tier) => tier.unit === officialComparisonUnit(model),
  );
  if (tiers.length === 1 && !tiers[0].conditions.length && official.length) {
    return official.map((tier) => ({
      ...getPricing(0),
      label: tier.label,
      officialTier: tier,
    }));
  }
  if (tiers.length)
    return tiers.map((tier, index) => {
      const reference = matchingOfficialTier(tiers, index, official);
      const conditions = tierConditions(tiers, index);
      const summary = conditions
        .map(
          (condition) =>
            `${t({ len: '上下文', p: '输入', c: '输出' }[condition.var] || condition.var)} ${condition.op} ${condition.value.toLocaleString('en-US')}`,
        )
        .join(' && ');
      return {
        ...getPricing(index),
        label: reference?.label || tier.label || t('默认'),
        condition: reference ? '' : summary || (index > 0 ? t('默认') : ''),
        officialTier: reference,
      };
    });
  if (official.length)
    return official.map((tier) => ({
      ...getPricing(),
      label: tier.label,
      officialTier: tier,
    }));
  return [];
}
