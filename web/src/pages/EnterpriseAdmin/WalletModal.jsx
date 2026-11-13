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
import {
  Input,
  InputNumber,
  Modal,
  Radio,
  RadioGroup,
} from '@douyinfe/semi-ui';
import { enterpriseRequest } from '../../helpers/enterprise';
import { renderQuota } from '../../helpers/render';
import { displayAmountToQuota } from '../../helpers/quota';
import { showError, showSuccess } from '../../helpers/toast';

// 入账（对公转账到账，凭证号防重复录入）与调账（纠错，必须写原因）共用一个弹窗。
export default function WalletModal({ mode, enterprise, onClose, onDone }) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState();
  const [sign, setSign] = useState(1);
  const [voucher, setVoucher] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const credit = mode === 'credit';

  useEffect(() => {
    setAmount(undefined);
    setSign(1);
    setVoucher('');
    setReason('');
  }, [enterprise?.id, mode]);

  if (!enterprise) return null;
  const quota = displayAmountToQuota(amount);
  const after = enterprise.unallocated + (credit ? quota : sign * quota);

  const submit = async () => {
    if (quota <= 0) return showError(t('请填写金额'));
    if (credit && !voucher.trim()) return showError(t('请填写转账凭证号'));
    if (!credit && !reason.trim()) return showError(t('请填写原因'));
    setBusy(true);
    try {
      await enterpriseRequest(
        'post',
        `/api/enterprise/admin/enterprises/${enterprise.id}/${mode}`,
        credit
          ? { amount: quota, voucher: voucher.trim(), reason: reason.trim() }
          : { delta: sign * quota, reason: reason.trim() },
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
      title={`${credit ? t('入账') : t('调账')} · ${enterprise.name}`}
      visible
      okText={credit ? t('入账') : t('调账')}
      okButtonProps={after < 0 ? { type: 'danger' } : undefined}
      confirmLoading={busy}
      onOk={submit}
      onCancel={onClose}
    >
      {!credit && (
        <div className='ec-field'>
          <RadioGroup
            type='button'
            aria-label={t('方向')}
            value={sign}
            onChange={(e) => setSign(e.target.value)}
          >
            <Radio value={1}>{t('增加')}</Radio>
            <Radio value={-1}>{t('扣减')}</Radio>
          </RadioGroup>
        </div>
      )}
      <label className='ec-field'>
        <span className='ec-field-head'>
          {t('金额')}
          <em className={after < 0 ? 'em-num neg' : undefined}>
            {quota > 0
              ? t('未分配余额 {{before}} → {{after}}', {
                  before: renderQuota(enterprise.unallocated),
                  after: renderQuota(after),
                })
              : t('未分配余额 {{amount}}', {
                  amount: renderQuota(enterprise.unallocated),
                })}
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
      {credit && (
        <label className='ec-field'>
          {t('转账凭证号')}
          <Input maxLength={128} value={voucher} onChange={setVoucher} />
        </label>
      )}
      <label className='ec-field'>
        {credit ? t('备注') : t('原因')}
        <Input maxLength={200} value={reason} onChange={setReason} />
      </label>
    </Modal>
  );
}
