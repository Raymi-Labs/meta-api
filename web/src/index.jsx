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

import React, { lazy, Suspense, useEffect } from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, useLocation } from 'react-router-dom';
import { UserProvider } from './context/User';
import { StatusProvider } from './context/Status';
import { ThemeProvider } from './context/Theme';
import './index.css';
import PublicHomeShell from './components/layout/PublicHomeShell';
import GlobalTopProgress from './components/common/ui/GlobalTopProgress';
import { beginGlobalLoading, endGlobalLoading } from './helpers/globalLoading';
import { isPublicLegalPath } from './helpers/publicRoutes';
import { initializeAnalyticsConsent } from './helpers/analyticsConsent';
import SiteLanguageBridge from './components/layout/SiteLanguageBridge';

// AppShell chunk 与初始语言包一起等待就绪后再渲染：
// 非中文用户若不等语言包，t() 会先返回中文 key 再切换目标语言，造成语言闪变。
const AppShell = lazy(() =>
  Promise.all([
    import('./components/layout/AppShell'),
    import('./i18n/i18n').then((module) => module.initialLocaleReady),
  ]).then(([module]) => module),
);

// 懒加载等待期间显示顶部进度条，替代纯白屏。
const ShellFallback = () => {
  useEffect(() => {
    beginGlobalLoading();
    return () => endGlobalLoading();
  }, []);
  return (
    <div className='fixed inset-0'>
      <GlobalTopProgress />
    </div>
  );
};

// 欢迎信息（二次开发者未经允许不准将此移除）
// Welcome message (Do not remove this without permission from the original developer)
if (typeof window !== 'undefined') {
  console.log(
    '%cWE ❤ NEWAPI%c Github: https://github.com/QuantumNous/new-api',
    'color: #10b981; font-weight: bold; font-size: 24px;',
    'color: inherit; font-size: 14px;',
  );
}

const ShellRouter = () => {
  const location = useLocation();
  if (
    location.pathname === '/' ||
    /^\/download\/?$/.test(location.pathname) ||
    isPublicLegalPath(location.pathname)
  ) {
    return <PublicHomeShell />;
  }
  return (
    <Suspense fallback={<ShellFallback />}>
      <AppShell />
    </Suspense>
  );
};

// initialization
initializeAnalyticsConsent();

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <StatusProvider>
      <UserProvider>
        <SiteLanguageBridge />
        <BrowserRouter
          future={{
            v7_startTransition: true,
            v7_relativeSplatPath: true,
          }}
        >
          <ThemeProvider>
            <ShellRouter />
          </ThemeProvider>
        </BrowserRouter>
      </UserProvider>
    </StatusProvider>
  </React.StrictMode>,
);
