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

// Keep the mutation result separate from list loading: a failed sync must never
// replace the user's current model list with an empty array.
export async function requestModelCapabilitySync(api) {
  const response = await api.post('/api/models/sync_capabilities');
  if (response?.data?.success !== true) {
    const error = new Error('Model capability sync failed');
    error.response = response;
    throw error;
  }
  const data = response.data.data || {};
  return {
    created:
      Number.isSafeInteger(data.created) && data.created >= 0
        ? data.created
        : 0,
    updated:
      Number.isSafeInteger(data.updated) && data.updated >= 0
        ? data.updated
        : 0,
  };
}
