import React, { useEffect, useState } from 'react';
import { Button, InputNumber, Modal, Space, Spin, Typography } from '@douyinfe/semi-ui';
import { API, renderQuota, showError, showSuccess } from '../../helpers';
import { displayAmountToQuota, getQuotaPerUnit, quotaToDisplayAmount } from '../../helpers/quota';
import { getCurrencyConfig } from '../../helpers/render';

export default function WalletAutoTopup({ t, enabled }) {
  const [open, setOpen] = useState(false);
  const [snapshot, setSnapshot] = useState(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [threshold, setThreshold] = useState(null);
  const [quantity, setQuantity] = useState(null);
  useEffect(() => {
    let active = true;
    setLoading(true);
    API.get('/api/user/stripe/auto-topup', { params: { quote: open } })
      .then(({ data }) => {
        if (!active) return;
        if (!data.success) throw new Error(data.message);
        setSnapshot(data.data);
        const { rule, quote } = data.data;
        setThreshold(rule.threshold > 0 ? quotaToDisplayAmount(rule.threshold) : null);
        setQuantity(rule.quantity || quote?.minimum || 1);
      })
      .catch((err) => { if (active && open) { setSnapshot(null); showError(err.message); } })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [open, enabled]);

  const rule = snapshot?.rule;
  const quote = snapshot?.quote;
  const configured = rule && rule.state !== 'disabled';
  if (!open && !enabled && !configured && !snapshot?.processing) return null;

  async function authorize() {
    if (!quote || saving) return;
    setSaving(true);
    try {
      const { data } = await API.post('/api/user/stripe/auto-topup/setup', {
        threshold: displayAmountToQuota(threshold), quantity, price: quote.price,
        unit_minor: quote.unit_minor, currency: quote.currency, grant_unit: quote.grant_unit,
      });
      if (!data.success) throw new Error(data.message);
      window.location.assign(data.data.url);
    } catch (error) { showError(error.message); } finally { setSaving(false); }
  }
  async function disable() {
    setSaving(true);
    try {
      const { data } = await API.delete('/api/user/stripe/auto-topup');
      if (!data.success) throw new Error(data.message);
      setSnapshot((old) => ({ ...old, rule: { ...old.rule, state: 'disabled' } }));
      showSuccess(t('已停用'));
    } catch (error) { showError(error.message); } finally { setSaving(false); }
  }
  const states = {
    disabled: t('未启用'), setup: t('等待支付授权'), armed: t('已启用'),
    waiting: t('等待余额恢复至阈值'), failed: t('已暂停，请重新授权'),
  };
  const valid = quote && displayAmountToQuota(threshold) > 0 &&
    Number.isInteger(quantity) && quantity >= quote.minimum && quantity <= 10000;
  const unit = getCurrencyConfig();
  return (
    <>
      <Button theme='borderless' onClick={() => setOpen(true)}>{t('余额自动充值')}</Button>
      <Modal title={t('余额自动充值')} visible={open} footer={null} onCancel={() => setOpen(false)}>
        {loading ? <Spin /> : <Space vertical align='start' spacing={12} style={{ width: '100%' }}>
          <Typography.Text>{t('状态')} · {snapshot?.processing ? t('处理中') : states[rule?.state]}</Typography.Text>
          {snapshot?.quote_error && <Typography.Text type='warning'>{snapshot.quote_error}</Typography.Text>}
          {rule?.authorized_at > 0 && configured && <Typography.Text>
            {t('余额低于')} {renderQuota(rule.threshold)} · {t('支付金额')} {rule.currency} {snapshot.charge_money}
          </Typography.Text>}
          {quote && <>
            <label>{t('余额低于')} ({unit.type === 'TOKENS' ? 'Tokens' : unit.symbol})<br />
              <InputNumber aria-label={t('余额低于')} min={0} value={threshold} onChange={setThreshold} /></label>
            <label>{t('充值数量')}<br />
              <InputNumber aria-label={t('充值数量')} min={quote.minimum} max={10000} precision={0}
                value={quantity} onChange={setQuantity} /></label>
            {valid && <Typography.Text>{t('余额低于 {{threshold}} 时，授权 Stripe 每次扣款 {{charge}}，到账 {{credit}}。', {
              threshold: renderQuota(displayAmountToQuota(threshold)),
              charge: new Intl.NumberFormat(undefined, { style: 'currency', currency: quote.currency, currencyDisplay: 'code' }).format(quote.unit_money * quantity),
              credit: renderQuota(quote.grant_unit * quantity * getQuotaPerUnit()),
            })}</Typography.Text>}
            <Button theme='solid' disabled={!valid || snapshot?.processing} loading={saving} onClick={authorize}>
              {t('前往 Stripe 授权')}
            </Button>
          </>}
          {configured && <Button type='danger' disabled={saving} onClick={disable}>{t('停用')}</Button>}
          <Typography.Text size='small' type='tertiary'>{t('可随时停用。已经开始的充值仍会完成。')}</Typography.Text>
        </Space>}
      </Modal>
    </>
  );
}
