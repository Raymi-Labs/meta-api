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

const ANNOUNCEMENT_KEY = 'notice_read_keys';
const CONTENT_KEY = 'notice_read_content';
const MUTED_KEY = 'notice_close_date';
const CHANGE_EVENT = 'newapi:notice-read-state';
const storageKeys = [ANNOUNCEMENT_KEY, CONTENT_KEY, MUTED_KEY];
const types = new Set(['default', 'ongoing', 'success', 'warning', 'error']);

// Keep the existing identifier so opening announcements on either shell shares
// the same browser-local history. This is not an account-level read receipt.
export const getAnnouncementKey = (item) =>
  `${typeof item?.publishDate === 'string' ? item.publishDate : ''}-${typeof item?.content === 'string' ? item.content.slice(0, 30) : ''}`;

export const normalizeAnnouncements = (value) =>
  Array.isArray(value)
    ? value
        .filter(
          (item) =>
            item &&
            typeof item === 'object' &&
            typeof item.content === 'string' &&
            item.content.trim(),
        )
        .map((item) => ({
          ...item,
          publishDate:
            typeof item.publishDate === 'string' ? item.publishDate : '',
          type: types.has(item.type) ? item.type : 'default',
          extra: typeof item.extra === 'string' ? item.extra : '',
        }))
    : [];

const validKeys = (value) =>
  Array.isArray(value)
    ? [...new Set(value.filter((key) => typeof key === 'string' && key))]
    : [];
const noticeText = (value) => (typeof value === 'string' ? value.trim() : '');

export const createNoticeReadStateStore = ({
  storage = () => globalThis.window?.localStorage,
  eventTarget = globalThis.window,
  now = () => new Date(),
} = {}) => {
  const memory = new Map();
  const unsaved = new Set();
  const listeners = new Set();
  const source = {};
  const read = (key) => {
    if (unsaved.has(key)) return memory.get(key) ?? null;
    try {
      const target = storage();
      if (target) {
        const value = target.getItem(key);
        memory.set(key, value);
        return value;
      }
    } catch {
      // Keep the current session usable when browser storage is unavailable.
    }
    return memory.get(key) ?? null;
  };
  const notify = () => {
    for (const listener of [...listeners]) listener();
  };
  const write = (key, value) => {
    memory.set(key, value);
    unsaved.add(key);
    let persisted = false;
    try {
      const target = storage();
      target?.setItem(key, value);
      persisted = target?.getItem(key) === value;
      if (persisted) unsaved.delete(key);
    } catch {
      // Readers still see this session's choice; false reports persistence failure.
    }
    notify();
    if (eventTarget?.dispatchEvent) {
      const EventType = eventTarget.Event || globalThis.Event;
      const event = new EventType(CHANGE_EVENT);
      Object.defineProperty(event, 'detail', {
        value: { source, key, value, persisted },
      });
      eventTarget.dispatchEvent(event);
    }
    return persisted;
  };
  const onLocalChange = ({ detail }) => {
    if (
      !detail ||
      detail.source === source ||
      !storageKeys.includes(detail.key) ||
      typeof detail.value !== 'string'
    )
      return;
    memory.set(detail.key, detail.value);
    if (detail.persisted) unsaved.delete(detail.key);
    else unsaved.add(detail.key);
    notify();
  };
  const onStorage = (event) => {
    if (event.key !== null && !storageKeys.includes(event.key)) return;
    if (event.storageArea) {
      try {
        if (event.storageArea !== storage()) return;
      } catch {
        return;
      }
    }
    for (const key of event.key === null ? storageKeys : [event.key]) {
      memory.delete(key);
      unsaved.delete(key);
    }
    // Readers inspect current storage, rather than a queued event's stale value.
    notify();
  };
  const getReadAnnouncementKeys = () => {
    try {
      return validKeys(JSON.parse(read(ANNOUNCEMENT_KEY) || '[]'));
    } catch {
      return [];
    }
  };
  return {
    getReadAnnouncementKeys,
    markAnnouncementKeysRead: (keys) => {
      const incoming = validKeys(keys);
      if (!incoming.length) return true;
      return write(
        ANNOUNCEMENT_KEY,
        JSON.stringify(validKeys([...getReadAnnouncementKeys(), ...incoming])),
      );
    },
    isNoticeContentUnread: (content) => {
      const text = noticeText(content);
      return Boolean(text && read(CONTENT_KEY) !== text);
    },
    markNoticeContentRead: (content) => {
      const text = noticeText(content);
      return text ? write(CONTENT_KEY, text) : true;
    },
    isNoticeMutedToday: () =>
      read(MUTED_KEY) === new Date(now()).toDateString(),
    muteNoticeToday: () => write(MUTED_KEY, new Date(now()).toDateString()),
    subscribeNoticeReadState: (listener) => {
      if (!listeners.size) {
        eventTarget?.addEventListener('storage', onStorage);
        eventTarget?.addEventListener(CHANGE_EVENT, onLocalChange);
      }
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
        if (!listeners.size) {
          eventTarget?.removeEventListener('storage', onStorage);
          eventTarget?.removeEventListener(CHANGE_EVENT, onLocalChange);
        }
      };
    },
  };
};

const noticeReadState = createNoticeReadStateStore();
export const {
  getReadAnnouncementKeys,
  markAnnouncementKeysRead,
  isNoticeContentUnread,
  markNoticeContentRead,
  subscribeNoticeReadState,
  isNoticeMutedToday,
  muteNoticeToday,
} = noticeReadState;
