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

import React, { useEffect, useRef, useState } from 'react';
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
  const [rawUnits, setRawUnits] = useState(false);
  const submitting = useRef(false);
  const credit = mode === 'credit';

  useEffect(() => {
    setAmount(undefined);
    setRawUnits(false);
    setSign(1);
    setVoucher('');
    setReason('');
  }, [enterprise?.id, mode]);

  if (!enterprise) return null;
  const quota = rawUnits ? Number(amount || 0) : displayAmountToQuota(amount);
  const after = enterprise.unallocated + (credit ? quota : sign * quota);

  const submit = async () => {
    if (submitting.current) return;
    if (!Number.isSafeInteger(quota) || quota <= 0 || quota > 1e15)
      return showError(t('请填写金额'));
    if (credit && !voucher.trim()) return showError(t('请填写转账凭证号'));
    if (!credit && !reason.trim()) return showError(t('请填写原因'));
    submitting.current = true;
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
      submitting.current = false;
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
      onCancel={() => !busy && onClose()}
      maskClosable={!busy}
      closeOnEsc={!busy}
      closable={!busy}
      cancelButtonProps={{ disabled: busy }}
    >
      {!credit && (
        <div className='ec-field'>
          <RadioGroup
            type='button'
            aria-label={t('方向')}
            value={sign}
            onChange={(e) => setSign(e.target.value)}
            disabled={busy}
          >
            <Radio value={1}>{t('增加')}</Radio>
            <Radio value={-1}>{t('扣减')}</Radio>
          </RadioGroup>
        </div>
      )}
      {!credit && (
        <RadioGroup
          type='button'
          aria-label={t('金额单位')}
          value={rawUnits}
          onChange={(e) => {
            setRawUnits(e.target.value);
            setAmount(undefined);
          }}
          disabled={busy}
        >
          <Radio value={false}>{t('金额')}</Radio>
          <Radio value={true}>{t('原始额度')}</Radio>
        </RadioGroup>
      )}
      <label className='ec-field'>
        <span className='ec-field-head'>
          {rawUnits ? t('原始额度') : t('金额')}
          <em className={after < 0 ? 'em-num neg' : undefined}>
            {quota > 0
              ? t('未分配余额 {{before}} → {{after}}', {
                  before: renderQuota(enterprise.unallocated, 8),
                  after: renderQuota(after, 8),
                })
              : t('未分配余额 {{amount}}', {
                  amount: renderQuota(enterprise.unallocated, 8),
                })}
          </em>
        </span>
        <InputNumber
          min={0}
          precision={rawUnits ? 0 : 8}
          aria-label={rawUnits ? t('原始额度') : t('金额')}
          disabled={busy}
          value={amount}
          onChange={setAmount}
          style={{ width: '100%' }}
        />
      </label>
      {credit && (
        <label className='ec-field'>
          {t('转账凭证号')}
          <Input
            maxLength={128}
            value={voucher}
            onChange={setVoucher}
            disabled={busy}
          />
        </label>
      )}
      <label className='ec-field'>
        {credit ? t('备注') : t('原因')}
        <Input
          maxLength={200}
          value={reason}
          onChange={setReason}
          disabled={busy}
        />
      </label>
    </Modal>
  );
}
