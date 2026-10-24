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
import { Link } from 'react-router-dom';
import { prefetchAppShell } from '../../helpers/prefetchAppShell';
import './PublicFooter.css';

const PublicFooter = ({
  systemName,
  logo,
  modules,
  docsLink,
  user,
  status,
  customLanding,
  isDownloadPage,
  t,
}) => {
  const copyright = (
    <p className='public-footer-copyright'>
      © {new Date().getFullYear()} {systemName}. {t('版权所有')}
    </p>
  );

  // Custom landing content owns its navigation and page sections.
  if (customLanding && !isDownloadPage) {
    return (
      <footer className='public-footer public-footer--compact'>
        {copyright}
      </footer>
    );
  }

  const pricingEnabled =
    typeof modules.pricing === 'object'
      ? modules.pricing?.enabled
      : modules.pricing;
  const showHomeSections = !customLanding && modules.home;
  const groups = [
    {
      id: 'products',
      title: '产品',
      links: [
        pricingEnabled && {
          text: '模型广场',
          to: modules.pricing?.requireAuth && !user ? '/login' : '/pricing',
        },
        { text: '下载客户端', to: '/download', prefetchApp: false },
      ],
    },
    {
      id: 'developers',
      title: '开发者',
      links: [
        modules.docs &&
          docsLink && { text: '文档', href: docsLink, external: true },
        showHomeSections && { text: '快速开始', href: '/#quick-start' },
        modules.console && {
          text: '控制台',
          to: user ? '/console' : '/login',
        },
        modules.usage && { text: '用量查询', to: '/usage' },
      ],
    },
    {
      id: 'about',
      title: '关于我们',
      links: [
        modules.about && { text: '关于', to: '/about' },
        showHomeSections && { text: '常见问题', href: '/#faq-title' },
      ],
    },
    {
      id: 'policies',
      title: '条款与政策',
      links: [
        status.user_agreement_enabled && {
          text: '用户协议',
          to: '/user-agreement',
        },
        status.privacy_policy_enabled && {
          text: '隐私政策',
          to: '/privacy-policy',
        },
      ],
    },
  ]
    .map((group) => ({ ...group, links: group.links.filter(Boolean) }))
    .filter((group) => group.links.length > 0);

  return (
    <footer className='public-footer public-footer--directory'>
      <div className='public-footer-inner'>
        <div className='public-footer-identity'>
          <Link to='/' className='public-footer-brand'>
            <img src={logo} alt='' width='36' height='36' />
            <span>{systemName}</span>
          </Link>
          <p className='public-footer-tagline'>
            {t('一个 API，')}
            {t('连接万千可能。')}
          </p>
          {copyright}
        </div>
        <nav
          className='public-footer-navigation'
          style={{ '--public-footer-columns': groups.length }}
          aria-label={t('页脚导航')}
        >
          {groups.map((group) => (
            <section
              className='public-footer-group'
              key={group.id}
              aria-labelledby={`public-footer-${group.id}`}
            >
              <h2 id={`public-footer-${group.id}`}>{t(group.title)}</h2>
              <ul>
                {group.links.map((link) => (
                  <li key={link.text}>
                    {link.href ? (
                      <a
                        href={link.href}
                        target={link.external ? '_blank' : undefined}
                        rel={link.external ? 'noopener noreferrer' : undefined}
                      >
                        {t(link.text)}
                      </a>
                    ) : (
                      <Link
                        to={link.to}
                        onMouseEnter={
                          link.prefetchApp === false
                            ? undefined
                            : prefetchAppShell
                        }
                        onTouchStart={
                          link.prefetchApp === false
                            ? undefined
                            : prefetchAppShell
                        }
                      >
                        {t(link.text)}
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </nav>
      </div>
    </footer>
  );
};

export default PublicFooter;
