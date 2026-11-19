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

// Decimal arithmetic stays exact until the final integer quota. This avoids
// floating point drift for micro amounts and never accepts a negative as zero.
export function parseEnterpriseQuota(
  value,
  { raw = false, quotaPerUnit = 1, rate = 1, type = 'USD' } = {},
) {
  const fraction = (input) => {
    const text = String(input ?? '').trim();
    if (!/^\d+(\.\d+)?$/.test(text) || text.length > 64) return null;
    const [whole, part = ''] = text.split('.');
    return [BigInt(whole + part), 10n ** BigInt(part.length)];
  };
  const amount = fraction(value);
  if (!amount) return null;
  if (raw || type === 'TOKENS') {
    if (amount[0] % amount[1]) return null;
    const result = amount[0] / amount[1];
    return result <= 1000000000000000n ? Number(result) : null;
  }
  const unit = fraction(quotaPerUnit);
  const currency = fraction(type === 'USD' ? 1 : rate);
  if (!unit || !currency || unit[0] <= 0n || currency[0] <= 0n) return null;
  const numerator = amount[0] * unit[0] * currency[1];
  const denominator = amount[1] * unit[1] * currency[0];
  const result = (numerator * 2n + denominator) / (denominator * 2n);
  return result <= 1000000000000000n ? Number(result) : null;
}

// Called only when two decimal places would hide a nonzero converted balance.
export function formatSmallEnterpriseBalance(value) {
  const fixed = value.toFixed(8);
  return Number(fixed) === 0 ? value.toExponential(2) : fixed.replace(/0+$/, '');
}
