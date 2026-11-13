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
import { useTranslation } from 'react-i18next';
import { Input, InputNumber, Modal } from '@douyinfe/semi-ui';
import { enterpriseRequest } from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { displayAmountToQuota } from '../../helpers/quota';
import { showError, showSuccess } from '../../helpers/toast';

// 分配（未分配余额 → 成员）与回收（成员 → 未分配余额）共用一个弹窗。
export default function AmountModal({
  enterpriseId,
  mode,
  member,
  unallocated,
  onClose,
  onDone,
}) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState();
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const allocate = mode === 'allocate';

  useEffect(() => {
    setAmount(undefined);
    setReason('');
  }, [member?.id, mode]);

  if (!member) return null;
  const debt = member.available < 0 ? -member.available : 0;
  const limit = allocate
    ? Math.max(unallocated, 0)
    : Math.max(member.available, 0);
  const name = member.display_name || member.username;

  const submit = async () => {
    const quota = displayAmountToQuota(amount);
    if (quota <= 0) {
      showError(t('请填写金额'));
      return;
    }
    if (quota > limit) {
      showError(
        allocate
          ? t('未分配余额不足，最多 {{amount}}', { amount: renderQuota(limit) })
          : t('回收金额超过成员可用额度，最多 {{amount}}', {
              amount: renderQuota(limit),
            }),
      );
      return;
    }
    setBusy(true);
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprises/${enterpriseId}/members/${member.id}/${mode}`,
        { amount: quota, reason: reason.trim() },
      );
      showSuccess(t('已完成'));
      onDone();
      onClose();
    } catch (e) {
      showError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      className='ec-modal'
      title={`${allocate ? t('分配额度') : t('回收额度')} · ${name}`}
      visible
      okText={allocate ? t('分配') : t('回收')}
      confirmLoading={busy}
      onOk={submit}
      onCancel={onClose}
    >
      <label className='ec-field'>
        <span className='ec-field-head'>
          {t('金额')}
          <em>
            {allocate
              ? t('未分配余额 {{amount}}', { amount: renderQuota(unallocated) })
              : t('最多可回收 {{amount}}', { amount: renderQuota(limit) })}
            {allocate && debt > 0
              ? ` · ${t('先抵欠额 {{amount}}', { amount: renderQuota(debt) })}`
              : ''}
          </em>
        </span>
        <InputNumber
          min={0}
          precision={2}
          value={amount}
          onChange={setAmount}
          style={{ width: '100%' }}
        />
      </label>
      <label className='ec-field'>
        {t('备注')}
        <Input maxLength={200} value={reason} onChange={setReason} />
      </label>
    </Modal>
  );
}
