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

import { useContext, useEffect, useRef } from 'react';
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';
import {
  createLanguageAccountSync,
  createLanguageIdentityGuard,
} from '../../helpers/languageAccount';
import { siteLanguageStore } from '../../helpers/siteLanguage';

// This bridge is above both shells so navigation never creates competing saves
// or re-applies an old account setting when the user returns to the homepage.
const SiteLanguageBridge = () => {
  const [userState, userDispatch] = useContext(UserContext);
  const user = userState?.user;
  const current = useRef(user);
  current.current = user;
  const sync = useRef(null);
  useEffect(() => {
    siteLanguageStore.initialize();
    siteLanguageStore.restore();
    const controller = createLanguageAccountSync({
      store: siteLanguageStore,
      isIdentityCurrent: createLanguageIdentityGuard({
        getIdentity: () => current.current?.id,
      }),
      readAccount: async (id) => {
        if (String(current.current?.id) !== id)
          throw new Error('Account changed');
        const response = await API.get('/api/user/self', {
          skipGlobalLoading: true,
          skipErrorHandler: true,
          disableDuplicate: true,
          headers: { 'New-API-User': id },
        });
        if (!response.data?.success) throw new Error('Account lookup failed');
        return response.data.data;
      },
      save: async (id, language) => {
        if (String(current.current?.id) !== id)
          throw new Error('Account changed');
        const response = await API.put(
          '/api/user/self',
          { language },
          {
            skipGlobalLoading: true,
            skipErrorHandler: true,
            headers: { 'New-API-User': id },
          },
        );
        if (!response.data?.success)
          throw new Error('Language preference was not saved');
      },
      onSaved: (id, language) => {
        const active = current.current;
        if (String(active?.id) !== id) return;
        let settings = {};
        try {
          settings = JSON.parse(active.setting || '{}');
        } catch {
          /* Invalid legacy settings. */
        }
        const next = {
          ...active,
          setting: JSON.stringify({ ...settings, language }),
        };
        // Update the cache too, so refresh cannot restore an older preference.
        try {
          const cached = JSON.parse(localStorage.getItem('user') || 'null');
          if (String(cached?.id) !== id) return;
          localStorage.setItem('user', JSON.stringify(next));
        } catch {
          /* The in-memory account remains usable without storage. */
        }
        current.current = next;
        userDispatch({ type: 'login', payload: next });
      },
    });
    sync.current = controller;
    controller.setIdentity(current.current?.id);
    return () => {
      controller.dispose();
      if (sync.current === controller) sync.current = null;
    };
  }, [userDispatch]);
  useEffect(() => {
    sync.current?.setIdentity(user?.id);
  }, [user?.id]);
  return null;
};
export default SiteLanguageBridge;
