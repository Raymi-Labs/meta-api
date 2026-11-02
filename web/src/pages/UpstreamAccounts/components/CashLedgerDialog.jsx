import React, { useRef, useState } from 'react';
import {
  Banner,
  Input,
  Modal,
  Select,
  Switch,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../helpers';

const { Text } = Typography;
const kinds = {
  opening: '期初现金基础',
  purchase: '购入 credits',
  refund: '退回 credits',
  expense: '直接费用',
  byok: 'BYOK 外部成本',
};

export { kinds as cashEntryKinds };

export default function CashLedgerDialog({
  initial,
  accountID,
  onClose,
  onSaved,
  t,
}) {
  const [draft, setDraft] = useState(initial);
  const [saving, setSaving] = useState(false);
  const busy = useRef(false);
  const set = (key, value) => setDraft((d) => ({ ...d, [key]: value }));
  const field = (key, label, props = {}) => (
    <label className='block space-y-1' key={key}>
      <Text>{t(label)}</Text>
      <Input
        aria-label={t(label)}
        value={draft[key] ?? ''}
        onChange={(v) => set(key, v)}
        {...props}
      />
    </label>
  );
  const save = async () => {
    if (busy.current) return;
    busy.current = true;
    setSaving(true);
    try {
      let url, payload;
      if (draft.type === 'enable') {
        url = '/api/cash_accounting/enable';
        payload = {};
      } else if (draft.type === 'binding') {
        url = `/api/upstream_accounts/${accountID}/cash/bindings`;
        payload = { key_hash: draft.key_hash, site_only: draft.site_only };
      } else if (draft.type === 'void') {
        url = `/api/upstream_accounts/${accountID}/cash/entries/${draft.id}/void`;
        payload = { reason: draft.reason };
      } else if (draft.type === 'receipt') {
        url = `/api/cash_accounting/funding/${draft.lot_id}/receipt`;
        const receivedAt = Date.parse(`${draft.received_time}:00Z`);
        if (!Number.isFinite(receivedAt)) throw new Error(t('请选择有效日期'));
        payload = {
          request_id: draft.request_id,
          previous_id: draft.previous_id,
          currency: draft.currency,
          gross_amount: draft.gross_amount,
          fee_amount: draft.fee_amount,
          fx_rate: draft.fx_rate,
          received_at: Math.floor(receivedAt / 1000),
          evidence: draft.evidence,
          note: draft.note,
        };
      } else {
        url = `/api/upstream_accounts/${accountID}/cash/entries`;
        payload = {
          request_id: draft.request_id,
          kind: draft.kind,
          date: draft.date,
          credits: draft.credits,
          currency: draft.currency,
          gross_amount: draft.gross_amount,
          fee_amount: draft.fee_amount,
          fx_rate: draft.fx_rate,
          reference: draft.reference,
          evidence: draft.evidence,
        };
      }
      const { data } = await API.post(url, payload, {
        timeout: 70000,
        skipErrorHandler: true,
      });
      if (!data.success) throw new Error(data.message);
      showSuccess(t('保存成功'));
      onSaved();
    } catch (e) {
      showError(t(e.message));
    } finally {
      busy.current = false;
      setSaving(false);
    }
  };
  const title = {
    enable: '启用现金收益',
    binding: '调用密钥归属',
    void: '作废记录',
    receipt: '核实到账凭证',
    entry: '录入上游账单',
  }[draft.type];
  return (
    <Modal
      visible
      title={t(title)}
      onCancel={() => !saving && onClose()}
      onOk={save}
      okText={t('保存')}
      cancelText={t('取消')}
      confirmLoading={saving}
      width={560}
      style={{ maxWidth: 'calc(100vw - 24px)' }}
      maskClosable={!saving}
    >
      <div className='space-y-4'>
        {draft.type === 'enable' ? (
          <Banner
            type='info'
            description={t(
              '从启用时开始记录资金来源。历史余额、套餐和未追溯的变动保持待核对；不改变用户扣费。',
            )}
          />
        ) : draft.type === 'binding' ? (
          <>
            <Text className='break-all'>{draft.key_hash}</Text>
            <Text className='block'>
              {draft.channels?.map((c) => `${c.name} (#${c.id})`).join(' · ')}
            </Text>
            <label className='flex items-center gap-3'>
              <Switch
                aria-label={t('仅用于本站')}
                checked={draft.site_only}
                onChange={(v) => set('site_only', v)}
              />
              {t('仅用于本站')}
            </label>
            <Text type='tertiary'>
              {t(
                '请核实该调用密钥没有站外使用。共享密钥的完整利润保持待核对。',
              )}
            </Text>
          </>
        ) : draft.type === 'void' ? (
          <>
            <Text>{draft.reference}</Text>
            {field('reason', '作废原因', { maxLength: 255 })}
          </>
        ) : (
          <>
            {draft.type === 'entry' ? (
              <>
                <label className='block space-y-1'>
                  <Text>{t('账单类型')}</Text>
                  <Select
                    aria-label={t('账单类型')}
                    value={draft.kind}
                    style={{ width: '100%' }}
                    optionList={Object.entries(kinds).map(([value, label]) => ({
                      value,
                      label: t(label),
                    }))}
                    onChange={(kind) =>
                      setDraft((d) => ({
                        ...d,
                        kind,
                        credits: ['expense', 'byok'].includes(kind)
                          ? '0'
                          : d.credits,
                      }))
                    }
                  />
                </label>
                {field('date', '账单日期 (UTC)', { type: 'date' })}
                {field('credits', 'OpenRouter credits (USD)', {
                  inputMode: 'decimal',
                  disabled: ['expense', 'byok'].includes(draft.kind),
                })}
                {draft.kind === 'opening' && (
                  <Text type='tertiary'>
                    {t(
                      '期初填写该 UTC 日开始时的剩余 credits 及对应现金基础，不是累计充值金额。',
                    )}
                  </Text>
                )}
                {field('reference', '账单编号', { maxLength: 128 })}
              </>
            ) : (
              <>
                <Text>
                  {t('资金批次')} #{draft.lot_id} · {t('用户 ID')}{' '}
                  {draft.user_id}
                </Text>
                <Banner
                  type='info'
                  description={t(
                    '填写整个资金批次对应的真实到账及费用；期初批次填写剩余额度的现金基础。零元赠送也需凭证。',
                  )}
                />
                {field('received_time', '到账时间 (UTC)', {
                  type: 'datetime-local',
                })}
              </>
            )}
            <div className='grid grid-cols-2 gap-3'>
              {field('currency', '原币币种', { maxLength: 3 })}
              {field('fx_rate', '实际汇率（人民币 / 原币）', {
                inputMode: 'decimal',
              })}
              {field('gross_amount', '原币金额（不含手续费）', {
                inputMode: 'decimal',
              })}
              {field('fee_amount', '原币手续费', { inputMode: 'decimal' })}
            </div>
            <Text type='tertiary'>
              {t(
                '到账与退款扣除手续费；购款和费用加上手续费。人民币汇率填 1。',
              )}
            </Text>
            {field('evidence', '凭证引用', { maxLength: 255 })}
            {draft.type === 'receipt' &&
              field('note', '核实或更正说明', { maxLength: 255 })}
          </>
        )}
      </div>
    </Modal>
  );
}
