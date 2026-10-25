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
import { ArrowLeft, ArrowUpRight, ShieldCheck } from 'lucide-react';
import { Link, useLocation } from 'react-router-dom';
import { usePublicTranslation } from '../../helpers/publicLocale';
import { PUBLIC_LANGUAGES } from '../../helpers/publicLanguage';
import { useContentLanguage } from '../../helpers/contentLanguageReact';
import { PUBLIC_POLICIES, POLICY_UPDATED_AT, getPublicPolicy } from './content';
import PrivacyChoices from './PrivacyChoices';
import './publicLegal.css';

export default function PublicLegal({ status = {} }) {
  const { t, language } = usePublicTranslation();
  const bodyLanguage = useContentLanguage(language);
  const bodyLang = bodyLanguage.ready ? language : 'zh-CN';
  const bodyLanguageName = PUBLIC_LANGUAGES.find(
    ([code]) => code === bodyLang,
  )?.[1];
  const { pathname } = useLocation();
  const privacyChoices = /^\/privacy-choices\/?$/.test(pathname);
  const slug = pathname.replace(/^\/legal\/?/, '').replace(/\/$/, '');
  const policy = getPublicPolicy(slug);
  const title = privacyChoices ? '你的隐私选择' : policy?.title || '文档不存在';
  const links = [
    status.user_agreement_enabled && {
      title: '服务条款',
      to: '/user-agreement',
    },
    status.privacy_policy_enabled && {
      title: '隐私政策',
      to: '/privacy-policy',
    },
    ...PUBLIC_POLICIES.map((document) => ({
      title: document.title,
      to: `/legal/${document.slug}`,
    })),
    { title: '你的隐私选择', to: '/privacy-choices' },
  ].filter(Boolean);

  return (
    <main className='public-legal'>
      <div className='public-legal-masthead'>
        <Link to='/' className='public-legal-back'>
          <ArrowLeft size={16} aria-hidden='true' /> {t('返回首页')}
        </Link>
        <div className='public-legal-eyebrow'>
          <ShieldCheck size={18} aria-hidden='true' /> {t('条款与政策')}
        </div>
        <h1>{t(title)}</h1>
        {policy && (
          <p className='public-legal-meta'>
            <span>
              {t('正文语言')}：{bodyLanguageName}
            </span>
            <span>
              {t('更新日期')}{' '}
              <time dateTime={POLICY_UPDATED_AT}>{POLICY_UPDATED_AT}</time>
            </span>
          </p>
        )}
      </div>
      <div className='public-legal-layout'>
        <nav className='public-legal-nav' aria-label={t('政策说明')}>
          {links.map((link) => (
            <Link
              key={link.to}
              to={link.to}
              aria-current={
                pathname.replace(/\/$/, '') === link.to ? 'page' : undefined
              }
            >
              {t(link.title)}
              <ArrowUpRight size={14} aria-hidden='true' />
            </Link>
          ))}
        </nav>
        <div className='public-legal-body'>
          {privacyChoices ? (
            <PrivacyChoices t={t} />
          ) : policy ? (
            <article lang={bodyLang}>
              {(bodyLanguage.pending || bodyLanguage.failed) && (
                <p className='public-legal-translation' role='status'>
                  {t(
                    bodyLanguage.failed
                      ? '正文翻译加载失败，请重试。'
                      : '正在加载正文翻译…',
                  )}
                  {bodyLanguage.failed && (
                    <button type='button' onClick={bodyLanguage.retry}>
                      {t('重试')}
                    </button>
                  )}
                </p>
              )}
              <p className='public-legal-intro'>
                {bodyLanguage.translate(policy.intro)}
              </p>
              {policy.sections.map((section) => (
                <section key={section.title}>
                  <h2>{bodyLanguage.translate(section.title)}</h2>
                  {section.paragraphs.map((paragraph) => (
                    <p key={paragraph}>{bodyLanguage.translate(paragraph)}</p>
                  ))}
                </section>
              ))}
            </article>
          ) : (
            <p className='public-legal-intro'>{t('请选择其他政策文档。')}</p>
          )}
        </div>
      </div>
    </main>
  );
}
