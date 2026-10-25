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

import { useCallback, useEffect, useState } from 'react';
import { normalizePublicLanguage } from './publicLanguage';
import { translateContentText, translateMarkdown } from './contentLanguage';

const dictionaries = new Map([['zh-CN', null]]);
const inflight = new Map();
const loaders = import.meta.glob('./contentLocales/*.json');

export async function loadContentLanguage(language) {
  if (dictionaries.has(language)) return dictionaries.get(language);
  if (!inflight.has(language)) {
    const loader = loaders[`./contentLocales/${language}.json`];
    if (!loader) throw new Error('Content language unavailable');
    inflight.set(
      language,
      loader()
        .then((module) => {
          dictionaries.set(language, module.default);
          return module.default;
        })
        .finally(() => inflight.delete(language)),
    );
  }
  return inflight.get(language);
}

export function useContentLanguage(value) {
  const language = normalizePublicLanguage(value);
  const [revision, setRevision] = useState(0);
  const [state, setState] = useState({ language, failed: false });
  const ready = dictionaries.has(language);
  const failed = state.language === language && state.failed;
  const dictionary = ready ? dictionaries.get(language) : null;
  useEffect(() => {
    let cancelled = false;
    if (!dictionaries.has(language)) {
      setState({ language, failed: false });
      loadContentLanguage(language).then(
        () => {
          if (!cancelled) setState({ language, failed: false });
        },
        () => {
          if (!cancelled) setState({ language, failed: true });
        },
      );
    }
    return () => {
      cancelled = true;
    };
  }, [language, revision]);
  const retry = useCallback(() => setRevision((current) => current + 1), []);
  const translate = useCallback(
    (source, values) => translateContentText(source, dictionary, values),
    [dictionary],
  );
  const markdown = useCallback(
    (source, values) =>
      translateContentText(translateMarkdown(source, dictionary), null, values),
    [dictionary],
  );
  return {
    language,
    dictionary,
    ready,
    pending: !ready && !failed,
    failed,
    retry,
    translate,
    markdown,
  };
}
