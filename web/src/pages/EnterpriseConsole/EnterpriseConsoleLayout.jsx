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
import { Link, NavLink, Navigate, Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button, Tag } from '@douyinfe/semi-ui';
import {
  BarChart3,
  ChevronLeft,
  ClipboardCheck,
  LayoutDashboard,
  Settings,
  Users,
} from 'lucide-react';
import { useEnterpriseSelf } from '../../hooks/enterprise/useEnterpriseSelf';
import { renderQuota } from '../../helpers/render';
import './enterpriseConsole.css';

const NAV_ITEMS = [
  {
    to: '/enterprise/console',
    end: true,
    label: '总览',
    Icon: LayoutDashboard,
  },
  { to: '/enterprise/console/members', label: '成员', Icon: Users },
  {
    to: '/enterprise/console/approvals',
    label: '审批',
    Icon: ClipboardCheck,
    badge: true,
  },
  { to: '/enterprise/console/usage', label: '用量', Icon: BarChart3 },
  { to: '/enterprise/console/settings', label: '设置', Icon: Settings },
];

export default function EnterpriseConsoleLayout() {
  const { t } = useTranslation();
  const ent = useEnterpriseSelf();

  if (!ent.enabled || ent.loading) return null;
  if (ent.error)
    return (
      <div className='p-6' role='alert'>
        <p>{t(ent.error)}</p>
        <Button onClick={ent.refresh}>{t('重试')}</Button>
      </div>
    );
  if (!ent.isManager) return <Navigate to='/console/enterprise' replace />;

  const { enterprise, member } = ent.data;
  const unallocated = Number(ent.data.unallocated) || 0;
  const context = {
    enterpriseId: enterprise.id,
    enterprise,
    unallocated,
    role: member.role,
    memberId: member.id,
    refreshSelf: ent.refresh,
  };

  return (
    <div className='ec-shell'>
      <header className='ec-top'>
        <div className='ec-top-left'>
          <Link to='/console' className='ec-back'>
            <ChevronLeft size={18} aria-hidden='true' />
            {t('返回控制台')}
          </Link>
          <span className='ec-divider' aria-hidden='true' />
          <span className='ec-mark' aria-hidden='true'>
            {Array.from(enterprise.name || '企')[0]}
          </span>
          <strong className='ec-name'>{enterprise.name}</strong>
          <Tag size='small'>{t('企业工作台')}</Tag>
          {enterprise.status === 'frozen' && (
            <Tag size='small' color='red'>
              {t('已冻结')}
            </Tag>
          )}
        </div>
        <div className='ec-top-right'>
          <span className={`ec-chip${unallocated <= 0 ? ' low' : ''}`}>
            <span>{t('未分配')}</span>
            <strong>{renderQuota(unallocated)}</strong>
          </span>
        </div>
      </header>
      <div className='ec-body'>
        <nav className='ec-side' aria-label={t('企业工作台')}>
          {NAV_ITEMS.map(({ to, end, label, Icon, badge }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) => `ec-nav${isActive ? ' active' : ''}`}
            >
              <Icon size={18} aria-hidden='true' />
              <span>{t(label)}</span>
              {badge && ent.pendingRequests > 0 && (
                <em className='ec-badge'>{ent.pendingRequests}</em>
              )}
            </NavLink>
          ))}
        </nav>
        <main className='ec-main'>
          <Outlet key={`${enterprise.id}:${member.id}`} context={context} />
        </main>
      </div>
    </div>
  );
}
