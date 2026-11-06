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
export function playgroundGroupOptions(groups) {
  return groups.map((group) => ({ value: group.value, label: group.value }));
}

export function playgroundDefaultGroup(groups, saved) {
  if (groups.some((group) => group.value === saved)) return saved;
  return (
    groups.find((group) => group.value === '福利')?.value ||
    groups[0]?.value ||
    ''
  );
}

export function compactPrice(value) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0)
    return null;
  if (value === 0) return '0';
  if (value < 0.01) return Number(value.toPrecision(3)).toString();
  return Number(value.toFixed(2)).toString();
}

export function playgroundMoney(raw, status, displayType = 'CUSTOM') {
  if (typeof raw !== 'number' || !Number.isFinite(raw) || raw < 0) return null;
  const customRate = Number(status?.custom_currency_exchange_rate);
  const rechargeRate = Number(status?.price);
  const custom =
    displayType === 'CUSTOM' && Number.isFinite(customRate) && customRate > 0;
  const symbol = custom ? status?.custom_currency_symbol || '⚡' : '$';
  const amount = raw * (custom ? customRate : 1);
  return {
    primary: `${symbol}${compactPrice(amount)}`,
    cny:
      Number.isFinite(rechargeRate) && rechargeRate > 0
        ? `≈¥${compactPrice(raw * rechargeRate)}`
        : null,
  };
}

export function readPlaygroundCurrency() {
  try {
    return {
      status: JSON.parse(localStorage.getItem('status') || '{}'),
      displayType: localStorage.getItem('quota_display_type') || 'USD',
    };
  } catch {
    return { status: {}, displayType: 'USD' };
  }
}
