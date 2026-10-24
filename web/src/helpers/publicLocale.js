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

import { useEffect, useSyncExternalStore } from 'react';
import { createPublicLanguageStore } from './publicLanguage';
import {
  normalizeLocale,
  PUBLIC_LANGUAGE_CHANGE_EVENT,
} from './localeNormalize';
export { normalizePublicLanguage } from './publicLanguage';

const store = createPublicLanguageStore();
let mountedConsumers = 0;
export const getPublicLanguage = () => store.getSnapshot().language;
export const setPublicLanguage = async (language) => {
  const changed = await store.change(language);
  if (changed && typeof window !== 'undefined') {
    window.dispatchEvent(
      new CustomEvent(PUBLIC_LANGUAGE_CHANGE_EVENT, {
        detail: normalizeLocale(store.getSnapshot().language),
      }),
    );
  }
  return changed;
};

export const usePublicTranslation = () => {
  const state = useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    store.getSnapshot,
  );
  useEffect(() => {
    if (mountedConsumers++ === 0) {
      store.restore();
      window.addEventListener('storage', store.handleStorage);
    }
    return () => {
      if (--mountedConsumers === 0)
        window.removeEventListener('storage', store.handleStorage);
    };
  }, []);
  useEffect(() => {
    document.documentElement.lang = state.language;
  }, [state.language]);
  return {
    ...state,
    t: store.translate,
    isChinese: state.language.startsWith('zh'),
    setLanguage: setPublicLanguage,
  };
};
