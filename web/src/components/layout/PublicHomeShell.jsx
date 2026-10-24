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

import { Monitor, Moon, Sun, Menu, X } from 'lucide-react';
import React, {
  lazy,
  Suspense,
  useContext,
  useEffect,
  useLayoutEffect,
  useState,
} from 'react';
import { Link, useLocation } from 'react-router-dom';
import { StatusContext } from '../../context/Status';
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';
import { setStatusData, setUserData } from '../../helpers/data';
import { getLogo, getSystemName } from '../../helpers/storage';
import { useSetTheme, useTheme } from '../../context/Theme';
import { usePublicTranslation } from '../../helpers/publicLocale';
import {
  prefetchAppShell,
  schedulePrefetchAppShell,
} from '../../helpers/prefetchAppShell';
import Home from '../../pages/Home';
import GlobalTopProgress from '../common/ui/GlobalTopProgress';
import PublicFooter from './PublicFooter';
import PublicLanguageSelector from './PublicLanguageSelector';
import { isPublicLegalPath } from '../../helpers/publicRoutes';

const DesktopDownload = lazy(() => import('../../pages/DesktopDownload'));
const PublicLegal = lazy(() => import('../../pages/PublicLegal'));

const defaultModules = {
  home: true,
  console: true,
  pricing: true,
  docs: true,
  about: true,
  usage: false,
};

const parseHeaderModules = (rawConfig) => {
  if (!rawConfig) return defaultModules;
  try {
    const modules = JSON.parse(rawConfig);
    if (typeof modules.pricing === 'boolean') {
      modules.pricing = {
        enabled: modules.pricing,
        requireAuth: false,
      };
    }
    return modules;
  } catch (error) {
    return defaultModules;
  }
};

