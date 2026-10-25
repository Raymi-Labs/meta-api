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

import React, { useState } from 'react';
import { Checkbox } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { hasRequiredAuthConsent } from '../../helpers/authConsent';
import { showInfo } from '../../helpers/toast';

export function useAuthConsent(status, t) {
  const [consent, setConsent] = useState({ agreement: false, privacy: false });
  const hasConsent = hasRequiredAuthConsent(status, consent);
  const requireConsent = () => {
    if (!hasConsent) showInfo(t('请先阅读并同意用户协议和隐私政策'));
    return hasConsent;
  };
  return { consent, setConsent, hasConsent, requireConsent };
}

export default function AuthConsent({ status, value, onChange }) {
  const { t } = useTranslation();
  const policies = [
    {
      key: 'agreement',
      enabled: status.user_agreement_enabled,
      href: '/user-agreement',
      title: t('用户协议'),
    },
    {
      key: 'privacy',
      enabled: status.privacy_policy_enabled,
      href: '/privacy-policy',
      title: t('隐私政策'),
    },
  ].filter((policy) => policy.enabled);
  if (!policies.length) return null;
  return (
    <div className='auth-consent'>
      {policies.map(({ key, href, title }) => (
        <Checkbox
          key={key}
          checked={value[key]}
          onChange={(event) =>
            onChange((current) => ({ ...current, [key]: event.target.checked }))
          }
        >
          <span>
            {t('我已阅读并同意')}{' '}
            <a
              href={href}
              target='_blank'
              rel='noopener noreferrer'
              onClick={(event) => event.stopPropagation()}
            >
              {title}
            </a>
          </span>
        </Checkbox>
      ))}
    </div>
  );
}
