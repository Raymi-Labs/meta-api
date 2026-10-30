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

export function dashboardMetricValues(quota, tokens, requests, cacheRate) {
  const valid = (value) =>
    typeof value === 'number' && Number.isFinite(value) && value >= 0;
  const unitPrice =
    valid(quota) && valid(tokens) && tokens > 0
      ? (quota / tokens) * 1000000
      : null;
  return {
    quota: valid(quota) ? quota : null,
    tokens: valid(tokens) ? tokens : null,
    requests: valid(requests) ? requests : null,
    cacheRate: valid(cacheRate) && cacheRate <= 1 ? cacheRate : null,
    unitPrice: valid(unitPrice) ? unitPrice : null,
  };
}

export function dashboardCacheStatUrl(inputs, isAdmin) {
  const query = new URLSearchParams({
    type: '2',
    start_timestamp: String(
      Math.floor(new Date(inputs.start_timestamp).getTime() / 1000),
    ),
    end_timestamp: String(
      Math.floor(new Date(inputs.end_timestamp).getTime() / 1000),
    ),
  });
  if (isAdmin && inputs.username)
    query.set('username', `name:${inputs.username}`);
  return `${isAdmin ? '/api/log/stat' : '/api/log/self/stat'}?${query}`;
}
