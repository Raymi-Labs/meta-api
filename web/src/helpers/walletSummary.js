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
export function validWalletSummary(data, userId) {
  const usage = data?.usage;
  return (
    data?.user_id === userId &&
    Number.isSafeInteger(data.balance) &&
    Number.isSafeInteger(data.as_of) &&
    data.as_of > 0 &&
    Number.isSafeInteger(usage?.start_timestamp) &&
    usage?.end_timestamp === data.as_of &&
    usage.end_timestamp - usage.start_timestamp === 86400 &&
    ['ok', 'not_recorded', 'unavailable'].includes(usage.status) &&
    (usage.status === 'ok'
      ? Number.isSafeInteger(usage.net_quota)
      : usage.net_quota === null)
  );
}

export function walletEstimate(data) {
  if (!data || data.usage.status !== 'ok') return { kind: 'unknown' };
  if (data.balance <= 0) return { kind: 'empty' };
  if (data.usage.net_quota <= 0) return { kind: 'idle' };
  const days = data.balance / data.usage.net_quota;
  return { kind: days < 1 ? 'urgent' : days < 3 ? 'low' : 'normal', days };
}
