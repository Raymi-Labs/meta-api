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

import { getUptimeRate } from '../../group-health/healthUtils.js';

export const TOKEN_GROUP_OTHER_VENDOR = '__other__';

// Filter candidate display only; never mutate permissions or selected routes.
export const filterTokenGroupsByHealth = (groups, health, selected = []) => {
  const data = health?.data;
  if (
    health?.error ||
    !data ||
    data.enabled === false ||
    data.collection_incomplete
  )
    return groups;
  const preserved = new Set(selected);
  const unavailable = new Set(
    (data.groups || [])
      .filter((stats) => {
        const count = stats.uptime_sample_count ?? stats.request_count;
        return (
          Number.isFinite(count) && count > 0 && getUptimeRate(stats) === 0
        );
      })
      .map((stats) => stats.group),
  );
  return groups.filter(
    ({ value }) => preserved.has(value) || !unavailable.has(value),
  );
};

const TOKEN_GROUP_PRICE_LABELS = {
  企业级: '与官方价格一致',
  企业: '与官方价格一致',
  优选: '比官方价格低20%',
  优质: '比官方价格低20%',
  福利: '比官方价格低40%',
};

export const tokenGroupPriceLabel = (groupName) =>
  Object.hasOwn(TOKEN_GROUP_PRICE_LABELS, groupName)
    ? TOKEN_GROUP_PRICE_LABELS[groupName]
    : '';

const TOKEN_GROUP_PRIORITY = ['企业级', '优选', '福利'];

// Candidate display only; selected groups keep the user's routing order.
export const prioritizeTokenGroups = (groups = []) => {
  const rank = ({ value }) => {
    const index = TOKEN_GROUP_PRIORITY.indexOf(value);
    return index === -1 ? TOKEN_GROUP_PRIORITY.length : index;
  };
  return [...groups].sort((left, right) => rank(left) - rank(right));
};

// Presentation only: no mutation of the permission catalog or routing order.
export const sortTokenGroupsByRatio = (groups = []) => {
  const validRatio = (group) =>
    group.value !== 'auto' &&
    typeof group.ratio === 'number' &&
    Number.isFinite(group.ratio) &&
    group.ratio >= 0;
  return [...groups].sort((left, right) => {
    const leftValid = validRatio(left);
    const rightValid = validRatio(right);
    if (leftValid && rightValid) return right.ratio - left.ratio;
    if (leftValid !== rightValid) return leftValid ? -1 : 1;
    return 0;
  });
};

const TOKEN_GROUP_VENDOR_ORDER = [
  'OpenAI',
  'Claude',
  'Anthropic',
  'Gemini',
  'Google',
  'Grok',
  'xAI',
  'Kimi',
  'Moonshot',
  'DeepSeek',
  'MiMo',
  'Inception',
  'Qwen',
  'GLM',
  'Baidu',
  'Spark',
  'Hunyuan',
  'Yi',
  'MiniMax',
  'Cohere',
  'Jina',
  'Mistral',
  'Perplexity',
  '360',
  'Suno',
  'Coze',
  'Kling',
  'Jimeng',
  'Vidu',
];

const TOKEN_GROUP_TIER_ORDER = ['企业专属', '优质', '标准', '特价', '限时福利'];

const TOKEN_GROUP_OTHER_ORDER = [
  'auto',
  'default',
  '官方渠道',
  '第三方渠道',
  '企业专属',
  '分销商',
];

const CANONICAL_VENDOR_NAMES = new Map([
  ['openai', 'OpenAI'],
  ['claude', 'Claude'],
  ['anthropic', 'Anthropic'],
  ['gemini', 'Gemini'],
  ['google', 'Google'],
  ['grok', 'Grok'],
  ['xai', 'xAI'],
  ['kimi', 'Kimi'],
  ['moonshot', 'Moonshot'],
  ['deepseek', 'DeepSeek'],
  ['mimo', 'MiMo'],
  ['xiaomi', 'MiMo'],
  ['inception', 'Inception'],
  ['inception labs', 'Inception'],
  ['inceptionlabs', 'Inception'],
  ['mercury', 'Inception'],
  ['qwen', 'Qwen'],
  ['alibaba', 'Qwen'],
  ['阿里巴巴', 'Qwen'],
  ['阿里通义千问', 'Qwen'],
  ['通义千问', 'Qwen'],
  ['glm', 'GLM'],
  ['zhipu', 'GLM'],
  ['智谱', 'GLM'],
  ['baidu', 'Baidu'],
  ['百度', 'Baidu'],
  ['spark', 'Spark'],
  ['讯飞', 'Spark'],
  ['hunyuan', 'Hunyuan'],
  ['腾讯', 'Hunyuan'],
  ['yi', 'Yi'],
  ['零一万物', 'Yi'],
  ['minimax', 'MiniMax'],
  ['cohere', 'Cohere'],
  ['jina', 'Jina'],
  ['mistral', 'Mistral'],
  ['perplexity', 'Perplexity'],
  ['360', '360'],
  ['suno', 'Suno'],
  ['coze', 'Coze'],
  ['kling', 'Kling'],
  ['可灵', 'Kling'],
  ['jimeng', 'Jimeng'],
  ['即梦', 'Jimeng'],
  ['vidu', 'Vidu'],
]);

