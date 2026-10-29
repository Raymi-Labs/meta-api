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

export const USAGE_LOG_COLUMN_MODE_STORAGE_KEY = 'logs-table-column-mode-user';

export const readUsageLogColumnMode = (isAdminUser, storage) => {
  if (isAdminUser) return 'full';
  try {
    return storage?.getItem(USAGE_LOG_COLUMN_MODE_STORAGE_KEY) === 'full'
      ? 'full'
      : 'summary';
  } catch {
    return 'summary';
  }
};

export const getEffectiveVisibleColumns = (
  visibleColumns,
  mode,
  columnKeys,
  isAdminUser,
) => {
  const columns =
    visibleColumns && typeof visibleColumns === 'object' ? visibleColumns : {};
  if (isAdminUser || mode === 'full') return { ...columns };

  const summaryKeys = new Set([
    columnKeys.TIME,
    columnKeys.MODEL,
    columnKeys.TYPE,
    columnKeys.COST,
    columnKeys.USE_TIME,
    columnKeys.PROMPT,
    columnKeys.COMPLETION,
  ]);
  return Object.fromEntries(
    Object.entries(columns).map(([key, visible]) => [
      key,
      summaryKeys.has(key) ? visible : false,
    ]),
  );
};
