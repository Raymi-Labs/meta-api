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

import React, { useContext, useEffect } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowRight,
  ArrowUpRight,
  Wallet,
  KeyRound,
  Users,
  ClipboardCheck,
  ChartNoAxesCombined,
} from 'lucide-react';
import { UserContext } from '../../context/User';
import { usePublicTranslation } from '../../helpers/publicLocale';
import { useEnterpriseSelf } from '../../hooks/enterprise/useEnterpriseSelf';
import { getSystemName } from '../../helpers/storage';
import { buildAuthPathWithNext } from '../../helpers/authRedirect';
import { ENTERPRISE_CONTACT } from './enterpriseData';
import { getEnterpriseEntry } from './enterpriseEntry';
import './Enterprise.css';

const capabilities = [
  {
    Icon: Wallet,
    title: '企业余额',
    description: '企业统一入账，分配到成员额度，不转入个人钱包。',
  },
  {
    Icon: Users,
    title: '成员管理',
    description: '邀请成员、分配额度，按企业与成员设置模型范围。',
  },
  {
    Icon: ClipboardCheck,
    title: '审批',
    description: '成员申请追加额度，批准后从企业未分配余额划拨。',
  },
  {
    Icon: ChartNoAxesCombined,
    title: '用量',
    description: '按成员、模型与时间核对消费，查看记录并导出。',
  },
];
const roles = [
  {
    role: '所有者',
    text: '开通时指定；管理企业与管理员，可分配本人额度、审批本人申请。',
  },
  { role: '管理员', text: '管理普通成员与审批；自己的额度申请由所有者处理。' },
  { role: '成员', text: '使用企业额度、创建企业令牌，申请追加额度。' },
];

export function EnterpriseEntry({ user, workspace, t, contactHref }) {
  const entry = getEnterpriseEntry(user, workspace);
  return (
    <aside className='enterprise-entry' aria-label={t('企业工作空间')}>
      {entry.member && (
        <div className='enterprise-entry-heading'>
          <p className='enterprise-name'>{entry.name}</p>
          <span className='enterprise-role'>{t(entry.role)}</span>
        </div>
      )}
      {entry.frozen && <p className='enterprise-frozen'>{t('已冻结')}</p>}
      {entry.state === 'loading' ? (
        <p role='status'>{t('加载中...')}</p>
      ) : entry.state === 'error' ? (
        <div role='alert'>
          <p>{t('企业信息读取失败，请重试')}</p>
          <button
            className='enterprise-button'
            type='button'
            onClick={workspace.refresh}
          >
            {t('重试')}
          </button>
        </div>
      ) : (
        <div className='enterprise-entry-actions'>
          {entry.to ? (
            <Link
              className='enterprise-button enterprise-primary'
              to={entry.to}
            >
              {t(entry.label)}
              <ArrowRight size={18} aria-hidden='true' />
            </Link>
          ) : (
            <>
              <a
                className='enterprise-button enterprise-primary'
                href={contactHref}
              >
                {t('联系开通')}
              </a>
            </>
          )}
          {entry.member && (
            <Link
              className='enterprise-button enterprise-secondary'
              to='/console/token'
            >
              {t('令牌管理')}
            </Link>
          )}
          {!user?.id && (
            <Link
              className='enterprise-button enterprise-secondary'
              to={buildAuthPathWithNext('/login', '/enterprise')}
            >
              {t('登录')}
            </Link>
          )}
        </div>
      )}
      {entry.platform && entry.to !== '/console/enterprise-admin' && (
        <Link
          className='enterprise-platform-link'
          to='/console/enterprise-admin'
        >
          {t('企业管理')}
          <ArrowUpRight size={16} aria-hidden='true' />
        </Link>
      )}
    </aside>
  );
}

export default function Enterprise() {
  const { t, language } = usePublicTranslation();
  const [userState] = useContext(UserContext);
  const workspace = useEnterpriseSelf();
  const brandName = getSystemName();
  const contactHref = `mailto:${ENTERPRISE_CONTACT}?subject=${encodeURIComponent(t('企业接入咨询'))}`;
  useEffect(() => {
    const previous = document.title;
    document.title = `${t('企业服务')} · ${brandName}`;
    return () => {
      document.title = previous;
    };
  }, [brandName, language, t]);

  return (
    <main className='enterprise-page' lang={language}>
      <section
        className='enterprise-container enterprise-hero'
        aria-labelledby='enterprise-title'
      >
        <div className='enterprise-intro'>
          <h1 id='enterprise-title'>{t('企业服务')}</h1>
          <p className='enterprise-lead'>
            {t('企业统一付款，成员按额度使用。')}
          </p>
          <EnterpriseEntry
            user={userState?.user}
            workspace={workspace}
            t={t}
            contactHref={contactHref}
          />
        </div>
        <div className='enterprise-flow-panel'>
          <ol className='enterprise-funding-flow' aria-label={t('额度分配')}>
            <li>
              <span className='enterprise-flow-icon'>
                <Wallet size={22} aria-hidden='true' />
              </span>
              <span>{t('未分配余额')}</span>
            </li>
            <li>
              <span className='enterprise-flow-icon'>
                <Users size={22} aria-hidden='true' />
              </span>
              <span>{t('成员额度')}</span>
            </li>
            <li>
              <span className='enterprise-flow-icon'>
                <KeyRound size={22} aria-hidden='true' />
              </span>
              <span>{t('企业令牌')}</span>
            </li>
          </ol>
          <Link className='enterprise-text-link' to='/docs/api-examples'>
            {t('接入文档')}
            <ArrowUpRight size={16} aria-hidden='true' />
          </Link>
        </div>
      </section>

      <section
        className='enterprise-management'
        aria-labelledby='enterprise-management-title'
      >
        <div className='enterprise-container enterprise-management-grid'>
          <div className='enterprise-section-heading'>
            <h2 id='enterprise-management-title'>{t('团队管理')}</h2>
            <p>{t('企业令牌目前支持文本、图片、音频、向量与重排。')}</p>
          </div>
          <div className='enterprise-capabilities'>
            {capabilities.map(({ Icon, title, description }) => (
              <article className='enterprise-capability' key={title}>
                <Icon size={23} strokeWidth={1.65} aria-hidden='true' />
                <div>
                  <h3>{t(title)}</h3>
                  <p>{t(description)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section
        className='enterprise-container enterprise-roles-section'
        aria-labelledby='enterprise-roles-title'
      >
        <h2 id='enterprise-roles-title'>{t('角色与额度')}</h2>
        <dl className='enterprise-roles'>
          {roles.map(({ role, text }) => (
            <div key={role}>
              <dt>{t(role)}</dt>
              <dd>{t(text)}</dd>
            </div>
          ))}
        </dl>
      </section>
      <section
        className='enterprise-container enterprise-contact'
        aria-labelledby='enterprise-contact-title'
      >
        <h2 id='enterprise-contact-title'>{t('定制需求')}</h2>
        <a className='enterprise-text-link' href={contactHref}>
          {t('发送邮件咨询')}
          <ArrowUpRight size={17} aria-hidden='true' />
        </a>
      </section>
    </main>
  );
}
