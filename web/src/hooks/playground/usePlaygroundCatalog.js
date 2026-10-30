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
import { API } from '../../helpers';
import { getPlaygroundOwner } from '../../components/playground/configStorage';

export function usePlaygroundCatalog(owner) {
  const [catalog, setCatalog] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(false);
    API.get('/api/pricing', { disableDuplicate: true, skipErrorHandler: true })
      .then(({ data }) => {
        if (!active || owner !== getPlaygroundOwner()) return;
        if (!data.success || !Array.isArray(data.data))
          throw new Error('catalog unavailable');
        const vendors = new Map(
          (data.vendors || []).map((vendor) => [vendor.id, vendor]),
        );
        setCatalog({
          models: data.data.map((model) => ({
            ...model,
            vendor: vendors.get(model.vendor_id),
          })),
          groupRatio: data.group_ratio || {},
          timeRatio: data.time_ratio || {},
        });
      })
      .catch(() => {
        if (active && owner === getPlaygroundOwner()) {
          setCatalog(null);
          setError(true);
        }
      })
      .finally(() => {
        if (active && owner === getPlaygroundOwner()) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [owner, revision]);
  return {
    catalog,
    loading,
    error,
    reload: () => setRevision((value) => value + 1),
  };
}
