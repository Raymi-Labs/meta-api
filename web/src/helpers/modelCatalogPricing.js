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

import { calculateModelPrice, getModelPriceItems } from './utils';
import { parseTiersFromExpr } from './render';
import { buildFirstTierGroupPriceItems } from './tieredBillingDisplay';
import { splitBillingExprAndRequestRules } from '../pages/Setting/Ratio/components/requestRuleExpr';

// Cards and the comparison table must use the same group, currency and tier.
export function getModelCatalogPricing(options, t) {
  const precision = options.tokenUnit === 'K' ? 6 : 4;
  const data = calculateModelPrice({ ...options, precision });
  if (!data.isDynamicPricing) {
    return {
      data,
      items: getModelPriceItems(data, t, options.quotaDisplayType),
    };
  }
  const { billingExpr } = splitBillingExprAndRequestRules(
    options.record.billing_expr || '',
  );
  const tier = buildFirstTierGroupPriceItems({
    tiers: parseTiersFromExpr(billingExpr),
    effectiveBillingRatio: data.effectiveBillingRatio,
    displayPrice: options.displayPrice,
    tokenUnit: options.tokenUnit,
    precision,
    t,
  });
  return { data, items: tier.items };
}
