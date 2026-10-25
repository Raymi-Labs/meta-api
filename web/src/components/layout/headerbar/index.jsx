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

import React, { Suspense, lazy } from 'react';
import { useHeaderBar } from '../../../hooks/common/useHeaderBar';
import { useNotifications } from '../../../hooks/common/useNotifications';
import { useNavigation } from '../../../hooks/common/useNavigation';
import MobileMenuButton from './MobileMenuButton';
import HeaderLogo from './HeaderLogo';
import Navigation from './Navigation';
import ActionButtons from './ActionButtons';

const NoticeModal = lazy(() => import('../NoticeModal'));

const HeaderBar = ({ onMobileMenuToggle, drawerOpen }) => {
  const {
    userState,
    statusState,
    isMobile,
    collapsed,
    logoLoaded,
    currentLang,
    isLoading,
    systemName,
    logo,
    isNewYear,
    isSelfUseMode,
    docsLink,
    isDemoSiteMode,
    isConsoleRoute,
    theme,
    headerNavModules,
    pricingRequireAuth,
    logout,
    handleLanguageChange,
    handleThemeToggle,
    handleMobileMenuToggle,
    navigate,
    t,
  } = useHeaderBar({ onMobileMenuToggle, drawerOpen });

  const {
    noticeVisible,
    unreadCount,
    announcementUnread,
    pendingWithdrawalCount,
    pendingWithdrawals,
    pendingInvoiceCount,
    pendingInvoices,
    isAdminUser,
    handleNoticeOpen,
    handleNoticeClose,
    getUnreadKeys,
  } = useNotifications(statusState);

  const { mainNavLinks } = useNavigation(t, docsLink, headerNavModules);

  return (
    <header
      className='site-header site-header--full-width sticky top-0 z-50 transition-colors duration-300'
      data-onboarding='app-header'
    >
      {noticeVisible && (
        <Suspense fallback={null}>
          <NoticeModal
            visible={noticeVisible}
            onClose={handleNoticeClose}
            isMobile={isMobile}
            defaultTab={
              isAdminUser && pendingWithdrawalCount > 0
                ? 'withdrawals'
                : isAdminUser && pendingInvoiceCount > 0
                  ? 'invoices'
                  : announcementUnread > 0
                    ? 'system'
                    : 'inApp'
            }
            unreadKeys={getUnreadKeys()}
            pendingWithdrawalCount={pendingWithdrawalCount}
            pendingWithdrawals={pendingWithdrawals}
            pendingInvoiceCount={pendingInvoiceCount}
            pendingInvoices={pendingInvoices}
            showWithdrawalTab={isAdminUser}
            showInvoiceTab={isAdminUser}
          />
        </Suspense>
      )}

      <div className='public-header-inner'>
        <div className='flex items-center justify-between h-16'>
          <div className='flex flex-shrink-0 items-center'>
            <MobileMenuButton
              isConsoleRoute={isConsoleRoute}
              isMobile={isMobile}
              drawerOpen={drawerOpen}
              collapsed={collapsed}
              onToggle={handleMobileMenuToggle}
              t={t}
            />

            <HeaderLogo
              isMobile={isMobile}
              isConsoleRoute={isConsoleRoute}
              logo={logo}
              logoLoaded={logoLoaded}
              isLoading={isLoading}
              systemName={systemName}
              isSelfUseMode={isSelfUseMode}
              isDemoSiteMode={isDemoSiteMode}
              t={t}
            />
          </div>

          <Navigation
            mainNavLinks={mainNavLinks}
            isMobile={isMobile}
            isLoading={isLoading}
            userState={userState}
            pricingRequireAuth={pricingRequireAuth}
          />

          <ActionButtons
            isNewYear={isNewYear}
            unreadCount={unreadCount}
            onNoticeOpen={handleNoticeOpen}
            theme={theme}
            onThemeToggle={handleThemeToggle}
            currentLang={currentLang}
            onLanguageChange={handleLanguageChange}
            userState={userState}
            isLoading={isLoading}
            isMobile={isMobile}
            isSelfUseMode={isSelfUseMode}
            logout={logout}
            navigate={navigate}
            t={t}
          />
        </div>
      </div>
    </header>
  );
};

export default HeaderBar;
