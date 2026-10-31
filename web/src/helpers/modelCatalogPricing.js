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

import {
  parseImagePriceExpression,
  isImagePriceExpression,
} from './imagePricing';
import { formatModelPricingUnitPrice } from './modelPricingCurrency';

import { BILLING_PRICING_VARS } from '../constants/billing.constants';
import { calculateModelPrice, getModelPriceItems } from './utils';
import { parseTiersFromExpr } from './render';
import { buildFirstTierGroupPriceItems } from './tieredBillingDisplay';
import { splitBillingExprAndRequestRules } from '../pages/Setting/Ratio/components/requestRuleExpr';
import { videoPriceItems } from './videoPriceDisplay';
import { isVideoPriceExpression } from './videoPricingTemplate';

const cachePriceKeys = new Set([
  'cache',
  'cache-ratio',
  'create-cache',
  'create-cache-ratio',
  'create-cache-1h',
  'create-cache-1h-ratio',
  ...BILLING_PRICING_VARS.filter((variable) => variable.group === 'cache').map(
    (variable) => `tiered-${variable.key}`,
  ),
]);

export const isCachePriceItem = (item) => cachePriceKeys.has(item?.key);

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
  if (isImagePriceExpression(options.record.billing_expr)) {
    const rows = parseImagePriceExpression(options.record.billing_expr);
    return {
      data,
      items: rows
        ? rows.flatMap((row) =>
            ['input', 'output'].map((key) => ({
              key: `image-${row.resolution}-${key}`,
              label: `${row.resolution} ${t(key === 'input' ? '输入图片' : '输出图片')}`,
              value: formatModelPricingUnitPrice(
                row[key] * data.effectiveBillingRatio,
                options.displayPrice,
              ),
              suffix: `/${t('张')}`,
              isDynamic: true,
            })),
          )
        : [
            {
              key: 'image-parameters',
              label: t('图片输入 / 输出计费'),
              value: t('见价格详情'),
              suffix: '',
              isDynamic: true,
            },
          ],
    };
  }
  if (isVideoPriceExpression(options.record.billing_expr)) {
    const videoItems = videoPriceItems(
      options.record.billing_expr,
      data.effectiveBillingRatio,
      options.displayPrice,
      t,
    );
    if (videoItems.length) return { data, items: videoItems };
    return {
      data,
      items: [
        {
          key: 'video-parameters',
          label: t('视频按秒 / 表达式'),
          value: t('见价格详情'),
          suffix: '',
          isDynamic: true,
        },
      ],
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
  return {
    data: {
      ...data,
      firstTierLabel: tier.firstTier?.label,
      unitPricesUSD: Object.fromEntries(
        BILLING_PRICING_VARS.map((variable) => [
          `tiered-${variable.key}`,
          (tier.firstTier?.[variable.field] * data.effectiveBillingRatio) /
            (options.tokenUnit === 'K' ? 1000 : 1),
        ]),
      ),
    },
    items: tier.items,
  };
}