const PublicHomeShell = () => {
  const location = useLocation();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  useLayoutEffect(() => {
    setMobileNavOpen(false);
    // Match PageLayout navigation, while leaving explicit anchors intact.
    if (!location.hash) window.scrollTo(0, 0);
  }, [location.pathname, location.hash]);

  useEffect(() => {
    if (!mobileNavOpen) return;
    const onEscape = (event) => {
      if (event.key === 'Escape') {
        setMobileNavOpen(false);
        document.getElementById('public-menu-toggle')?.focus();
      }
    };
    document.addEventListener('keydown', onEscape);
    return () => document.removeEventListener('keydown', onEscape);
  }, [mobileNavOpen]);
  const isDownloadPage = /^\/download\/?$/.test(location.pathname);
  const isLegalPage = isPublicLegalPath(location.pathname);
  const isPublicSubpage = isDownloadPage || isLegalPage;
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);
  const { t } = usePublicTranslation();
  const theme = useTheme();
  const setTheme = useSetTheme();
  const status = statusState?.status || {};
  const systemName = getSystemName();
  const logo = getLogo();
  const modules = parseHeaderModules(status.HeaderNavModules);
  const docsLink = status.docs_link || localStorage.getItem('docs_link') || '';
  const showRegister = !status.self_use_mode_enabled;

  // 自定义首页（自带顶部导航）时隐藏项目 public-header，避免双顶栏。
  // 初值同步读取 localStorage 缓存避免首屏闪烁，随后由 Home 的 onLandingChange 校正。
  const [customLanding, setCustomLanding] = useState(() => {
    try {
      return Boolean(localStorage.getItem('home_page_content'));
    } catch {
      return false;
    }
  });

  const navLinks = [
    modules.home && { text: t('首页'), to: '/' },
    { text: t('下载客户端'), to: '/download', prefetchApp: false },
    modules.console && {
      text: t('控制台'),
      to: userState.user ? '/console' : '/login',
    },
    (typeof modules.pricing === 'object'
      ? modules.pricing.enabled
      : modules.pricing) && {
      text: t('模型广场'),
      to:
        modules.pricing?.requireAuth && !userState.user ? '/login' : '/pricing',
    },
    modules.docs &&
      docsLink && { text: t('文档'), href: docsLink, external: true },
    modules.about && { text: t('关于'), to: '/about' },
  ].filter(Boolean);

  useEffect(() => {
    const loadUser = async () => {
      const storedUser = localStorage.getItem('user');
      if (!storedUser) return;
      try {
        userDispatch({ type: 'login', payload: JSON.parse(storedUser) });
      } catch (error) {
        // ignore broken cache
      }
      try {
        const res = await API.get('/api/user/self', {
          skipGlobalLoading: true,
          skipErrorHandler: true,
        });
        if (res.data?.success && res.data?.data) {
          userDispatch({ type: 'login', payload: res.data.data });
          setUserData(res.data.data);
        }
      } catch (error) {
        // keep local cache
      }
    };

    const loadStatus = async () => {
      try {
        const res = await API.get('/api/status', {
          skipGlobalLoading: true,
          skipErrorHandler: true,
        });
        if (res.data?.success) {
          statusDispatch({ type: 'set', payload: res.data.data });
          setStatusData(res.data.data);
          if (res.data.data.system_name) document.title = getSystemName();
          if (res.data.data.logo) {
            const linkElement = document.querySelector("link[rel~='icon']");
            if (linkElement) linkElement.href = getLogo();
          }
        }
      } catch (error) {
        // render default home without blocking on status
      }
    };

    loadUser();
    loadStatus();
    schedulePrefetchAppShell();
  }, [statusDispatch, userDispatch]);

  const toggleTheme = () => {
    if (theme === 'dark') {
      setTheme('light');
    } else if (theme === 'light') {
      setTheme('auto');
    } else {
      setTheme('dark');
    }
  };

  const isCurrentNavLink = (to) => {
    if (!to) return false;
    if (to === '/') return location.pathname === '/';
    return location.pathname.replace(/\/$/, '') === to;
  };

  return (
    <div
      className={`public-shell ${!isPublicSubpage && !customLanding ? 'public-shell--landing' : ''} min-h-screen bg-[var(--semi-color-bg-0,#fff)] text-[var(--semi-color-text-0,#111827)]`}
    >
      <GlobalTopProgress />
      {(!customLanding || isPublicSubpage) && (
        <header className='public-header public-header--hero fixed top-0 left-0 right-0 z-50 backdrop-blur-lg'>
          <div className='public-header-inner flex items-center justify-between h-16'>
            <Link
              to='/'
              className='public-brand group flex items-center gap-2 min-w-0'
            >
              <img
                src={logo}
                alt='logo'
                className='w-8 h-8 rounded-full object-contain transition-transform duration-200 group-hover:scale-110'
              />
              <span className='public-brand-name text-lg font-semibold truncate'>
                {systemName}
              </span>
            </Link>

            <nav
              id='public-site-nav'
              data-open={mobileNavOpen}
              aria-label={t('主导航')}
              className='public-primary-nav flex flex-1 items-center gap-1 lg:gap-2 mx-2 md:mx-4 overflow-x-auto whitespace-nowrap scrollbar-hide'
              onClick={() => setMobileNavOpen(false)}
            >
              {navLinks.map((link) =>
                link.external ? (
                  <a
                    key={link.text}
                    href={link.href}
                    target='_blank'
                    rel='noopener noreferrer'
                    className='public-nav-link'
                  >
                    {link.text}
                  </a>
                ) : (
                  <Link
                    key={link.text}
                    to={link.to}
                    className='public-nav-link'
                    aria-current={
                      isCurrentNavLink(link.to) ? 'page' : undefined
                    }
                    onMouseEnter={
                      link.prefetchApp === false ? undefined : prefetchAppShell
                    }
                    onTouchStart={
                      link.prefetchApp === false ? undefined : prefetchAppShell
                    }
                  >
                    {link.text}
                  </Link>
                ),
              )}
            </nav>

            <div className='public-header-actions flex items-center gap-2 md:gap-3'>
              <button
                id='public-menu-toggle'
                type='button'
                className='public-icon-button public-menu-toggle'
                aria-label={t('导航菜单')}
                aria-expanded={mobileNavOpen}
                aria-controls='public-site-nav'
                onClick={() => setMobileNavOpen((open) => !open)}
              >
                {mobileNavOpen ? <X size={18} /> : <Menu size={18} />}
              </button>
              <button
                type='button'
                className='public-icon-button public-theme-button'
                aria-label={t('切换主题')}
                onClick={toggleTheme}
              >
                {theme === 'dark' ? (
                  <Moon size={16} />
                ) : theme === 'light' ? (
                  <Sun size={16} />
                ) : (
                  <Monitor size={16} />
                )}
              </button>
              <PublicLanguageSelector />
              {userState.user ? (
                <Link
                  to='/console/personal'
                  className='public-user-link'
                  onMouseEnter={prefetchAppShell}
                  onTouchStart={prefetchAppShell}
                >
                  <span className='public-avatar'>
                    {userState.user.username?.[0]?.toUpperCase() || 'U'}
                  </span>
                  <span className='hidden md:inline max-w-[120px] truncate'>
                    {userState.user.username}
                  </span>
                </Link>
              ) : (
                <div className='flex items-center'>
                  <Link
                    to='/login'
                    className='public-login-link'
                    onMouseEnter={prefetchAppShell}
                    onTouchStart={prefetchAppShell}
                  >
                    {t('登录')}
                  </Link>
                  {showRegister && (
                    <Link
                      to='/register'
                      className='public-register-link'
                      onMouseEnter={prefetchAppShell}
                      onTouchStart={prefetchAppShell}
                    >
                      {t('注册')}
                    </Link>
                  )}
                </div>
              )}
            </div>
          </div>
        </header>
      )}
      {isPublicSubpage ? (
        <Suspense
          fallback={
            <main className='min-h-screen bg-[var(--semi-color-bg-0,#fff)]'>
              <span className='sr-only'>{t('正在加载')}</span>
            </main>
          }
        >
          {isLegalPage ? <PublicLegal status={status} /> : <DesktopDownload />}
        </Suspense>
      ) : (
        <Home onLandingChange={setCustomLanding} />
      )}
      <PublicFooter
        systemName={systemName}
        logo={logo}
        modules={modules}
        docsLink={docsLink}
        user={userState.user}
        status={status}
        customLanding={customLanding}
        isDownloadPage={isDownloadPage}
        isLegalPage={isLegalPage}
        t={t}
      />
    </div>
  );
};

export default PublicHomeShell;
