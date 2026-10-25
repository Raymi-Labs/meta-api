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
import { buildAuthPathWithNext } from '../../helpers/authRedirect';
import { withDocsLanguage } from '../../helpers/docsLink';
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
  isLegalPage,
  isEnterprisePage,
  t,
}) => {
  const copyright = (
    <p className='public-footer-copyright'>
      © {new Date().getFullYear()} {systemName}. {t('版权所有')}
    </p>
  );

  // Custom landing content owns its navigation and page sections.
  if (customLanding && !isDownloadPage && !isLegalPage && !isEnterprisePage) {
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
  const showDocs = modules.docs && Boolean(docsLink);
  const protectedPath = (path) =>
    user ? path : buildAuthPathWithNext('/login', path);
  const groups = [
    {
      id: 'products',
      title: '产品',
      links: [
        pricingEnabled && {
          text: '模型广场',
          to: modules.pricing?.requireAuth
            ? protectedPath('/pricing')
            : '/pricing',
        },
        modules.console && {
          text: '智能路由',
          to: protectedPath('/console/token'),
        },
        modules.console && {
          text: '模型体验',
          to: protectedPath('/console/playground'),
        },
        modules.console && {
          text: 'AI 图像创作',
          to: protectedPath('/console/image-playground'),
        },
        { text: '下载客户端', to: '/download', prefetchApp: false },
        { text: '企业定制', to: '/enterprise', prefetchApp: false },
        showDocs && { text: 'Skills', to: '/docs/image-generation' },
      ],
    },
    {
      id: 'developers',
      title: '开发者',
      links: [
        showDocs && {
          text: '文档',
          href: withDocsLanguage(docsLink),
          external: true,
        },
        showDocs && { text: 'API 参考', to: '/docs/api-examples' },
        showDocs && { text: '接入指南', to: '/docs/clients' },
        showDocs && { text: 'Agent 接入', to: '/docs/clients/openclaw' },
        showHomeSections && { text: '快速开始', href: '/#quick-start' },
        modules.console && {
          text: '控制台',
          to: protectedPath('/console'),
        },
        modules.usage && { text: '用量查询', to: '/usage' },
      ],
    },
    {
      id: 'about',
      title: '关于我们',
      links: [
        modules.about && { text: '关于', to: '/about' },
        showDocs && { text: '支持', to: '/docs/troubleshooting' },
        showHomeSections && { text: '常见问题', href: '/#faq-title' },
      ],
    },
    {
      id: 'policies',
      title: '条款与政策',
      links: [
        status.user_agreement_enabled && {
          text: '服务条款',
          to: '/user-agreement',
        },
        status.privacy_policy_enabled && {
          text: '隐私政策',
          to: '/privacy-policy',
        },
        {
          text: 'Cookie 政策',
          to: '/legal/cookies',
          prefetchApp: false,
        },
        {
          text: '数据隐私',
          to: '/legal/data-privacy',
          prefetchApp: false,
        },
        {
          text: '支付须知',
          to: '/legal/payment',
          prefetchApp: false,
        },
        {
          text: 'ZDR 条款',
          to: '/legal/zdr',
          prefetchApp: false,
        },
        {
          text: '异步生成条款',
          to: '/legal/asynchronous-generation',
          prefetchApp: false,
        },
        {
          text: '你的隐私选择',
          to: '/privacy-choices',
          prefetchApp: false,
          privacyChoice: true,
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
                        className={
                          link.privacyChoice
                            ? 'public-footer-privacy-choice'
                            : undefined
                        }
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
                        <span>{t(link.text)}</span>
                        {link.privacyChoice && (
                          <svg
                            width='16'
                            height='18'
                            viewBox='0 0 16 18'
                            fill='none'
                            aria-hidden='true'
                            focusable='false'
                          >
                            <path
                              d='M8 1.5 14 4v5c0 3.2-3.8 6.1-6 7.5C5.8 15.1 2 12.2 2 9V4l6-2.5Z'
                              stroke='currentColor'
                              strokeWidth='1.5'
                              strokeLinejoin='round'
                            />
                            <path
                              d='m5.5 8.5 1.8 1.8 3.2-3.8'
                              stroke='currentColor'
                              strokeWidth='1.5'
                              strokeLinecap='round'
                              strokeLinejoin='round'
                            />
                          </svg>
                        )}
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
