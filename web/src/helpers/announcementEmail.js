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

export function createAnnouncementID(source = globalThis.crypto) {
  if (source.randomUUID) return source.randomUUID();
  // getRandomValues is also available on HTTP intranet installations.
  const bytes = source.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function parseAnnouncementDraft(raw) {
  const items = raw ? JSON.parse(raw) : [];
  if (!Array.isArray(items)) throw new Error('Invalid announcement list');
  // Preserve the legacy editor's identities for missing, zero and empty IDs.
  return items.map((item, index) => ({ ...item, id: item.id || index + 1 }));
}

export function announcementDraftReducer(state, action) {
  switch (action.type) {
    case 'receive':
      return state.dirty ? state : { ...state, items: action.items };
    case 'edit':
      return { items: action.items, dirty: true, revision: state.revision + 1 };
    case 'saved':
      return action.revision === state.revision
        ? { ...state, dirty: false }
        : state;
    default:
      return state;
  }
}

export async function publishAnnouncements(
  api,
  items,
  emailSync,
  fallbackMessage,
) {
  const { data } = await api.put('/api/option/', {
    key: 'console_setting.announcements',
    value: JSON.stringify(items),
    email_sync: emailSync === true,
  });
  if (!data?.success) throw new Error(data?.message || fallbackMessage);
  return Number(data.data?.email_jobs_queued) || 0;
}

export async function fetchAnnouncementEmailJobs(
  api,
  page,
  pageSize,
  fallbackMessage,
) {
  const { data } = await api.get('/api/option/announcement-email/jobs', {
    params: { p: page, page_size: pageSize },
  });
  if (!data?.success) throw new Error(data?.message || fallbackMessage);
  return data.data;
}
