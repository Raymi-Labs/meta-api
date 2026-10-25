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

import { useCallback, useContext, useEffect, useState } from 'react';
import { API } from '../../helpers/apiCore';
import { UserContext } from '../../context/User';
import { publicDocsAPI } from './publicDocsAPI';

const EMPTY = {
  loading: true,
  pricing: null,
  serverAddress: '',
  updatedAt: null,
  failed: false,
};

export default function useRuntimeDocs({ publicOnly = false } = {}) {
  const [userState] = useContext(UserContext);
  const user = publicOnly ? null : userState?.user;
  const owner = JSON.stringify([user?.id, user?.role, user?.group]);
  const [state, setState] = useState(EMPTY);
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);

  useEffect(() => {
    let disposed = false;
    let pending = false;
    const controller = new AbortController();
    async function load() {
      if (pending || disposed) return;
      pending = true;
      setState((old) => ({
        ...(old.owner === owner ? old : EMPTY),
        owner,
        loading: true,
      }));
      const options = {
        signal: controller.signal,
        timeout: 15000,
        skipGlobalLoading: true,
        skipErrorHandler: true,
        disableDuplicate: true,
      };
      const client = publicOnly ? publicDocsAPI : API;
      const [status, pricing] = await Promise.allSettled([
        client.get('/api/status', options),
        client.get('/api/pricing', options),
      ]);
      pending = false;
      if (disposed) return;
      const statusData =
        status.status === 'fulfilled' && status.value.data?.success
          ? status.value.data.data
          : null;
      const pricingData =
        pricing.status === 'fulfilled' &&
        pricing.value.data?.success &&
        Array.isArray(pricing.value.data.data)
          ? pricing.value.data
          : null;
      setState({
        owner,
        loading: false,
        serverAddress: statusData?.server_address || '',
        pricing: pricingData,
        failed: !statusData || !pricingData,
        updatedAt: statusData && pricingData ? Date.now() : null,
      });
    }
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    load();
    const timer = setInterval(onVisible, 60000);
    window.addEventListener('focus', onVisible);
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      disposed = true;
      controller.abort();
      clearInterval(timer);
      window.removeEventListener('focus', onVisible);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [owner, revision, publicOnly]);

  // Do not render data from the previous identity, even for one render.
  return { ...(state.owner === owner ? state : EMPTY), refresh };
}
