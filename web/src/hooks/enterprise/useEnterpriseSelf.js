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

import { useCallback, useContext, useEffect, useSyncExternalStore } from 'react';
import { StatusContext } from '../../context/Status';
import { API } from '../../helpers/apiCore';
import { getUserIdFromLocalStorage } from '../../helpers/storage';

// 登录账号的企业身份快照，所有组件共用同一份，避免侧栏、顶栏、令牌弹窗各自请求。
// 功能开关关闭或未登录时不发请求。
let snapshot = { loading: false, loaded: false, userId: 0, data: null };
const listeners = new Set();
let requestSeq = 0;

const emit = (next) => {
  snapshot = next;
  listeners.forEach((listener) => listener());
};
const subscribe = (listener) => {
  listeners.add(listener);
  return () => listeners.delete(listener);
};
const getSnapshot = () => snapshot;

const currentUserId = () => {
  const id = Number(getUserIdFromLocalStorage());
  return Number.isFinite(id) && id > 0 ? id : 0;
};

export async function refreshEnterpriseSelf() {
  const userId = currentUserId();
  const seq = ++requestSeq;
  if (userId <= 0) {
    emit({ loading: false, loaded: false, userId: 0, data: null });
    return;
  }
  emit({ ...snapshot, loading: true, userId });
  let data = null;
  try {
    const res = await API.get('/api/enterprise/self', {
      skipGlobalLoading: true,
      skipErrorHandler: true,
      disableDuplicate: true,
    });
    if (res.data?.success) data = res.data.data || null;
  } catch {
    // 查询失败按未加入处理，不影响控制台其余功能。
  }
  if (seq !== requestSeq) return;
  emit({ loading: false, loaded: true, userId, data });
}

export function useEnterpriseSelf() {
  const [statusState] = useContext(StatusContext);
  const enabled = statusState?.status?.enterprise_enabled === true;
  const state = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const userId = currentUserId();
  const matches = state.userId === userId;
  const needsFetch = enabled && userId > 0 && (!matches || !state.loaded);
  const busy = matches && state.loading;

  useEffect(() => {
    if (needsFetch && !busy) refreshEnterpriseSelf();
  }, [needsFetch, busy]);

  const refresh = useCallback(() => refreshEnterpriseSelf(), []);
  const data = enabled && matches ? state.data : null;
  return {
    enabled,
    loading: enabled && (needsFetch || busy),
    data,
    isMember: !!data,
    isManager: !!data?.manager,
    pendingRequests: Number(data?.pending_requests) || 0,
    refresh,
  };
}
