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

// Only visible pages poll. Slow requests never overlap; disposal removes every trigger.
export function startVisibleRefresh(
  refresh,
  { interval = 15000, document: doc = document, window: win = window } = {},
) {
  let disposed = false;
  let running = false;
  const run = async () => {
    if (disposed || running || doc.visibilityState !== 'visible') return;
    running = true;
    try {
      await refresh();
    } catch {
      // The caller owns presentation of request failures. Keep future retries alive.
    } finally {
      running = false;
    }
  };
  const timer = win.setInterval(run, interval);
  doc.addEventListener('visibilitychange', run);
  win.addEventListener('focus', run);
  return () => {
    disposed = true;
    win.clearInterval(timer);
    doc.removeEventListener('visibilitychange', run);
    win.removeEventListener('focus', run);
  };
}
