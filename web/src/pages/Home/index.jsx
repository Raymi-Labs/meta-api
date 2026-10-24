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

import React, { lazy, Suspense, useContext, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { API_ENDPOINTS } from '../../constants/common.constant';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { API } from '../../helpers/apiCore';
import { copy } from '../../helpers/clipboard';
import { usePublicTranslation } from '../../helpers/publicLocale';
import { isHttpsUrl } from '../../helpers/richContent';
import { getLogo, getSystemName } from '../../helpers/storage';
import RichContent from '../../components/common/RichContent';
import EnterpriseHome from './EnterpriseHome';
import { getPublicHomeServerAddress } from './homeAddress';

const PublicNoticeModal = lazy(
  () => import('../../components/layout/PublicNoticeModal'),
);

const isRemoteHomePage = isHttpsUrl;

const getCachedHomePageContent = () => {
  try {
    return localStorage.getItem('home_page_content') || '';
  } catch (error) {
    return '';
  }
};

const Home = ({ onLandingChange } = {}) => {
  const { t, language } = usePublicTranslation();
  const navigate = useNavigate();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContent, setHomePageContent] = useState(
    getCachedHomePageContent,
  );
  const [homePageResolved, setHomePageResolved] = useState(
    () => getCachedHomePageContent() !== '',
  );
  const [noticeContent, setNoticeContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const docsLink = statusState?.status?.docs_link || '';
  const publicHomeServerAddress = getPublicHomeServerAddress(
    statusState?.status?.server_address,
    window.location.origin,
  );
  const endpointItems = API_ENDPOINTS.map((e) => ({ value: e }));
  const [endpointIndex, setEndpointIndex] = useState(0);
  const useDefaultHome = homePageContent === '';
  const currentEndpoint = endpointItems[endpointIndex]?.value || '';
  const brandName = getSystemName();
  const brandLogo = getLogo();

  const displayHomePageContent = async () => {
    const cachedContent = getCachedHomePageContent();
    if (cachedContent) {
      setHomePageContent(cachedContent);
    }

    try {
      const res = await API.get('/api/home_page_content', {
        skipGlobalLoading: true,
        skipErrorHandler: true,
      });
      const { success, message, data } = res.data;
      if (success) {
        const content = data || '';
        setHomePageContent(content);
        if (content) {
          localStorage.setItem('home_page_content', content);
        } else {
          localStorage.removeItem('home_page_content');
        }

        if (isRemoteHomePage(data || '')) {
          const iframe = document.querySelector('iframe');
          if (iframe) {
            iframe.onload = () => {
              iframe.contentWindow.postMessage({ themeMode: actualTheme }, '*');
              iframe.contentWindow.postMessage({ lang: language }, '*');
            };
          }
        }
      } else {
        console.error(message);
      }
    } catch (error) {
      if (cachedContent) return;
      console.error('加载首页内容失败:', error);
    } finally {
      setHomePageResolved(true);
    }
  };

  const handleCopyEndpoint = async () => {
    return copy(`${publicHomeServerAddress}${currentEndpoint}`);
  };

  const handleOpenDocs = () => {
    if (!docsLink) return;
    if (docsLink.startsWith('/')) {
      navigate(docsLink);
      return;
    }
    window.open(docsLink, '_blank', 'noopener,noreferrer');
  };

  useEffect(() => {
    const checkNoticeAndShow = async () => {
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const today = new Date().toDateString();
      if (lastCloseDate !== today) {
        try {
          const res = await API.get('/api/notice', {
            skipGlobalLoading: true,
            skipErrorHandler: true,
          });
          const { success, data } = res.data;
          if (success && data && data.trim() !== '') {
            setNoticeContent(data.trim());
            setNoticeVisible(true);
          }
        } catch (error) {
          console.error('获取公告失败:', error);
        }
      }
    };

    const taskId = window.setTimeout(checkNoticeAndShow, 4500);
    return () => window.clearTimeout(taskId);
  }, []);

  useEffect(() => {
    displayHomePageContent().then();
  }, []);

  // 上报当前是否为自定义首页（自带顶部导航）：homePageContent 非空即为远程 iframe
  // 或自定义 HTML，二者都自带 nav。外层 PublicHomeShell 据此隐藏项目 public-header，
  // 避免与自定义 nav 叠成双顶栏。onLandingChange 可选，App.jsx 内联使用 Home 时不传。
  useEffect(() => {
    if (typeof onLandingChange === 'function') {
      onLandingChange(homePageContent !== '');
    }
  }, [homePageContent, onLandingChange]);

  if (!homePageResolved) {
    return (
      <div
        className='w-full min-h-[calc(100vh-64px)] bg-[var(--semi-color-bg-0,#fff)]'
        aria-busy='true'
      />
    );
  }

  return (
    <div className='w-full overflow-x-hidden'>
      {noticeVisible && (
        <Suspense fallback={null}>
          <PublicNoticeModal
            visible={noticeVisible}
            content={noticeContent}
            onClose={() => setNoticeVisible(false)}
          />
        </Suspense>
      )}
      {useDefaultHome ? (
        <EnterpriseHome
          brandName={brandName}
          brandLogo={brandLogo}
          currentEndpoint={currentEndpoint}
          endpointItems={endpointItems}
          handleCopyEndpoint={handleCopyEndpoint}
          handleOpenDocs={docsLink ? handleOpenDocs : null}
          serverAddress={publicHomeServerAddress}
          setEndpointIndex={setEndpointIndex}
          t={t}
        />
      ) : (
        <div className='overflow-x-hidden w-full'>
          {isRemoteHomePage(homePageContent) ? (
            <iframe
              src={homePageContent}
              className='w-full h-screen border-none'
            />
          ) : (
            // 自定义首页 HTML 自带顶部 nav 时，PageLayout 已经隐藏了全局 Header，
            // 这里也不再加 mt-[60px] 顶部留白，避免与自定义 nav 之间出现空白带。
            <RichContent content={homePageContent} />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
