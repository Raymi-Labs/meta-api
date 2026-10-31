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

import { useEffect, useState } from 'react';
import { API } from '../../helpers/api';
import { usableOfficialExchangeRate } from '../../helpers/modelGroupDiscount';

export function useOfficialExchangeRate(enabled) {
  const [state, setState] = useState({ quote: null, loading: false });
  useEffect(() => {
    if (!enabled) return;
    let active = true;
    const refresh = async () => {
      setState((old) => ({ ...old, loading: true }));
      try {
        const { data } = await API.get('/api/pricing/exchange-rate', {
          skipErrorHandler: true,
        });
        const quote =
          data.success && usableOfficialExchangeRate(data.data)
            ? data.data
            : null;
        if (active) setState({ quote, loading: false });
      } catch {
        if (active)
          setState((old) => ({
            quote: usableOfficialExchangeRate(old.quote)
              ? { ...old.quote, stale: true }
              : null,
            loading: false,
          }));
      }
    };
    refresh();
    const timer = setInterval(refresh, 15 * 60 * 1000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [enabled]);
  return enabled ? state : { quote: null, loading: false };
}
