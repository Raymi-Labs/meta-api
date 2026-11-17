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

const operationPrefix = 'enterprise-operation:v1:';

// One persisted intent per actor + form. A transport error never rotates the key.
export function createEnterpriseOperationStore(storage, scope, makeKey) {
  const storageKey = `${operationPrefix}${scope}`;
  let pending = null;
  const read = () => {
    const raw = storage.getItem(storageKey);
    pending = raw ? JSON.parse(raw) : null;
    if (
      pending &&
      (typeof pending.key !== 'string' ||
        !pending.key ||
        typeof pending.url !== 'string' ||
        !pending.url ||
        !pending.data ||
        typeof pending.data !== 'object' ||
        Array.isArray(pending.data))
    ) {
      throw new Error('无法恢复操作，请联系管理员核对');
    }
    return pending;
  };
  return {
    read,
    begin(url, data, context) {
      const old = read();
      if (old) {
        if (old.url !== url) throw new Error('无法恢复操作，请联系管理员核对');
        return old;
      }
      const value = {
        key: makeKey(),
        url,
        data,
        ...(context ? { context } : {}),
      };
      // Fail before sending if the browser cannot retain the recovery key.
      storage.setItem(storageKey, JSON.stringify(value));
      pending = value;
      return value;
    },
    queryOnly(key) {
      const old = read();
      if (old?.key === key) {
        old.queryOnly = true;
        storage.setItem(storageKey, JSON.stringify(old));
      }
    },
    clear(key) {
      if (read()?.key === key) storage.removeItem(storageKey);
      pending = null;
    },
  };
}

export function clearEnterpriseOperationDrafts(storage) {
  try {
    for (const key of Object.keys(storage)) {
      if (key.startsWith(operationPrefix)) storage.removeItem(key);
    }
  } catch {
    /* Authentication cleanup must work with unavailable storage. */
  }
}

// A resolved reservation can leave the server's review list before its response
// reaches the browser. Keep its recovery action reachable from that same page.
// Context is display-only: URLs are derived and all authority stays server-side.
export function listPendingEnterpriseReservations(storage, actorID) {
  if (!Number.isSafeInteger(actorID) || actorID <= 0) return [];
  const prefix = `${operationPrefix}${actorID}:reservation:`;
  const reservations = [];
  for (const storageKey of Object.keys(storage)) {
    if (!storageKey.startsWith(prefix)) continue;
    try {
      const intent = JSON.parse(storage.getItem(storageKey));
      const reservation = intent?.context?.reservation;
      if (
        !reservation ||
        !Number.isSafeInteger(reservation.id) ||
        reservation.id <= 0 ||
        !Number.isSafeInteger(reservation.enterprise_id) ||
        reservation.enterprise_id <= 0 ||
        !Number.isSafeInteger(reservation.quota) ||
        reservation.quota < 0 ||
        storageKey !== `${prefix}${reservation.id}` ||
        intent.url !==
          `/api/enterprise/admin/reservations/${reservation.id}/resolve` ||
        typeof intent.key !== 'string' ||
        !/^[A-Za-z0-9_-]{16,128}$/.test(intent.key) ||
        ['enterprise_name', 'member_name', 'request_id'].some(
          (field) =>
            reservation[field] !== undefined &&
            typeof reservation[field] !== 'string',
        ) ||
        typeof intent.data?.settle !== 'boolean' ||
        !Number.isSafeInteger(intent.data.amount) ||
        intent.data.amount < 0 ||
        intent.data.amount > 1e15
      )
        continue;
      reservations.push(reservation);
    } catch {
      // One corrupt draft must not hide other recoverable operations.
    }
  }
  return reservations;
}
