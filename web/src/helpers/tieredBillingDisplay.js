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

import { BILLING_PRICING_VARS } from '../constants/billing.constants.js';
import { formatModelPricingUnitPrice } from './modelPricingCurrency.js';
import {
  getCacheCreationBreakdown,
  promptTokensExcludeCache,
} from './cacheCreation.js';

const numberOrZero = (value) => {
  const numeric = Number(value || 0);
  return Number.isFinite(numeric) ? numeric : 0;
};

export const buildFirstTierGroupPriceItems = ({
  tiers,
  effectiveBillingRatio,
  displayPrice,
  tokenUnit = 'M',
  precision = 4,
  t = (value) => value,
}) => {
  const firstTier = Array.isArray(tiers) ? tiers[0] : null;
  if (!firstTier) {
    return { firstTier: null, items: [] };
  }

  const numericRatio = Number(effectiveBillingRatio);
  const safeRatio = Number.isFinite(numericRatio) ? numericRatio : 1;
  const items = BILLING_PRICING_VARS.map((variable) => {
    const unitPriceUSD = numberOrZero(firstTier[variable.field]);
    if (unitPriceUSD === 0) return null;
    return {
      key: `tiered-${variable.key}`,
      label: t(variable.label),
      value: formatModelPricingUnitPrice(
        (unitPriceUSD * safeRatio) / (tokenUnit === 'K' ? 1000 : 1),
        displayPrice,
        precision,
      ),
      suffix: tokenUnit === 'K' ? ' / 1K Tokens' : ' / 1M Tokens',
    };
  }).filter(Boolean);

  return { firstTier, items };
};

const hasTierPrice = (tier, key) => {
  const variable = BILLING_PRICING_VARS.find((item) => item.key === key);
  return variable ? numberOrZero(tier?.[variable.field]) !== 0 : false;
};

export const resolveTieredLogParams = (record, tier) => {
  if (record?.tiered_params && typeof record.tiered_params === 'object') {
    return Object.fromEntries(
      ['p', 'c', 'len', 'cr', 'cc', 'cc1h', 'img', 'img_o', 'ai', 'ao'].map(
        (key) => [key, numberOrZero(record.tiered_params[key])],
      ),
    );
  }

  const promptTokens = numberOrZero(record?.prompt_tokens);
  const completionTokens = numberOrZero(record?.completion_tokens);
  const cacheReadTokens = numberOrZero(record?.cache_tokens);
  const creation = getCacheCreationBreakdown(record);
  const cacheCreation1h = creation.oneHour;
  const cacheCreation5m = creation.fiveMinute + creation.unclassified;
  const imageInputTokens = numberOrZero(
    record?.image_input_tokens ?? record?.image_output,
  );
  const imageOutputTokens = numberOrZero(record?.image_output_tokens);
  const audioInputTokens = numberOrZero(
    record?.audio_input_token_count ?? record?.audio_input,
  );
  const audioOutputTokens = numberOrZero(record?.audio_output);
  const excludesCache = promptTokensExcludeCache(record);

  let promptPriceTokens = promptTokens;
  let completionPriceTokens = completionTokens;
  if (!excludesCache) {
    if (hasTierPrice(tier, 'cr')) promptPriceTokens -= cacheReadTokens;
    if (hasTierPrice(tier, 'cc')) promptPriceTokens -= cacheCreation5m;
    if (hasTierPrice(tier, 'cc1h')) promptPriceTokens -= cacheCreation1h;
    if (hasTierPrice(tier, 'img')) promptPriceTokens -= imageInputTokens;
    if (hasTierPrice(tier, 'ai')) promptPriceTokens -= audioInputTokens;
    if (hasTierPrice(tier, 'img_o')) {
      completionPriceTokens -= imageOutputTokens;
    }
    if (hasTierPrice(tier, 'ao')) {
      completionPriceTokens -= audioOutputTokens;
    }
  }

  return {
    p: Math.max(promptPriceTokens, 0),
    c: Math.max(completionPriceTokens, 0),
    len: excludesCache
      ? promptTokens + cacheReadTokens + cacheCreation5m + cacheCreation1h
      : promptTokens,
    cr: cacheReadTokens,
    cc: cacheCreation5m,
    cc1h: cacheCreation1h,
    img: imageInputTokens,
    img_o: imageOutputTokens,
    ai: audioInputTokens,
    ao: audioOutputTokens,
  };
};

export const buildTieredBillingBreakdown = (tier, params) => {
  const items = BILLING_PRICING_VARS.map((variable) => {
    const tokens = numberOrZero(params?.[variable.key]);
    const unitPriceUSD = numberOrZero(tier?.[variable.field]);
    return {
      ...variable,
      tokens,
      unitPriceUSD,
      amountUSD: (tokens / 1000000) * unitPriceUSD,
    };
  }).filter((item) => item.unitPriceUSD !== 0 && item.tokens !== 0);

  return {
    items,
    subtotalUSD: items.reduce((sum, item) => sum + item.amountUSD, 0),
  };
};
