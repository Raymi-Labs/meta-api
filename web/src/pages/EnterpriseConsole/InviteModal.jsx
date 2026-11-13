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
import { useTranslation } from 'react-i18next';
import {
  Button,
  Input,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
  Tag,
} from '@douyinfe/semi-ui';
import { enterpriseRequest } from '../../helpers/enterprise';
import { displayAmountToQuota } from '../../helpers/quota';
import { showError, showSuccess } from '../../helpers/toast';
import { copy } from '../../helpers/utils';

// 邀请链接只在创建时返回一次，所以成功后留在弹窗里展示并提供复制。
export default function InviteModal({
  enterpriseId,
  visible,
  isOwner,
  onClose,
  onDone,
}) {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState('member');
  const [amount, setAmount] = useState();
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);

  const close = () => {
    setEmail('');
    setRole('member');
    setAmount(undefined);
    setResult(null);
    onClose();
  };

  const submit = async () => {
    if (!email.trim()) {
      showError(t('请填写邮箱'));
      return;
    }
    setBusy(true);
    try {
      const data = await enterpriseRequest(
        'post',
        `/api/enterprises/${enterpriseId}/invitations`,
        {
          email: email.trim(),
          role,
          initial_quota: displayAmountToQuota(amount),
        },
      );
      setResult(data);
      onDone();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const copyLink = async () => {
    if (await copy(result.link)) showSuccess(t('已复制'));
    else showError(t('复制失败，请手动选择'));
  };

  return (
    <Modal
      className='ec-modal'
      title={t('邀请成员')}
      visible={visible}
      okText={t('发送邀请')}
      confirmLoading={busy}
      onOk={submit}
      onCancel={close}
      footer={
        result ? (
          <Button theme='solid' type='primary' onClick={close}>
            {t('完成')}
          </Button>
        ) : undefined
      }
    >
      {result ? (
        <div className='ec-invite-result'>
          <Tag color={result.mail_sent ? 'green' : 'orange'} size='large'>
            {result.mail_sent
              ? t('邮件已发送')
              : t('邮件未发出，请复制链接转交')}
          </Tag>
          <Input readonly value={result.link} />
          <Button onClick={copyLink}>{t('复制链接')}</Button>
        </div>
      ) : (
        <>
          <label className='ec-field'>
            {t('邮箱')}
            <Input value={email} onChange={setEmail} />
          </label>
          {isOwner && (
            <div className='ec-field'>
              {t('角色')}
              <RadioGroup
                value={role}
                onChange={(e) => setRole(e.target.value)}
              >
                <Radio value='member'>{t('成员')}</Radio>
                <Radio value='admin'>{t('管理员')}</Radio>
              </RadioGroup>
            </div>
          )}
          <label className='ec-field'>
            {t('初始额度')}
            <InputNumber
              min={0}
              precision={2}
              value={amount}
              onChange={setAmount}
              style={{ width: '100%' }}
            />
          </label>
        </>
      )}
    </Modal>
  );
}
