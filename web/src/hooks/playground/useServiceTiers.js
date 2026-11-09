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
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';

export default function useServiceTiers(model, group, owner, enabled = true) {
  const [user] = useContext(UserContext);
  const id = user?.user?.id;
  const scope = JSON.stringify([id, owner, model, group, enabled]);
  const [version, setVersion] = useState(0);
  const [state, setState] = useState({
    scope: '',
    data: null,
    loading: true,
    error: false,
  });
  useEffect(() => {
    const abort = new AbortController();
    let active = true;
    setState({ scope, data: null, loading: true, error: false });
    if (!enabled || !id || !model || !group) {
      setState({ scope, data: null, loading: false, error: false });
      return () => abort.abort();
    }
    API.get('/api/user/service-tiers', {
      params: { model, group },
      headers: { 'New-API-User': id },
      signal: abort.signal,
      disableDuplicate: true,
      skipErrorHandler: true,
      skipGlobalLoading: true,
    })
      .then(({ data }) => {
        if (!active) return;
        const result = data?.data;
        const rates = { default: 1, priority: 2, ultrafast: 6 };
        if (
          !data.success ||
          result?.model !== model ||
          result.group !== group ||
          typeof result.managed !== 'boolean' ||
          !Array.isArray(result.tiers) ||
          result.tiers.length < 1 ||
          result.tiers.length > 3 ||
          result.tiers[0]?.id !== 'default' ||
          result.tiers.some((t) => rates[t.id] !== t.multiplier) ||
          new Set(result.tiers.map((t) => t.id)).size !== result.tiers.length
        )
          throw new Error('invalid tier capabilities');
        setState({ scope, data: result, loading: false, error: false });
      })
      .catch((error) => {
        if (!active || abort.signal.aborted) return;
        // Older servers have no paid capability contract; never infer support.
        setState({
          scope,
          data: null,
          loading: false,
          error: error.response?.status !== 404,
        });
      });
    return () => {
      active = false;
      abort.abort();
    };
  }, [scope, id, model, group, version, enabled]);
  const current = state.scope === scope;
  const tiers =
    current && state.data?.managed
      ? state.data.tiers
      : [{ id: 'default', multiplier: 1 }];
  return {
    tiers,
    managed: current && state.data?.managed === true,
    loading: !current || state.loading,
    error: current && state.error,
    retry: useCallback(() => setVersion((v) => v + 1), []),
    allows: (tier) =>
      !tier ||
      tier === 'default' ||
      (current &&
        !state.loading &&
        state.data?.managed &&
        tiers.some((item) => item.id === tier)),
  };
}
