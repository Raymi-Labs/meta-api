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
import { useEnterpriseOperation } from '../../hooks/enterprise/useEnterpriseOperation';
import EnterpriseOperationStatus from '../../components/common/EnterpriseOperationStatus';
import { renderQuota, getCurrencyConfig } from '../../helpers/render';
import { parseEnterpriseQuota } from '../../helpers/enterpriseQuota';
import { getQuotaPerUnit } from '../../helpers/quota';
import { showError } from '../../helpers/toast';

// 入账（对公转账到账，凭证号防重复录入）与调账（纠错，必须写原因）共用一个弹窗。
export default function WalletModal({ mode, enterprise, onClose, onDone }) {
  const { t } = useTranslation();
  const [amount, setAmount] = useState();
  const [sign, setSign] = useState(1);
  const [voucher, setVoucher] = useState('');
  const [reason, setReason] = useState('');
  const [rawUnits, setRawUnits] = useState(false);
  const credit = mode === 'credit';
  const base = `/api/enterprise/admin/enterprises/${enterprise?.id}`;
  const operation = useEnterpriseOperation({
    scope: `${base}/${mode}`,
    url: `${base}/${mode}`,
    lookupBase: enterprise ? base : null,
    onDone,
    onClose,
  });
  const { busy, locked } = operation;

  useEffect(() => {
    setAmount(undefined);
    setRawUnits(false);
    setSign(1);
    setVoucher('');
    setReason('');
  }, [enterprise?.id, mode]);

  useEffect(() => {
    const data = operation.pending?.data;
    if (!data) return;
    const originalAmount = data.delta ?? data.amount;
    setAmount(Math.abs(originalAmount));
    setSign(originalAmount < 0 ? -1 : 1);
    setRawUnits(true);
    setVoucher(data.voucher || '');
    setReason(data.reason || '');
  }, [operation.pending]);

  if (!enterprise) return null;
  const quota = parseEnterpriseQuota(amount, {
    raw: rawUnits,
    quotaPerUnit: getQuotaPerUnit(),
    ...getCurrencyConfig(),
  });
  const after = enterprise.unallocated + (credit ? quota : sign * quota);

  const submit = async () => {
    if (operation.pending) return operation.submit(operation.pending.data);
    if (busy) return;
    if (!Number.isSafeInteger(quota) || quota <= 0 || quota > 1e15)
      return showError(t('请填写金额'));
    if (credit && !voucher.trim()) return showError(t('请填写转账凭证号'));
    if (!credit && !reason.trim()) return showError(t('请填写原因'));
    await operation.submit(
      credit
        ? { amount: quota, voucher: voucher.trim(), reason: reason.trim() }
        : { delta: sign * quota, reason: reason.trim() },
    );
  };

  return (
    <Modal
      className='ec-modal ec-financial-modal'
      title={`${credit ? t('入账') : t('调账')} · ${enterprise.name}`}
      visible
      okText={
        operation.pending ? t('重试原操作') : credit ? t('入账') : t('调账')
      }
      okButtonProps={after < 0 ? { type: 'danger' } : undefined}
      confirmLoading={busy}
      onOk={submit}
      onCancel={() => !busy && onClose()}
      maskClosable={!busy}
      closeOnEsc={!busy}
      closable={!busy}
      cancelButtonProps={{ disabled: busy }}
    >
      <EnterpriseOperationStatus operation={operation} />
      {!credit && (
        <div className='ec-field'>
          <RadioGroup
            type='button'
            aria-label={t('方向')}
            value={sign}
            onChange={(e) => setSign(e.target.value)}
            disabled={locked}
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
          disabled={locked}
        >
          <Radio value={false}>{t('金额')}</Radio>
          <Radio value={true}>{t('原始额度')}</Radio>
        </RadioGroup>
      )}
      <label className='ec-field'>
        <span className='ec-field-head'>
          {rawUnits ? t('原始额度') : t('金额')}
          <em className={after < 0 ? 'em-num neg' : undefined}>
            {!operation.pending && quota > 0
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
          disabled={locked}
          value={amount}
          onChange={setAmount}
          style={{ width: '100%' }}
        />
      </label>
      {credit && (
        <label className='ec-field'>
          {t('转账凭证号')}
          <Input
            aria-label={t('转账凭证号')}
            maxLength={128}
            value={voucher}
            onChange={setVoucher}
            disabled={locked}
          />
        </label>
      )}
      <label className='ec-field'>
        {credit ? t('备注') : t('原因')}
        <Input
          maxLength={200}
          value={reason}
          onChange={setReason}
          disabled={locked}
        />
      </label>
    </Modal>
  );
}
