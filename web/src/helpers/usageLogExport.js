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

// Shared by the list and export so every role uses the same selected filters.
export const buildUsageLogQuery = (values, admin) => {
  const params = {
    type: values.logType,
    token_name: values.token_name,
    model_name: values.model_name,
    start_timestamp: values.startTimestamp,
    end_timestamp: values.endTimestamp,
    group: values.group,
    request_id: values.request_id,
  };
  if (admin) {
    for (const key of [
      'username',
      'channel',
      'channel_ids',
      'channel_keyword',
      'group_vendor',
    ]) {
      params[key] = values[key];
    }
  }
  return params;
};

export const readLogExportError = async (error, fallback) => {
  try {
    const data = error?.response?.data;
    const payload = data instanceof Blob ? JSON.parse(await data.text()) : data;
    return payload?.message || fallback;
  } catch {
    return fallback;
  }
};
