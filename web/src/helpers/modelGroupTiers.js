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

// Explicit period names compare rates, not calendars or the currently active tier.
const periodName = (label = '') => {
  const name = label.trim().toLowerCase().replace(/[ _]/g, '-');
  if (['峰时', '高峰', 'peak', 'peak-hours'].includes(name)) return 'peak';
  if (['谷时', '低谷', 'off-peak', 'off-peak-hours'].includes(name))
    return 'off-peak';
  return null;
};

export function groupComparisonUnavailable(model) {
  const official = visibleOfficialTiers(model.official_pricing);
  if (!official.length) return '暂无官方参考价';
  if (!official.some((tier) => tier.unit === officialComparisonUnit(model)))
    return '官方与本站的计价单位不同';
  return '尚未匹配到对应的官方档位';
}

function matchingPeriodTier(tiers, index, official) {
  if (tiers.length !== 2 || tiers.some((tier) => tier.conditions.length))
    return undefined;
  const periods = tiers.map((tier) => periodName(tier.label));
  if (!periods.includes('peak') || !periods.includes('off-peak'))
    return undefined;
  // Exact labels only: e.g. "US · PEAK" has additional conditions, not an alias.
  const matches = official.filter(
    (tier) => periodName(tier.label) === periods[index],
  );
  return matches.length === 1 ? matches[0] : undefined;
}

// Do not strip regions, service tiers or arbitrary qualifiers to force a match.
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
      /^(?:Standard(?:\s*·\s*|\s+))?(?:Input\s+|输入\s*)?(≤|<=|<|≥|>=|>)\s*([\d.]+)([KM]?)(?:\s*(?:input\s+)?tokens)?(?:\\?\*? Permanent 50% off)?$/i,
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
      const periodReference = matchingPeriodTier(tiers, index, official);
      const reference =
        periodReference || matchingOfficialTier(tiers, index, official);
      const conditions = tierConditions(tiers, index);
      const summary = conditions
        .map(
          (condition) =>
            `${t({ len: '上下文', p: '输入', c: '输出' }[condition.var] || condition.var)} ${condition.op} ${condition.value.toLocaleString('en-US')}`,
        )
        .join(' && ');
      return {
        ...getPricing(index),
        label: periodReference
          ? tier.label
          : reference?.label || tier.label || t('默认'),
        comparisonNote: periodReference
          ? '按峰谷档位比较，生效时段以各自规则为准'
          : undefined,
        comparisonUnavailable: reference
          ? undefined
          : groupComparisonUnavailable(model),
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
