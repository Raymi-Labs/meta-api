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
import {
  Avatar,
  Typography,
  Card,
  Button,
  Input,
  Badge,
  Space,
} from '@douyinfe/semi-ui';
import {
  Copy,
  Users,
  BarChart2,
  TrendingUp,
  Gift,
  Zap,
  Wallet,
} from 'lucide-react';

import InvitePromotionModal from './modals/InvitePromotionModal';

const { Text } = Typography;

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
  const hasAvailableReward =
    !!userState?.user?.aff_quota && userState?.user?.aff_quota > 0;

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      {/* 卡片头部 */}
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='green' className='mr-3 shadow-md'>
          <Gift size={16} />
        </Avatar>
        <div>
          <Typography.Text className='text-lg font-medium'>
            {t('邀请奖励')}
          </Typography.Text>
          <div className='text-xs'>{t('邀请好友获得额外奖励')}</div>
        </div>
      </div>

      {/* 收益展示区域 */}
      <Space vertical style={{ width: '100%' }}>
        {/* 统计数据统一卡片 */}
        <Card
          className='!rounded-xl w-full'
          cover={
            <div
              className='relative h-30'
              style={{
                '--palette-primary-darkerChannel': '0 75 80',
                backgroundImage: `linear-gradient(0deg, rgba(var(--palette-primary-darkerChannel) / 80%), rgba(var(--palette-primary-darkerChannel) / 80%)), url('/cover-4.webp')`,
                backgroundSize: 'cover',
                backgroundPosition: 'center',
                backgroundRepeat: 'no-repeat',
              }}
            >
              {/* 标题和按钮 */}
              <div className='relative z-10 h-full flex flex-col justify-between p-4'>
                <div className='flex justify-between items-center'>
                  <Text strong style={{ color: 'white', fontSize: '16px' }}>
                    {t('收益统计')}
                  </Text>
                  <Space spacing={8}>
                    <Button
                      type='primary'
                      theme='solid'
                      size='small'
                      disabled={!hasAvailableReward}
                      onClick={() => onOpenTransfer && onOpenTransfer()}
                      className='!rounded-lg'
                    >
                      <Zap size={12} className='mr-1' />
                      {t('划转到余额')}
                    </Button>
                    <Button
                      type='warning'
                      theme='solid'
                      size='small'
                      disabled={!hasAvailableReward}
                      onClick={() => onOpenWithdrawal && onOpenWithdrawal()}
                      className='!rounded-lg'
                    >
                      <Wallet size={12} className='mr-1' />
                      {t('提现')}
                    </Button>
                  </Space>
                </div>

                {/* 统计数据 */}
                <div className='grid grid-cols-3 gap-6 mt-4'>
                  {/* 待使用收益 */}
                  <div className='text-center'>
                    <div
                      className='text-base sm:text-2xl font-bold mb-2'
                      style={{ color: 'white' }}
                    >
                      {renderQuota(userState?.user?.aff_quota || 0)}
                    </div>
                    <div className='flex items-center justify-center text-sm'>
                      <TrendingUp
                        size={14}
                        className='mr-1'
                        style={{ color: 'rgba(255,255,255,0.8)' }}
                      />
                      <Text
                        style={{
                          color: 'rgba(255,255,255,0.8)',
                          fontSize: '12px',
                        }}
                      >
                        {t('待使用收益')}
                      </Text>
                    </div>
                  </div>

                  {/* 总收益 */}
                  <div className='text-center'>
                    <div
                      className='text-base sm:text-2xl font-bold mb-2'
                      style={{ color: 'white' }}
                    >
                      {renderQuota(userState?.user?.aff_history_quota || 0)}
                    </div>
                    <div className='flex items-center justify-center text-sm'>
                      <BarChart2
                        size={14}
                        className='mr-1'
                        style={{ color: 'rgba(255,255,255,0.8)' }}
                      />
                      <Text
                        style={{
                          color: 'rgba(255,255,255,0.8)',
                          fontSize: '12px',
                        }}
                      >
                        {t('总收益')}
                      </Text>
                    </div>
                  </div>

                  {/* 邀请人数 */}
                  <div
                    className='text-center cursor-pointer rounded-xl transition hover:bg-white/10 focus:outline-none focus:ring-2 focus:ring-white/60'
                    role='button'
                    tabIndex={0}
                    title={t('查看邀请返佣明细')}
                    onClick={onOpenInviteCommissions}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        onOpenInviteCommissions?.();
                      }
                    }}
                  >
                    <div
                      className='text-base sm:text-2xl font-bold mb-2'
                      style={{ color: 'white' }}
                    >
                      {userState?.user?.aff_count || 0}
                    </div>
                    <div className='flex items-center justify-center text-sm'>
                      <Users
                        size={14}
                        className='mr-1'
                        style={{ color: 'rgba(255,255,255,0.8)' }}
                      />
                      <Text
                        style={{
                          color: 'rgba(255,255,255,0.8)',
                          fontSize: '12px',
                        }}
                      >
                        {t('邀请人数')}
                      </Text>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          }
        >
          {/* 邀请链接部分 */}
          <Input
            value={affLink}
            readonly
            className='!rounded-lg'
            prefix={t('邀请链接')}
            suffix={
              <Button
                type='primary'
                theme='solid'
                onClick={handleAffLinkClick}
                icon={<Copy size={14} />}
                className='!rounded-lg'
              >
                {t('复制')}
              </Button>
            }
          />
          <Button
            className='mt-3 w-full'
            onClick={() => setPromotionVisible(true)}
            disabled={!affLink}
          >
            {t('推广素材与点击统计')}
          </Button>
          {/* 设置专属短邀请码：链接变 域名/i/<code>，好记好分享 */}
          <Input
            value={newCode}
            onChange={(v) => setNewCode(v)}
            className='!rounded-lg mt-2'
            prefix={t('专属邀请码')}
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
                  const ok = await onSaveAffCode?.(newCode);
                  setSavingCode(false);
                  if (ok) setNewCode('');
                }}
                className='!rounded-lg'
              >
                {t('保存')}
              </Button>
            }
          />
          <div className='mt-2 flex justify-end'>
            <Button
              size='small'
              theme='borderless'
              type='tertiary'
              onClick={openWithdrawalHistory}
            >
              {t('我的提现记录')}
            </Button>
          </div>
        </Card>

        {/* 奖励说明 */}
        <Card
          className='!rounded-xl w-full'
          title={<Text type='tertiary'>{t('奖励说明')}</Text>}
        >
          <div className='space-y-3'>
            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('邀请好友注册，好友充值后您可获得相应奖励')}
              </Text>
            </div>

            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('通过划转功能将奖励额度转入到您的账户余额中')}
              </Text>
            </div>

            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('也可以提交提现申请，管理员审核后通过支付宝转账')}
              </Text>
            </div>

            <div className='flex items-start gap-2'>
              <Badge dot type='success' />
              <Text type='tertiary' className='text-sm'>
                {t('邀请的好友越多，获得的奖励越多')}
              </Text>
            </div>
          </div>
        </Card>
      </Space>
      <InvitePromotionModal
        visible={promotionVisible}
        onCancel={() => setPromotionVisible(false)}
        affLink={affLink}
      />
    </Card>
  );
};

export default InvitationCard;
