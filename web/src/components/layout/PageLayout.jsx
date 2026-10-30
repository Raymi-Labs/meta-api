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

import HeaderBar from './headerbar';
import { Layout } from '@douyinfe/semi-ui';
import App from '../../App';
import ErrorBoundary from '../common/ErrorBoundary';
import React, {
  lazy,
  Suspense,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from 'react';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { useSidebarCollapsed } from '../../hooks/common/useSidebarCollapsed';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers/apiCore';
import { setStatusData, setUserData } from '../../helpers/data';
import { getLogo, getSystemName } from '../../helpers/storage';
import { showError } from '../../helpers/toast';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useLocation } from 'react-router-dom';
import { isModelDetailPath } from '../../helpers/modelDetailNavigation';
import GlobalTopProgress from '../common/ui/GlobalTopProgress';
import UserOnboarding from '../onboarding/UserOnboarding';
import ContentSafetyNotice from '../content-safety/ContentSafetyNotice';
const { Sider, Content, Header } = Layout;
const SiderBar = lazy(() => import('./SiderBar'));

const PageLayout = () => {
  const [, userDispatch] = useContext(UserContext);
  const [, statusDispatch] = useContext(StatusContext);
  const isMobile = useIsMobile();
  const [collapsed, , setCollapsed] = useSidebarCollapsed();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { i18n } = useTranslation();
  const location = useLocation();

  const isConsoleRoute = /^\/console(?:\/|$)/i.test(location.pathname);
  const isDocsRoute = /^\/docs(?:\/|$)/i.test(location.pathname);

  const shouldInnerPadding =
    location.pathname.includes('/console') &&
    !location.pathname.startsWith('/console/chat');

  const isStandalonePage = location.pathname === '/usage';
  const showSider = isConsoleRoute && (!isMobile || drawerOpen);

  useLayoutEffect(() => {
    // 桌面端滚动右侧容器，手机端滚动窗口；切页都从顶部开始。
    document.querySelector('.app-workspace')?.scrollTo(0, 0);
    window.scrollTo(0, 0);
  }, [location.pathname]);

  // 首页若设置了自定义 home_page_content，自定义 HTML 自带顶部导航，
  // 全局 Header 与之并排会出现"双顶栏"。此时隐藏全局 Header，
  // 让自定义 landing 独立接管视觉。其他路径行为完全不变。
  const hideHeaderForCustomLanding = useMemo(() => {
    if (location.pathname !== '/') return false;
    try {
      return Boolean(localStorage.getItem('home_page_content'));
    } catch {
      return false;
    }
  }, [location.pathname]);

  useEffect(() => {
    if (isMobile && drawerOpen && collapsed) {
      setCollapsed(false);
    }
  }, [isMobile, drawerOpen, collapsed, setCollapsed]);

  const loadUser = async () => {
    const storedUser = localStorage.getItem('user');
    if (!storedUser) {
      return;
    }

    try {
      const data = JSON.parse(storedUser);
      userDispatch({ type: 'login', payload: data });
    } catch (error) {
      // Ignore broken local cache and try refreshing from the server below.
    }

    try {
      const res = await API.get('/api/user/self');
      const { success, data } = res.data || {};
      if (success && data) {
        userDispatch({ type: 'login', payload: data });
        setUserData(data);
      }
    } catch (error) {
      // Keep the current local cache to avoid blocking the page on refresh failure.
    }
  };

  const loadStatus = async () => {
    try {
      const res = await API.get('/api/status');
      const { success, data } = res.data;
      if (success) {
        statusDispatch({ type: 'set', payload: data });
        setStatusData(data);
      } else {
        showError('Unable to connect to server');
      }
    } catch (error) {
      showError('Failed to load status');
    }
  };

  useEffect(() => {
    loadUser().catch(console.error);
    loadStatus().catch(console.error);
    let systemName = getSystemName();
    if (systemName) {
      document.title = systemName;
    }
    let logo = getLogo();
    if (logo) {
      let linkElement = document.querySelector("link[rel~='icon']");
      if (linkElement) {
        linkElement.href = logo;
      }
    }
  }, [i18n]);

  if (isStandalonePage) {
    return (
      <>
        <GlobalTopProgress />
        <ContentSafetyNotice />
        <ErrorBoundary>
          <App />
        </ErrorBoundary>
      </>
    );
  }

  return (
    <Layout
      className='app-layout'
      style={{
        display: 'flex',
        flexDirection: 'column',
        overflow: isMobile ? 'visible' : 'hidden',
      }}
    >
      <GlobalTopProgress />
      {!hideHeaderForCustomLanding && (
        <Header
          style={{
            padding: 0,
            height: 'auto',
            lineHeight: 'normal',
            position: 'fixed',
            width: '100%',
            top: 0,
            zIndex: 100,
          }}
        >
          <HeaderBar
            onMobileMenuToggle={() => setDrawerOpen((prev) => !prev)}
            drawerOpen={drawerOpen}
          />
        </Header>
      )}
      <Layout
        style={{
          overflow: isMobile ? 'visible' : 'auto',
          display: 'flex',
          flexDirection: isMobile ? 'column' : 'row',
          paddingTop: hideHeaderForCustomLanding ? '0' : '64px',
          flex: '1 1 auto',
          minHeight: 0,
        }}
      >
        {showSider && (
          <Sider
            className='app-sider'
            style={{
              position: isMobile ? 'fixed' : 'sticky',
              left: isMobile ? 0 : 'auto',
              top: isMobile ? '64px' : 0,
              zIndex: isMobile ? 99 : 'auto',
              border: 'none',
              paddingRight: '0',
              width: 'var(--sidebar-current-width)',
              minWidth: 'var(--sidebar-current-width)',
              maxWidth: 'var(--sidebar-current-width)',
              flex: '0 0 var(--sidebar-current-width)',
            }}
          >
            <Suspense fallback={null}>
              <SiderBar
                onNavigate={() => {
                  if (isMobile) setDrawerOpen(false);
                }}
              />
            </Suspense>
          </Sider>
        )}
        <Layout
          className='app-workspace'
          style={{
            flex: '1 1 auto',
            display: 'flex',
            flexDirection: 'column',
            minWidth: 0,
            minHeight: 0,
            overflowX: isMobile ? 'visible' : 'hidden',
            overflowY: isMobile ? 'visible' : 'auto',
          }}
        >
          <ContentSafetyNotice />
          <Content
            style={{
              flex: '1 0 auto',
              overflowY:
                isMobile || isDocsRoute || isModelDetailPath(location.pathname)
                  ? 'visible'
                  : 'hidden',
              WebkitOverflowScrolling: 'touch',
              padding: shouldInnerPadding ? (isMobile ? '5px' : '24px') : '0',
              position: 'relative',
            }}
          >
            <ErrorBoundary>
              <App />
            </ErrorBoundary>
            <UserOnboarding />
          </Content>
        </Layout>
      </Layout>
    </Layout>
  );
};

export default PageLayout;
