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
import { Button, Modal, Radio, RadioGroup, TextArea } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { copy } from '../../../helpers/clipboard';
import { showError, showSuccess } from '../../../helpers/toast';
import {
  buildInvitePromotionHTML,
  INVITE_PROMOTION_SOURCES,
} from '../../../helpers/invitePromotion';
import InvitePromotionStats from '../../common/invitation/InvitePromotionStats';

export default function InvitePromotionModal({ visible, onCancel, affLink }) {
  const { t } = useTranslation();
  const [type, setType] = useState('banner');
  const html = buildInvitePromotionHTML(affLink, type, t);
  return (
    <Modal
      title={t('推广素材与点击统计')}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width='min(760px, calc(100vw - 24px))'
      bodyStyle={{ maxHeight: 'calc(100dvh - 160px)', overflowY: 'auto' }}
    >
      {visible && (
        <div className='space-y-6'>
          <InvitePromotionStats endpoint='/api/user/invite-promotion/self' />
          <section className='space-y-3' aria-label={t('HTML 广告代码')}>
            <h3 className='font-semibold'>{t('HTML 广告代码')}</h3>
            <p className='text-sm text-semi-color-text-2'>
              {t(
                '复制到博客或网站即可展示，已包含你的邀请码。修改邀请码后，请同步更新已投放的代码。',
              )}
            </p>
            <RadioGroup
              type='button'
              value={type}
              onChange={(event) => setType(event.target.value)}
              aria-label={t('素材类型')}
            >
              {['text', 'button', 'banner'].map((key) => (
                <Radio key={key} value={key}>
                  {t(INVITE_PROMOTION_SOURCES[key])}
                </Radio>
              ))}
            </RadioGroup>
            {html ? (
              <>
                <iframe
                  title={t('广告预览')}
                  sandbox=''
                  srcDoc={`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="margin:8px">${html}</body></html>`}
                  className='w-full rounded-xl border border-semi-color-border bg-semi-color-white'
                  style={{ height: type === 'banner' ? 500 : 120 }}
                />
                <TextArea
                  value={html}
                  readOnly
                  rows={5}
                  aria-label={t('HTML 广告代码')}
                />
                <Button
                  theme='solid'
                  onClick={async () => {
                    if (await copy(html)) showSuccess(t('复制成功'));
                    else showError(t('复制失败'));
                  }}
                >
                  {t('复制 HTML 代码')}
                </Button>
                <p className='text-xs text-semi-color-text-2'>
                  {t(
                    '预览不产生点击统计；实际嵌入网站后，访问广告链接才计数。',
                  )}
                </p>
              </>
            ) : (
              <p role='alert'>{t('邀请链接不可用，请刷新页面后重试')}</p>
            )}
          </section>
        </div>
      )}
    </Modal>
  );
}
