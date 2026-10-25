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

import { getSiteLanguageSnapshot } from './siteLanguage';

// Transfer only the explicit language preference across the two site origins.
export function withDocsLanguage(
  href,
  language = getSiteLanguageSnapshot().language,
) {
  try {
    const url = new URL(href);
    if (url.origin !== 'https://docs.meta-api.vip') return href;
    url.searchParams.set('lang', language);
    return url.href;
  } catch {
    return href;
  }
}
