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

import React from 'react';
import '../../i18n/i18n';
import '@douyinfe/semi-ui/dist/css/semi.css';
import { LocaleProvider } from '@douyinfe/semi-ui';
import PageLayout from './PageLayout';
import { getSemiLocale } from '../../helpers/semiLocale';
import { useSiteLanguage } from '../../hooks/common/useSiteLanguage';

const SemiLocaleWrapper = ({ children }) => {
  const { language } = useSiteLanguage();
  return (
    <LocaleProvider locale={getSemiLocale(language)}>{children}</LocaleProvider>
  );
};

const AppShell = () => (
  <SemiLocaleWrapper>
    <PageLayout />
  </SemiLocaleWrapper>
);

export default AppShell;
