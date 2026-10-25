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

import React, { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { LocaleProvider } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { ToastContainer } from 'react-toastify';
import DocsPage from './index';
import {
  DOCS_ORIGIN,
  MAIN_ORIGIN,
  getActiveDoc,
  docPath,
} from './docsNavigation';
import { getSemiLocale } from '../../helpers/semiLocale';
import { useContentLanguage } from '../../helpers/contentLanguageReact';
import { useSiteLanguage } from '../../hooks/common/useSiteLanguage';
import { useSetTheme, useTheme } from '../../context/Theme';
import PublicLanguageSelector from '../../components/layout/PublicLanguageSelector';
import ThemeToggle from '../../components/layout/headerbar/ThemeToggle';

export default function DocsSite() {
  const { t } = useTranslation();
  const { language } = useSiteLanguage();
  const bodyLanguage = useContentLanguage(language);
  const theme = useTheme();
  const setTheme = useSetTheme();
  const location = useLocation();
  useEffect(() => {
    const doc = getActiveDoc(location.pathname, true);
    document.title = `${doc ? t(bodyLanguage.translate(doc.title)) : t('文档不存在')} · 元衡 API`;
    document
      .querySelector('link[rel="canonical"]')
      ?.setAttribute(
        'href',
        `${DOCS_ORIGIN}${doc ? docPath(doc.path, true) : location.pathname}`,
      );
  }, [location.pathname, t, bodyLanguage.translate]);
  return (
    <LocaleProvider locale={getSemiLocale(language)}>
      <DocsPage
        standalone
        brand={
          <a href={`${MAIN_ORIGIN}/?lang=${language}`} aria-label={t('首页')}>
            <img src='/yuanheng-logo.png' alt='' />
            <span>元衡 API</span>
          </a>
        }
        controls={
          <div className='nan-docs-controls'>
            <ThemeToggle theme={theme} onThemeToggle={setTheme} t={t} />
            <PublicLanguageSelector variant='label' />
          </div>
        }
      />
      <ToastContainer position='top-center' />
    </LocaleProvider>
  );
}
