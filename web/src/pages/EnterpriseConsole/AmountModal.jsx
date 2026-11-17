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
  Button,
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
  const allocate = mode === 'allocate';
  const [rawUnits, setRawUnits] = useState(false);
  const [allQuota, setAllQuota] = useState(null);
  const base = `/api/enterprises/${enterpriseId}`;
  const operation = useEnterpriseOperation({
    scope: `${base}/${member?.id}/${mode}`,
    url: `${base}/members/${member?.id}/${mode}`,
    lookupBase: member ? base : null,
    onDone,
    onClose,
  });
  const { busy, locked } = operation;

  useEffect(() => {
    setAmount(undefined);
    setRawUnits(false);
    setAllQuota(null);
    setReason('');
  }, [member?.id, mode]);

  useEffect(() => {
    const data = operation.pending?.data;
    if (!data) return;
    setAmount(data.amount);
    setRawUnits(true);
    setAllQuota(data.expected_available ?? null);
    setReason(data.reason || '');
  }, [operation.pending]);

  if (!member) return null;
  const debt = member.available < 0 ? -member.available : 0;
  const limit = allocate
    ? Math.max(unallocated, 0)
    : Math.max(member.available, 0);
  const name = member.display_name || member.username;

  const submit = async () => {
    if (operation.pending) return operation.submit(operation.pending.data);
    if (busy) return;
    const quota = parseEnterpriseQuota(amount, {
      raw: rawUnits,
      quotaPerUnit: getQuotaPerUnit(),
      ...getCurrencyConfig(),
    });
    if (
      amount === undefined ||
      amount === '' ||
      !Number.isSafeInteger(quota) ||
      quota <= 0 ||
      quota > 1e15
    ) {
      showError(t('请填写金额'));
      return;
    }
    if (quota > limit) {
      showError(
        allocate
          ? t('未分配余额不足，最多 {{amount}}', {
              amount: renderQuota(limit, 8),
            })
          : t('回收金额超过成员可用额度，最多 {{amount}}', {
              amount: renderQuota(limit, 8),
            }),
      );
      return;
    }
    await operation.submit({
      amount: quota,
      reason: reason.trim(),
      ...(allQuota !== null ? { expected_available: allQuota } : {}),
    });
  };

  return (
    <Modal
      className='ec-modal ec-financial-modal'
      title={`${allocate ? t('分配额度') : t('回收额度')} · ${name}`}
      visible
      okText={
        operation.pending ? t('重试原操作') : allocate ? t('分配') : t('回收')
      }
      confirmLoading={busy}
      onOk={submit}
      onCancel={() => !busy && onClose()}
      maskClosable={!busy}
      closeOnEsc={!busy}
      closable={!busy}
      cancelButtonProps={{ disabled: busy }}
    >
      <EnterpriseOperationStatus operation={operation} />
      <RadioGroup
        type='button'
        aria-label={t('金额单位')}
        value={rawUnits}
        disabled={locked}
        onChange={(e) => {
          setRawUnits(e.target.value);
          setAmount(undefined);
          setAllQuota(null);
        }}
      >
        <Radio value={false}>{t('金额')}</Radio>
        <Radio value={true}>{t('原始额度')}</Radio>
      </RadioGroup>
      {!allocate && (
        <Button
          disabled={
            locked || !Number.isSafeInteger(limit) || limit <= 0 || limit > 1e15
          }
          onClick={() => {
            setRawUnits(true);
            setAmount(limit);
            setAllQuota(limit);
          }}
        >
          {t('全部回收')}
        </Button>
      )}
      <label className='ec-field'>
        <span className='ec-field-head'>
          {rawUnits ? t('原始额度') : t('金额')}
          <em>
            {allocate
              ? t('未分配余额 {{amount}}', {
                  amount: renderQuota(unallocated, 8),
                })
              : t('最多可回收 {{amount}}', { amount: renderQuota(limit, 8) })}
            {allocate && debt > 0
              ? ` · ${t('先抵欠额 {{amount}}', { amount: renderQuota(debt, 8) })}`
              : ''}
          </em>
        </span>
        <InputNumber
          min={0}
          precision={rawUnits ? 0 : 8}
          disabled={locked}
          aria-label={rawUnits ? t('原始额度') : t('金额')}
          value={amount}
          onChange={(v) => {
            setAmount(v);
            setAllQuota(null);
          }}
          style={{ width: '100%' }}
        />
      </label>
      <label className='ec-field'>
        {t('备注')}
        <Input
          aria-label={t('备注')}
          disabled={locked}
          maxLength={200}
          value={reason}
          onChange={setReason}
        />
      </label>
    </Modal>
  );
}
