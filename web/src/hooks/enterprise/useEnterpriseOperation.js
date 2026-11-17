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

import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { enterpriseRequest } from '../../helpers/enterprise';
import { createEnterpriseOperationStore } from '../../helpers/enterpriseOperation';
import { getStoredUser } from '../../helpers/storage';
import { showError, showSuccess } from '../../helpers/toast';

// Current authority is rechecked by the server. The account check also prevents
// a delayed result from closing a different account's form after navigation.
export function useEnterpriseOperation({
  scope,
  url,
  lookupBase,
  recoveryContext,
  onDone,
  onClose,
}) {
  const { t } = useTranslation();
  const actor = getStoredUser()?.id;
  const identity = `${actor}:${scope}`;
  const current = useRef(identity);
  current.current = identity;
  const mounted = useRef(false);
  const sending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState(null);
  const [error, setError] = useState('');
  const store = useMemo(
    () =>
      createEnterpriseOperationStore(
        {
          getItem: (key) => sessionStorage.getItem(key),
          setItem: (key, value) => sessionStorage.setItem(key, value),
          removeItem: (key) => sessionStorage.removeItem(key),
        },
        identity,
        () => crypto.randomUUID(),
      ),
    [identity],
  );
  const handlers = useRef({ onDone, onClose });
  handlers.current = { onDone, onClose };
  const active = () =>
    mounted.current &&
    current.current === identity &&
    getStoredUser()?.id === actor;
  const finish = (intent) => {
    if (!active()) return;
    try {
      store.clear(intent.key);
    } catch {
      /* a retained key can only replay */
    }
    setPending(null);
    showSuccess(t('已完成'));
    // A failed display refresh cannot turn a committed transaction into failure.
    try {
      Promise.resolve(handlers.current.onDone?.()).catch(() =>
        showError(t('已完成，余额待刷新')),
      );
    } catch {
      showError(t('已完成，余额待刷新'));
    }
    try {
      handlers.current.onClose?.();
    } catch {
      /* receipt remains successful */
    }
  };

  useEffect(() => {
    mounted.current = true;
    sending.current = false;
    setBusy(false);
    setError('');
    setPending(null);
    const abort = new AbortController();
    let intent;
    try {
      intent = store.read();
      setPending(intent);
    } catch {
      setError(t('无法恢复操作，请联系管理员核对'));
    }
    // Recovery is a single GET, with a finite timeout; no polling survives a modal.
    if (intent && lookupBase) {
      setBusy(true);
      enterpriseRequest(
        'get',
        `${lookupBase}/operations/${encodeURIComponent(intent.key)}`,
        undefined,
        undefined,
        { signal: abort.signal, timeout: 30000 },
      )
        .then((receipt) => {
          if (
            receipt?.protocol === 1 &&
            receipt.receipt &&
            !abort.signal.aborted
          )
            finish(intent);
        })
        .catch(() => {
          if (!abort.signal.aborted && active())
            setError(t('结果待确认，请查询或重试原操作'));
        })
        .finally(() => {
          if (!abort.signal.aborted && active()) setBusy(false);
        });
    }
    return () => {
      mounted.current = false;
      abort.abort();
    };
  }, [identity, store, lookupBase]);

  const submit = async (data) => {
    if (sending.current || busy || !active() || !actor) return;
    sending.current = true;
    setBusy(true);
    let intent;
    try {
      intent = store.begin(url, data, recoveryContext);
      if (!active()) return;
      setPending(intent);
      setError('');
      // Verify the recovery endpoint before any financial POST. A page opened
      // against an older server must not silently bypass the new contract.
      const lookup = await enterpriseRequest(
        'get',
        `${lookupBase}/operations/${encodeURIComponent(intent.key)}`,
        undefined,
        undefined,
        { timeout: 30000 },
      );
      if (!active()) return;
      if (lookup?.protocol !== 1)
        throw new Error('unsupported operation protocol');
      if (lookup.receipt) {
        finish(intent);
        return;
      }
      if (intent.queryOnly)
        throw new Error('legacy result requires reconciliation');
      const result = await enterpriseRequest(
        'post',
        intent.url,
        intent.data,
        undefined,
        { idempotencyKey: intent.key, timeout: 30000 },
      );
      // Fail closed against an old server ignoring the new header.
      if (!result?.operation_id) {
        store.queryOnly(intent.key);
        throw new Error('missing operation receipt');
      }
      finish(intent);
    } catch (e) {
      if (!active()) return;
      if (e.operationRejected && intent && !pending) {
        try {
          store.clear(intent.key);
          setPending(null);
          setError(e.message);
        } catch {
          setError(t('结果待确认，请查询或重试原操作'));
        }
      } else if (intent) {
        setError(t('结果待确认，请查询或重试原操作'));
      } else {
        setError(t('无法保存操作，请检查浏览器存储后重试'));
      }
    } finally {
      sending.current = false;
      if (active()) setBusy(false);
    }
  };
  return { submit, busy, pending, error, locked: busy || Boolean(pending) };
}
