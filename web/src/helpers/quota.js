import { getCurrencyConfig } from './render';

export const getQuotaPerUnit = () => {
  const raw = parseFloat(localStorage.getItem('quota_per_unit') || '1');
  return Number.isFinite(raw) && raw > 0 ? raw : 1;
};

export const quotaToDisplayAmount = (quota) => {
  const q = Number(quota || 0);
  if (!Number.isFinite(q) || q <= 0) return 0;
  const { type, rate } = getCurrencyConfig();
  if (type === 'TOKENS') return q;
  const usd = q / getQuotaPerUnit();
  if (type === 'USD') return usd;
  return usd * (rate || 1);
};

export const displayAmountToQuota = (amount) => {
  const val = Number(amount || 0);
  if (!Number.isFinite(val) || val <= 0) return 0;
  const { type, rate } = getCurrencyConfig();
  if (type === 'TOKENS') return Math.round(val);
  const usd = type === 'USD' ? val : val / (rate || 1);
  return Math.round(usd * getQuotaPerUnit());
};

// 将 USD 金额转换为配额，与当前展示货币无关。
export const usdAmountToQuota = (usdAmount) => {
  const val = Number(usdAmount || 0);
  if (!Number.isFinite(val) || val <= 0) return 0;
  return Math.round(val * getQuotaPerUnit());
};

// 将配额转换为 USD 金额，与当前展示货币无关。
export const quotaToUSDAmount = (quota) => {
  const q = Number(quota || 0);
  if (!Number.isFinite(q) || q <= 0) return 0;
  return q / getQuotaPerUnit();
};

export const quotaToCurrencyAmount = (quota) => {
  const q = Number(quota || 0);
  if (!Number.isFinite(q) || q <= 0) return 0;
  const { type, rate } = getCurrencyConfig();
  const usd = q / getQuotaPerUnit();
  return type === 'CNY' || type === 'CUSTOM' ? usd * (rate || 1) : usd;
};

export const currencyAmountToQuota = (amount) => {
  const val = Number(amount || 0);
  if (!Number.isFinite(val) || val <= 0) return 0;
  const { type, rate } = getCurrencyConfig();
  const usd = type === 'CNY' || type === 'CUSTOM' ? val / (rate || 1) : val;
  return Math.round(usd * getQuotaPerUnit());
};

export const renderCurrencyQuota = (quota, digits = 2) => {
  const { symbol } = getCurrencyConfig();
  return `${symbol}${quotaToCurrencyAmount(quota).toFixed(digits)}`;
};

// Payment minimum is CNY, independent of the user's display currency.
export const paymentCentsToQuota = (cents, price) => {
 const money = Number(cents), unitPrice = Number(price);
 if (!Number.isFinite(money) || money < 0 || !Number.isFinite(unitPrice) || unitPrice <= 0) return null;
 return Math.max(1, Math.ceil(money / 100 / unitPrice * getQuotaPerUnit()));
};
