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

export const CHANNEL_CATEGORY_ALL = 'all';
export const CHANNEL_CATEGORY_MIMO = 'vendor:mimo';

export const channelTypeCategoryKey = (channelType) => `type:${channelType}`;

export const CHANNEL_DISPLAY_VENDORS = [
  { value: 'deepseek', label: 'DeepSeek', type: 43, groups: ['DeepSeek'] },
  { value: 'openai', label: 'OpenAI', type: 1, groups: ['OpenAI'] },
  {
    value: 'anthropic',
    label: 'Claude',
    type: 14,
    groups: ['Claude', 'Anthropic'],
  },
  { value: 'google', label: 'Gemini', type: 24, groups: ['Gemini', 'Google'] },
  { value: 'qwen', label: 'Qwen', type: 17, groups: ['Qwen'] },
  { value: 'moonshot', label: 'Kimi', type: 25, groups: ['Kimi', 'Moonshot'] },
  { value: 'zhipu', label: 'GLM', type: 26, groups: ['GLM'] },
  { value: 'xai', label: 'Grok', type: 48, groups: ['Grok', 'xAI'] },
  { value: 'minimax', label: 'MiniMax', type: 35, groups: ['MiniMax'] },
  { value: 'mistral', label: 'Mistral', type: 42, groups: ['Mistral'] },
  {
    value: 'mimo',
    label: '小米 MiMo',
    icon: "Xiaomi.color='#FF6900'",
    groups: ['MiMo'],
  },
  {
    value: 'inception',
    label: 'Inception',
    icon: 'Inception',
    groups: ['Inception'],
  },
];

const CATEGORY_GROUP_VENDOR_CANDIDATES = new Map([
  ...CHANNEL_DISPLAY_VENDORS.map((vendor) => [
    `vendor:${vendor.value}`,
    vendor.groups,
  ]),
  [channelTypeCategoryKey(1), ['OpenAI']],
  [channelTypeCategoryKey(3), ['OpenAI']],
  [channelTypeCategoryKey(57), ['OpenAI']],
  [channelTypeCategoryKey(14), ['Claude', 'Anthropic']],
  [channelTypeCategoryKey(33), ['Claude', 'Anthropic']],
  [channelTypeCategoryKey(43), ['DeepSeek']],
  [channelTypeCategoryKey(24), ['Gemini', 'Google']],
  [channelTypeCategoryKey(11), ['Gemini', 'Google']],
  [channelTypeCategoryKey(25), ['Kimi', 'Moonshot']],
  [channelTypeCategoryKey(48), ['Grok', 'xAI']],
  [channelTypeCategoryKey(17), ['Qwen']],
  [channelTypeCategoryKey(16), ['GLM']],
  [channelTypeCategoryKey(26), ['GLM']],
  [channelTypeCategoryKey(15), ['Baidu']],
  [channelTypeCategoryKey(46), ['Baidu']],
  [channelTypeCategoryKey(18), ['Spark']],
  [channelTypeCategoryKey(23), ['Hunyuan']],
  [channelTypeCategoryKey(31), ['Yi']],
  [channelTypeCategoryKey(35), ['MiniMax']],
  [channelTypeCategoryKey(34), ['Cohere']],
  [channelTypeCategoryKey(38), ['Jina']],
  [channelTypeCategoryKey(42), ['Mistral']],
  [channelTypeCategoryKey(27), ['Perplexity']],
  [channelTypeCategoryKey(19), ['360']],
  [channelTypeCategoryKey(36), ['Suno']],
  [channelTypeCategoryKey(49), ['Coze']],
  [channelTypeCategoryKey(50), ['Kling']],
  [channelTypeCategoryKey(51), ['Jimeng']],
  [channelTypeCategoryKey(52), ['Vidu']],
]);

export const resolveChannelCategoryGroupVendor = (
  categoryKey,
  availableVendors = [],
  enableTagMode = false,
) => {
  if (enableTagMode || !categoryKey || categoryKey === CHANNEL_CATEGORY_ALL) {
    return '';
  }
  const availableVendorSet = new Set(availableVendors);
  const candidates = CATEGORY_GROUP_VENDOR_CANDIDATES.get(categoryKey) || [];
  return candidates.find((vendor) => availableVendorSet.has(vendor)) || '';
};

export const buildChannelCategoryQuery = (categoryKey, enableTagMode) => {
  if (enableTagMode || !categoryKey || categoryKey === CHANNEL_CATEGORY_ALL) {
    return '';
  }
  return `&category=${encodeURIComponent(categoryKey)}`;
};
