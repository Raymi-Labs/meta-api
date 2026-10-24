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
import React, { useState, useSyncExternalStore } from 'react';
import { Check, LockKeyhole, BarChart3 } from 'lucide-react';
import {
  getAnalyticsConsentSnapshot,
  subscribeAnalyticsConsent,
  saveAnalyticsConsent,
} from '../../helpers/analyticsConsent';

export default function PrivacyChoices({ t }) {
  const consent = useSyncExternalStore(
    subscribeAnalyticsConsent,
    getAnalyticsConsentSnapshot,
    getAnalyticsConsentSnapshot,
  );
  const [message, setMessage] = useState(null);
  const save = (allowed) => {
    const result = saveAnalyticsConsent(allowed);
    setMessage(result.success ? 'saved' : 'error');
  };
  const choices = {
    unset: '尚未选择，统计已关闭',
    granted: '已允许',
    denied: '已拒绝',
  };
  return (
    <div className='privacy-choices'>
      <p className='public-legal-intro'>
        {t('偏好仅保存在当前浏览器，不影响 API 调用和账务记录。')}
      </p>
      <section className='privacy-choice-card'>
        <div className='privacy-choice-heading'>
          <LockKeyhole aria-hidden='true' size={22} />
          <h2>{t('必要存储')}</h2>
          <span className='privacy-choice-badge'>
            <Check aria-hidden='true' size={14} /> {t('始终启用')}
          </span>
        </div>
        <p>{t('用于登录、安全、语言和主题等基本功能。')}</p>
      </section>
      <section className='privacy-choice-card'>
        <div className='privacy-choice-heading'>
          <BarChart3 aria-hidden='true' size={22} />
          <h2>{t('可选访问统计')}</h2>
        </div>
        <p>{t('允许统计脚本收集访问情况，帮助改进网站。')}</p>
        <p className='privacy-choice-state'>
          {t('当前选择')}：<strong>{t(choices[consent.choice])}</strong>
        </p>
        {!consent.configured && <p>{t('本站未配置可选统计服务。')}</p>}
        <div className='privacy-choice-actions'>
          <button
            type='button'
            aria-pressed={consent.choice === 'denied'}
            onClick={() => save(false)}
          >
            {t('拒绝访问统计')}
          </button>
          <button
            type='button'
            aria-pressed={consent.choice === 'granted'}
            onClick={() => save(true)}
          >
            {t('允许访问统计')}
          </button>
        </div>
        <p className='privacy-choice-hint'>
          {t('更改已运行的统计脚本会刷新页面。')}
        </p>
      </section>
      <p
        className={`privacy-choice-feedback ${message === 'error' ? 'privacy-choice-feedback--error' : ''}`}
        role={message === 'error' ? 'alert' : 'status'}
      >
        {message === 'error'
          ? t('无法保存偏好，请检查浏览器存储设置。')
          : message === 'saved'
            ? t('偏好已保存')
            : '\u00a0'}
      </p>
    </div>
  );
}