const compareByKnownOrder = (left, right, orderedValues) => {
  const leftIndex = orderedValues.indexOf(left);
  const rightIndex = orderedValues.indexOf(right);
  if (leftIndex >= 0 && rightIndex >= 0) return leftIndex - rightIndex;
  if (leftIndex >= 0) return -1;
  if (rightIndex >= 0) return 1;
  return String(left).localeCompare(String(right), 'zh-Hans');
};

const resolveTokenGroupTier = (groupName) =>
  TOKEN_GROUP_TIER_ORDER.find((tier) => String(groupName).includes(tier)) || '';

const isThirdPartyGroup = (groupName) =>
  /第三方|third[\s_-]*party/i.test(String(groupName));

const normalizeVendorName = (value) => {
  const normalized = String(value || '').trim();
  if (!normalized) return '';
  return CANONICAL_VENDOR_NAMES.get(normalized.toLowerCase()) || normalized;
};

export const resolveTokenGroupVendor = (groupName) => {
  const normalizedGroupName = String(groupName || '').trim();
  if (!normalizedGroupName) return '';

  const separatorIndex = normalizedGroupName.indexOf('·');
  if (separatorIndex > 0) {
    return normalizeVendorName(normalizedGroupName.slice(0, separatorIndex));
  }

  const canonicalVendor = CANONICAL_VENDOR_NAMES.get(
    normalizedGroupName.toLowerCase(),
  );
  return canonicalVendor || TOKEN_GROUP_OTHER_VENDOR;
};

export const buildTokenGroupVendorOptions = (
  groups = [],
  otherLabel = '多厂商通用',
) => {
  const vendorCounts = new Map();

  groups.forEach((group) => {
    const vendor = resolveTokenGroupVendor(group?.value);
    if (!vendor) return;
    vendorCounts.set(vendor, (vendorCounts.get(vendor) || 0) + 1);
  });

  return Array.from(vendorCounts.entries())
    .sort(([leftVendor], [rightVendor]) => {
      // 仅调整供应商入口的展示，不改变实际分组排序、标识或默认选择。
      if (leftVendor === rightVendor) return 0;
      if (leftVendor === TOKEN_GROUP_OTHER_VENDOR) return -1;
      if (rightVendor === TOKEN_GROUP_OTHER_VENDOR) return 1;
      if (leftVendor === 'Gemini') return 1;
      if (rightVendor === 'Gemini') return -1;
      return compareByKnownOrder(
        leftVendor,
        rightVendor,
        TOKEN_GROUP_VENDOR_ORDER,
      );
    })
    .map(([vendor, count]) => ({
      value: vendor,
      label: `${vendor === TOKEN_GROUP_OTHER_VENDOR ? otherLabel : vendor} · ${count}`,
    }));
};

export const sortTokenGroupOptions = (groups = []) =>
  [...groups].sort((left, right) => {
    const leftName = String(left?.value || left?.label || '').trim();
    const rightName = String(right?.value || right?.label || '').trim();
    const leftVendor = resolveTokenGroupVendor(leftName);
    const rightVendor = resolveTokenGroupVendor(rightName);

    const vendorOrder = compareByKnownOrder(
      leftVendor,
      rightVendor,
      TOKEN_GROUP_VENDOR_ORDER,
    );
    if (vendorOrder !== 0) return vendorOrder;

    if (leftVendor === TOKEN_GROUP_OTHER_VENDOR) {
      const otherOrder = compareByKnownOrder(
        leftName,
        rightName,
        TOKEN_GROUP_OTHER_ORDER,
      );
      if (otherOrder !== 0) return otherOrder;
    } else {
      const tierOrder = compareByKnownOrder(
        resolveTokenGroupTier(leftName),
        resolveTokenGroupTier(rightName),
        TOKEN_GROUP_TIER_ORDER,
      );
      if (tierOrder !== 0) return tierOrder;

      const sourceOrder =
        Number(isThirdPartyGroup(leftName)) -
        Number(isThirdPartyGroup(rightName));
      if (sourceOrder !== 0) return sourceOrder;
    }

    return leftName.localeCompare(rightName, 'zh-Hans');
  });

export const filterTokenGroupsByVendor = (
  groups = [],
  vendor = '',
  includeCommonGroups = false,
) => {
  if (!vendor) return [];
  return groups.filter((group) => {
    const groupVendor = resolveTokenGroupVendor(group?.value);
    return (
      groupVendor === vendor ||
      (includeCommonGroups &&
        vendor !== TOKEN_GROUP_OTHER_VENDOR &&
        groupVendor === TOKEN_GROUP_OTHER_VENDOR)
    );
  });
};

export const shouldClearTokenGroupForVendor = (groupName, vendor) => {
  const normalizedGroupName = String(groupName || '').trim();
  if (!normalizedGroupName) return false;
  return resolveTokenGroupVendor(normalizedGroupName) !== vendor;
};

export const formatTokenGroupSelectedLabel = (option) => {
  const groupName = String(option?.value || option?.label || '').trim();
  if (!groupName) return '';

  const ratio = option?.ratio;
  if (ratio === undefined || ratio === null || ratio === '') {
    return groupName;
  }

  const ratioText =
    typeof ratio === 'number' || /^\d+(\.\d+)?$/.test(String(ratio))
      ? `${ratio}x`
      : String(ratio);
  return `${groupName} · ${ratioText}`;
};
