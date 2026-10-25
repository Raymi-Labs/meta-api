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

export const createPublicDocsAPI = (request = globalThis.fetch) => ({
  async get(path, { signal, timeout = 15000 } = {}) {
    if (!['/api/status', '/api/pricing'].includes(path))
      throw new Error('Unsupported documentation endpoint');
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal?.addEventListener('abort', abort, { once: true });
    if (signal?.aborted) controller.abort();
    const timer = setTimeout(abort, timeout);
    try {
      const response = await request(path, {
        credentials: 'omit',
        signal: controller.signal,
      });
      if (!response.ok)
        throw new Error('Public documentation data unavailable');
      return { data: await response.json() };
    } finally {
      clearTimeout(timer);
      signal?.removeEventListener('abort', abort);
    }
  },
});
export const publicDocsAPI = createPublicDocsAPI();
