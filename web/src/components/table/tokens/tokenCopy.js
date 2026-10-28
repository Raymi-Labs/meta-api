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

/* Copy only keys fetched through the existing authorized detail endpoint. */
export async function collectTokenKeys(
  records,
  getFullKey,
  withName,
  isCurrent = () => true,
) {
  const lines = [];
  for (const record of records) {
    if (!isCurrent()) return null;
    const raw = await getFullKey(record);
    if (!isCurrent() || typeof raw !== 'string' || !raw || /[*•]/.test(raw))
      return null;
    const key = raw.startsWith('sk-') ? raw : `sk-${raw}`;
    lines.push(withName ? `${record.name || ''}    ${key}` : key);
  }
  return lines.length ? `${lines.join('\n')}\n` : null;
}
