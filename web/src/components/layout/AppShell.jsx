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

import React, { useContext, useEffect, useMemo } from 'react';
import '../../i18n/i18n';
import '@douyinfe/semi-ui/dist/css/semi.css';
import { LocaleProvider } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import zh_CN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';
import en_GB from '@douyinfe/semi-ui/lib/es/locale/source/en_GB';
import PageLayout from './PageLayout';
import { UserContext } from '../../context/User';

const SemiLocaleWrapper = ({ children }) => {
  const { i18n } = useTranslation();
  useEffect(() => {
    document.documentElement.lang = i18n.language;
  }, [i18n.language]);
  const semiLocale = useMemo(
    () => ({ zh: zh_CN, en: en_GB })[i18n.language] || zh_CN,
    [i18n.language],
  );
  return <LocaleProvider locale={semiLocale}>{children}</LocaleProvider>;
};

const UserLanguageSync = () => {
  const [userState] = useContext(UserContext);
  const { i18n } = useTranslation();

  useEffect(() => {
    if (!userState.user?.setting) return;
    try {
      const settings = JSON.parse(userState.user.setting);
      if (settings.language && settings.language !== i18n.language) {
        i18n.changeLanguage(settings.language);
      }
    } catch (error) {
      // ignore invalid user settings
    }
  }, [i18n, userState.user?.setting]);

  return null;
};

const AppShell = () => (
  <SemiLocaleWrapper>
    <UserLanguageSync />
    <PageLayout />
  </SemiLocaleWrapper>
);

export default AppShell;
