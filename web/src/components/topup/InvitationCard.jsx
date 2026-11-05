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
import { Card, Button, Input, Space } from '@douyinfe/semi-ui';
import { Copy } from 'lucide-react';
import InvitePromotionModal from './modals/InvitePromotionModal';

const InvitationCard = ({
  t,
  userState,
  renderQuota,
  onOpenTransfer,
  onOpenWithdrawal,
  openWithdrawalHistory,
  affLink,
  affCode,
  onSaveAffCode,
  handleAffLinkClick,
  onOpenInviteCommissions,
}) => {
  const [newCode, setNewCode] = useState('');
  const [promotionVisible, setPromotionVisible] = useState(false);
  const [savingCode, setSavingCode] = useState(false);
  const hasAvailableReward = Number(userState?.user?.aff_quota || 0) > 0;
  return (
    <Card className='wallet-invitation-card' bodyStyle={{ padding: 0 }}>
      <section className='wallet-invitation-summary'>
        <div className='wallet-summary-heading'>
          <h2>{t('邀请奖励')}</h2>
          <Button
            theme='borderless'
            size='small'
            onClick={onOpenInviteCommissions}
          >
            {t('奖励明细')}
          </Button>
        </div>
        <div className='wallet-reward-balance'>
          <span>{t('待使用收益')}</span>
          <strong>{renderQuota(userState?.user?.aff_quota || 0)}</strong>
        </div>
        <div className='wallet-reward-statistics'>
          <div>
            <span>{t('总收益')}</span>
            <strong>
              {renderQuota(userState?.user?.aff_history_quota || 0)}
            </strong>
          </div>
          <button type='button' onClick={onOpenInviteCommissions}>
            <span>{t('邀请人数')}</span>
            <strong>{userState?.user?.aff_count || 0}</strong>
          </button>
        </div>
        <div className='wallet-reward-actions'>
          <Button
            theme='solid'
            disabled={!hasAvailableReward}
            onClick={onOpenTransfer}
          >
            {t('划转到余额')}
          </Button>
          <Button
            theme='outline'
            disabled={!hasAvailableReward}
            onClick={onOpenWithdrawal}
          >
            {t('提现')}
          </Button>
        </div>
      </section>
      <div className='wallet-invitation-body'>
        <label className='wallet-field-label'>{t('邀请链接')}</label>
        <Input
          value={affLink}
          readonly
          aria-label={t('邀请链接')}
          suffix={
            <Button
              theme='borderless'
              onClick={handleAffLinkClick}
              icon={<Copy size={14} />}
            >
              {t('复制')}
            </Button>
          }
        />
        <Space wrap className='wallet-invitation-links'>
          <Button
            theme='borderless'
            size='small'
            onClick={() => setPromotionVisible(true)}
            disabled={!affLink}
          >
            {t('推广素材与点击统计')}
          </Button>
          <Button
            theme='borderless'
            size='small'
            onClick={openWithdrawalHistory}
          >
            {t('我的提现记录')}
          </Button>
        </Space>
        <details className='wallet-invite-code'>
          <summary>{t('修改专属邀请码')}</summary>
          <Input
            value={newCode}
            onChange={setNewCode}
            aria-label={t('专属邀请码')}
            placeholder={
              affCode
                ? `${t('当前')}: ${affCode}`
                : t('4-32位字母数字，好记好分享')
            }
            suffix={
              <Button
                theme='solid'
                loading={savingCode}
                disabled={!newCode || newCode === affCode}
                onClick={async () => {
                  setSavingCode(true);
                  try {
                    if (await onSaveAffCode?.(newCode)) setNewCode('');
                  } finally {
                    setSavingCode(false);
                  }
                }}
              >
                {t('保存')}
              </Button>
            }
          />
        </details>
        <details className='wallet-reward-explanation'>
          <summary>{t('奖励说明')}</summary>
          <p>{t('邀请好友注册，好友充值后您可获得相应奖励')}</p>
          <p>{t('通过划转功能将奖励额度转入到您的账户余额中')}</p>
          <p>{t('也可以提交提现申请，管理员审核后通过支付宝转账')}</p>
        </details>
      </div>
      <InvitePromotionModal
        visible={promotionVisible}
        onCancel={() => setPromotionVisible(false)}
        affLink={affLink}
      />
    </Card>
  );
};
export default InvitationCard;
