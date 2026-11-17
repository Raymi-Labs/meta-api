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

// UI state only; never manufacture observations or bypass health permissions.
export function modelGroupHealthState(
  health,
  value,
  { rate = false, samples = 0 } = {},
) {
  if (!health.isAuthenticated) return 'anonymous';
  if (health.error) return 'error';
  if (health.data?.enabled === false) return 'disabled';
  const valid =
    value !== null &&
    value !== undefined &&
    value !== '' &&
    Number.isFinite(Number(value)) &&
    Number(value) >= 0 &&
    (!rate || Number(value) <= 100);
  if (valid) return 'ready';
  if (health.loading) return 'loading';
  if (Number(samples) > 0) return 'limited';
  return 'empty';
}
