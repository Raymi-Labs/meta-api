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

import zhCN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';
import zhTW from '@douyinfe/semi-ui/lib/es/locale/source/zh_TW';
import en from '@douyinfe/semi-ui/lib/es/locale/source/en_GB';
import ja from '@douyinfe/semi-ui/lib/es/locale/source/ja_JP';
import ko from '@douyinfe/semi-ui/lib/es/locale/source/ko_KR';
import de from '@douyinfe/semi-ui/lib/es/locale/source/de';
import es from '@douyinfe/semi-ui/lib/es/locale/source/es';
import fr from '@douyinfe/semi-ui/lib/es/locale/source/fr';
import pt from '@douyinfe/semi-ui/lib/es/locale/source/pt_BR';
import ru from '@douyinfe/semi-ui/lib/es/locale/source/ru_RU';
import tr from '@douyinfe/semi-ui/lib/es/locale/source/tr_TR';
import pl from '@douyinfe/semi-ui/lib/es/locale/source/pl_PL';
import nl from '@douyinfe/semi-ui/lib/es/locale/source/nl_NL';
import it from '@douyinfe/semi-ui/lib/es/locale/source/it';
import vi from '@douyinfe/semi-ui/lib/es/locale/source/vi_VN';
import uk from '../i18n/semi/uk';
import bg from '../i18n/semi/bg';
import { normalizeSiteLanguage } from './siteLanguage';

export const SEMI_LOCALES = Object.freeze({
  'zh-CN': zhCN,
  'zh-TW': zhTW,
  en,
  ja,
  ko,
  de,
  es,
  fr,
  pt,
  ru,
  tr,
  pl,
  nl,
  it,
  vi,
  uk,
  bg,
});

export function getSemiLocale(language) {
  return SEMI_LOCALES[normalizeSiteLanguage(language)] || en;
}
