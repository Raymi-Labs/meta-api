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

import { describe, expect, test } from 'bun:test';
import {
  buildChannelCategoryQuery,
  CHANNEL_CATEGORY_ALL,
  CHANNEL_DISPLAY_VENDORS,
  CHANNEL_CATEGORY_MIMO,
  channelTypeCategoryKey,
  resolveChannelCategoryGroupVendor,
} from './channelCategory';

describe('channel category query', () => {
  test('keeps protocol and vendor categories independent', () => {
    expect(channelTypeCategoryKey(1)).toBe('type:1');
    expect(CHANNEL_CATEGORY_MIMO).toBe('vendor:mimo');
  });

  test('only sends category outside tag mode', () => {
    expect(buildChannelCategoryQuery(CHANNEL_CATEGORY_ALL, false)).toBe('');
    expect(buildChannelCategoryQuery('type:1', false)).toBe(
      '&category=type%3A1',
    );
    expect(buildChannelCategoryQuery(CHANNEL_CATEGORY_MIMO, false)).toBe(
      '&category=vendor%3Amimo',
    );
    expect(buildChannelCategoryQuery(CHANNEL_CATEGORY_MIMO, true)).toBe('');
  });

  test('maps only explicit provider categories to configured group vendors', () => {
    const vendors = ['OpenAI', 'Claude', 'Gemini', 'Kimi', 'Grok', 'MiMo'];
    expect(resolveChannelCategoryGroupVendor('type:1', vendors)).toBe('OpenAI');
    expect(resolveChannelCategoryGroupVendor('type:14', vendors)).toBe(
      'Claude',
    );
    expect(resolveChannelCategoryGroupVendor('type:25', vendors)).toBe('Kimi');
    expect(
      resolveChannelCategoryGroupVendor(CHANNEL_CATEGORY_MIMO, vendors),
    ).toBe('MiMo');
    expect(resolveChannelCategoryGroupVendor('type:20', vendors)).toBe('');
    expect(resolveChannelCategoryGroupVendor('type:8', vendors)).toBe('');
  });

  test('falls back safely for missing aliases, all categories, and tag mode', () => {
    expect(resolveChannelCategoryGroupVendor('type:14', ['Anthropic'])).toBe(
      'Anthropic',
    );
    expect(resolveChannelCategoryGroupVendor('type:43', ['OpenAI'])).toBe('');
    expect(
      resolveChannelCategoryGroupVendor(CHANNEL_CATEGORY_ALL, ['OpenAI']),
    ).toBe('');
    expect(resolveChannelCategoryGroupVendor('type:1', ['OpenAI'], true)).toBe(
      '',
    );
  });
});

test('all display vendors support category queries and matching group filters', () => {
  for (const vendor of CHANNEL_DISPLAY_VENDORS) {
    const key = `vendor:${vendor.value}`;
    expect(buildChannelCategoryQuery(key, false)).toBe(
      `&category=vendor%3A${vendor.value}`,
    );
    expect(resolveChannelCategoryGroupVendor(key, vendor.groups)).toBe(
      vendor.groups[0],
    );
    expect(resolveChannelCategoryGroupVendor(key, vendor.groups, true)).toBe(
      '',
    );
  }
});

test('Inception category matches only available Inception groups and has its own icon', () => {
  const vendor = CHANNEL_DISPLAY_VENDORS.find(
    ({ value }) => value === 'inception',
  );
  expect(vendor.icon).toBe('Inception');
  expect(vendor.type).toBeUndefined();
  expect(
    resolveChannelCategoryGroupVendor('vendor:inception', [
      'Inception',
      'OpenAI',
    ]),
  ).toBe('Inception');
  expect(
    resolveChannelCategoryGroupVendor('vendor:inception', ['OpenAI']),
  ).toBe('');
  expect(
    resolveChannelCategoryGroupVendor('vendor:inception', ['Inception'], true),
  ).toBe('');
});
