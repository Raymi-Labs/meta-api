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
import { renderQuota } from '../../../helpers';
import { useEnterpriseSelf } from '../../../hooks/enterprise/useEnterpriseSelf';
import './headerBalance.css';

// This is a view of the shared account snapshot, not another balance fetcher.
export default function HeaderBalance({ user, loading, t }) {
  const enterprise = useEnterpriseSelf();
  if (!user) return null;
  const quota = user.quota;
  const quotaPerUnit = Number(localStorage.getItem('quota_per_unit'));
  const known =
    !loading &&
    typeof quota === 'number' &&
    Number.isFinite(quota) &&
    (localStorage.getItem('quota_display_type') === 'TOKENS' ||
      (Number.isFinite(quotaPerUnit) && quotaPerUnit > 0));
  let threshold = 0;
  try {
    const settings = JSON.parse(user.setting || '{}');
    const value = settings?.quota_warning_threshold;
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
      threshold = value;
    }
  } catch {
    // A malformed preference must not break the shared header.
  }
  const low = known && (quota <= 0 || quota < threshold);
  const amount = known ? renderQuota(quota) : '—';
  const label = `${t('余额')} ${amount}`;
  const enterpriseAvailable = enterprise.data?.member?.available;
  const enterpriseKnown =
    known &&
    typeof enterpriseAvailable === 'number' &&
    Number.isFinite(enterpriseAvailable);
  const enterpriseLow = enterpriseKnown && enterpriseAvailable <= 0;
  const enterpriseLabel = enterpriseKnown
    ? [
        `${t('企业')} ${renderQuota(enterpriseAvailable)}`,
        enterprise.data?.enterprise?.name,
        t('我的额度'),
      ]
        .filter(Boolean)
        .join(' · ')
    : '';
  return (
    <>
      {enterpriseKnown && (
        <Link
          to='/console/enterprise'
          className={`header-balance header-balance-enterprise${enterpriseLow ? ' header-balance-low' : ''}`}
          title={enterpriseLabel}
          aria-label={enterpriseLabel}
        >
          <span className='header-balance-label'>{t('企业')}</span>
          <span className='header-balance-amount'>
            {renderQuota(enterpriseAvailable)}
          </span>
        </Link>
      )}
      <Link
        to='/console/topup'
        className={`header-balance${low ? ' header-balance-low' : ''}`}
        title={`${label} · ${t('钱包管理')}${low ? ` · ${t('余额不足')}` : ''}`}
        aria-label={`${label} · ${t('钱包管理')}${low ? ` · ${t('余额不足')}` : ''}`}
        aria-busy={loading || undefined}
      >
        <span className='header-balance-label'>{t('余额')}</span>
        <span className='header-balance-amount'>{amount}</span>
      </Link>
    </>
  );
}
