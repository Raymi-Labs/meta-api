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

import { useContext, useEffect, useState } from 'react';
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';

/** Only the current page is queried; responses never survive an identity change. */
export function useModelHealthSummaries(models, enabled = true) {
  const [userState] = useContext(UserContext);
  const user = userState?.user;
  const names = JSON.stringify(
    models.map((model) => model.model_name).slice(0, 100),
  );
  const [result, setResult] = useState(null);
  useEffect(() => {
    if (!enabled || !user?.id || names === '[]') return;
    let active = true;
    const query = new URLSearchParams({ hours: '24' });
    JSON.parse(names).forEach((name) => query.append('model', name));
    const refresh = () => {
      if (document.hidden) return;
      API.get(`/api/health/models?${query}`, {
        skipErrorHandler: true,
        skipGlobalLoading: true,
        disableDuplicate: true,
        timeout: 15000,
      })
        .then(({ data }) => {
          if (active) {
            setResult(
              data?.success && Array.isArray(data.data?.models)
                ? { user, names, models: data.data.models }
                : null,
            );
          }
        })
        .catch(() => {
          if (active) setResult(null);
        });
    };
    refresh();
    const timer = window.setInterval(refresh, 60000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [user, names, enabled]);
  return enabled && user?.id && result?.user === user && result?.names === names
    ? new Map(result.models.map((item) => [item.model, item]))
    : new Map();
}
