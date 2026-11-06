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
import { getPlaygroundOwner } from './configStorage';

export async function openSessionStore(owner) {
  const assertOwner = () => {
    if (getPlaygroundOwner() !== owner)
      throw new Error('登录状态已变化，请刷新页面');
  };
  assertOwner();
  const db = await new Promise((resolve, reject) => {
    const req = indexedDB.open('meta-api-playground-conversations', 1);
    req.onupgradeneeded = () => req.result.createObjectStore('accounts');
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
    req.onblocked = () => reject(new Error('本地存储被其他页面阻塞'));
  });
  let closed = false;
  db.onversionchange = () => {
    closed = true;
    db.close();
  };
  let tail = Promise.resolve();
  const transact = (mode, value) =>
    new Promise((resolve, reject) => {
      try {
        assertOwner();
        if (closed) throw new Error('本地存储已关闭');
        const tx = db.transaction('accounts', mode);
        const store = tx.objectStore('accounts');
        const req =
          mode === 'readonly' ? store.get(owner) : store.put(value, owner);
        tx.oncomplete = () => resolve(req.result);
        tx.onabort = () => reject(tx.error || new Error('本地保存失败'));
        tx.onerror = () => reject(tx.error);
      } catch (e) {
        reject(e);
      }
    });
  return {
    read: () => transact('readonly'),
    write(value) {
      // Capture the exact snapshot now; serialize transactions to avoid stale writes.
      const snapshot = structuredClone(value);
      const result = tail
        .catch(() => {})
        .then(() => transact('readwrite', snapshot));
      tail = result;
      return result;
    },
    close() {
      return tail
        .catch(() => {})
        .finally(() => {
          closed = true;
          db.close();
        });
    },
  };
}
