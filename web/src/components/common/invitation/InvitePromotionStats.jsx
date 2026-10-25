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

import React, { useEffect, useState } from 'react';
import { Button, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/apiCore';
import { INVITE_PROMOTION_SOURCES } from '../../../helpers/invitePromotion';

export default function InvitePromotionStats({ endpoint }) {
  const { t } = useTranslation();
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(false);
    setResult(null);
    API.get(endpoint)
      .then(({ data }) => {
        if (!active) return;
        if (!data?.success || !data.data) throw new Error('unavailable');
        setResult(data.data);
      })
      .catch(() => {
        if (active) setError(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [endpoint, revision]);
  return (
    <section aria-label={t('推广点击统计')} className='space-y-3'>
      <div className='flex items-center justify-between gap-2'>
        <strong>{t('近30天点击')}</strong>
        <Button
          size='small'
          theme='borderless'
          disabled={loading}
          onClick={() => setRevision((n) => n + 1)}
        >
          {t('刷新')}
        </Button>
      </div>
      <Spin spinning={loading}>
        {error ? (
          <p role='alert'>{t('点击统计加载失败，请刷新重试')}</p>
        ) : result ? (
          <>
            <div className='text-3xl font-bold text-semi-color-success'>
              {result.clicks}
            </div>
            <p className='text-xs text-semi-color-text-2'>
              {result.from} — {result.to} (UTC)
            </p>
            <div className='grid grid-cols-2 sm:grid-cols-4 gap-2'>
              {Object.entries(INVITE_PROMOTION_SOURCES).map(
                ([source, label]) => (
                  <div
                    key={source}
                    className='rounded-xl border border-semi-color-border p-3'
                  >
                    <span className='text-xs text-semi-color-text-2'>
                      {t(label)}
                    </span>
                    <div className='text-lg font-semibold'>
                      {result.sources?.[source] || 0}
                    </div>
                  </div>
                ),
              )}
            </div>
            <details>
              <summary className='cursor-pointer text-sm'>
                {t('每日点击明细')}
              </summary>
              <div className='max-h-48 overflow-auto mt-2'>
                <table className='w-full text-sm'>
                  <thead>
                    <tr>
                      <th className='text-left'>{t('日期')}</th>
                      <th className='text-right'>{t('点击次数')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[...(result.daily || [])].reverse().map((day) => (
                      <tr key={day.day}>
                        <td className='py-1'>{day.day}</td>
                        <td className='text-right'>{day.clicks}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </details>
          </>
        ) : (
          <p className='text-sm text-semi-color-text-2'>{t('加载中...')}</p>
        )}
      </Spin>
      <p className='text-xs leading-5 text-semi-color-text-2'>
        {t(
          '仅统计功能上线后的短邀请链接访问；重复访问可能计入，不代表独立访客或注册人数。',
        )}
      </p>
    </section>
  );
}
