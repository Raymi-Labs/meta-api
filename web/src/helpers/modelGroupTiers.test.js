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
import { groupTierRows } from './modelGroupTiers';

const translate = (value) => value;

describe('groupTierRows', () => {
  test('matches xAI regional official tiers in the default region', () => {
    const model = {
      billing_mode: 'tiered_expr',
      quota_type: 0,
      official_pricing: {
        currency: 'USD',
        tiers: [
          {
            label: 'us-east-1 · Standard · ≤ 200000 input tokens',
            unit: 'million_tokens',
            input: 2,
          },
          {
            label: 'us-east-1 · Standard · > 200000 input tokens',
            unit: 'million_tokens',
            input: 4,
          },
          {
            label: 'us-central-1 · Standard · ≤ 200000 input tokens',
            unit: 'million_tokens',
            input: 2.2,
          },
          {
            label: 'us-central-1 · Standard · > 200000 input tokens',
            unit: 'million_tokens',
            input: 4.4,
          },
        ],
      },
    };
    const tiers = [
      {
        label: 'standard',
        conditions: [{ var: 'len', op: '<=', value: 200000 }],
      },
      { label: 'long', conditions: [] },
    ];

    const rows = groupTierRows(model, tiers, () => ({ items: [] }), translate);

    expect(rows.map((row) => row.officialTier?.label)).toEqual([
      'us-east-1 · Standard · ≤ 200000 input tokens',
      'us-east-1 · Standard · > 200000 input tokens',
    ]);
    expect(rows.every((row) => !row.comparisonUnavailable)).toBe(true);
  });
});
