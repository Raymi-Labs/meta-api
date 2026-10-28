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

export const normalizeUsageLogFilters = (values) => {
  const range = values.dateRange;
  const start = Array.isArray(range) && range.length === 2 ? range[0] : null;
  const end = Array.isArray(range) && range.length === 2 ? range[1] : null;
  const timestamp = (value) =>
    value == null || value === ''
      ? NaN
      : Math.floor(new Date(value).getTime() / 1000);
  const result = {
    start_timestamp: start instanceof Date ? new Date(start.getTime()) : start,
    end_timestamp: end instanceof Date ? new Date(end.getTime()) : end,
    startTimestamp: timestamp(start),
    endTimestamp: timestamp(end),
    logType: Number(values.logType || 0),
    channel_ids: Array.isArray(values.channel_ids)
      ? values.channel_ids.join(',')
      : String(values.channel_ids || ''),
  };
  for (const key of [
    'username',
    'token_name',
    'model_name',
    'channel',
    'channel_keyword',
    'group_vendor',
    'group',
    'request_id',
  ])
    result[key] = values[key] || '';
  return result;
};
