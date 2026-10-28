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

import React, {
  useCallback,
  useEffect,
  useState,
  useContext,
  useRef,
} from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  getCurrencyConfig,
  renderQuota,
  renderQuotaWithAmount,
  handleCopyUrl,
  getQuotaPerUnit,
  setUserData,
  timestamp2string,
} from '../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../helpers/quota';
import {
  createPaymentQuote,
  formatPaymentMoney,
} from '../../helpers/paymentCurrency';
import {
  redirectToPayment,
  submitPaymentForm,
} from '../../helpers/paymentNavigation';
import {
  Button,
  Card,
  Form,
  Modal,
  Select,
  Space,
  Tag,
  Toast,
  Typography,
  Divider,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useDebouncedCallback } from 'use-debounce';
import { Sparkles, BellRing } from 'lucide-react';
import { IconGift } from '@douyinfe/semi-icons';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import {
  AFF_WITHDRAWAL_MINIMUM_PAYMENT_CNY,
  AFF_WITHDRAWAL_MINIMUM_QUOTA,
  meetsAffWithdrawalMinimum,
} from '../../constants';

import RechargeCard from './RechargeCard';
import WalletOverview from './WalletOverview';
import WalletRecentPayments from './WalletRecentPayments';
import './wallet.css';
import InvitationCard from './InvitationCard';
import TransferModal from './modals/TransferModal';
import WithdrawalModal from './modals/WithdrawalModal';
import PaymentConfirmModal from './modals/PaymentConfirmModal';
import TopupHistoryModal from './modals/TopupHistoryModal';
import InviteCommissionModal from './modals/InviteCommissionModal';
import WalletRedemptionModal from './modals/WalletRedemptionModal';
import SubscriptionIssuanceModal from '../subscriptions/SubscriptionIssuanceModal';

const { Text, Title } = Typography;

const EMPTY_PAYMENT_QUOTE = Object.freeze({
  amount: 0,
  currency: '',
  estimated: true,
});

const roundCurrencyAmountUp = (amount) => {
  const numericAmount = Number(amount || 0);
  const cents = Math.round(numericAmount * 100);
  return Math.ceil(cents - 0.01) / 100;
};

const roundCurrencyAmountDown = (amount) => {
  const numericAmount = Number(amount || 0);
  return Math.floor(numericAmount * 100 + 1e-8) / 100;
};

const PERIOD_LABELS = {
  hourly: '每小时',
  daily: '每日',
  weekly: '每周',
  monthly: '每月',
  custom: '自定义',
};

const renderPeriodLabel = (t, period) =>
  t(PERIOD_LABELS[period] || PERIOD_LABELS.custom);
const renderValidityLabel = (t, validitySeconds) =>
  Number(validitySeconds || 0) > 0
    ? `${Number(validitySeconds || 0)}s`
    : t('长期有效');

const getInviteOrigin = (serverAddress = '') => {
  const configuredOrigin = String(serverAddress || '')
    .trim()
    .replace(/\/+$/, '');
  if (configuredOrigin) return configuredOrigin;
  return window.location.origin.replace(/\/$/, '');
};

const TopUpContent = () => {
  const { t, i18n } = useTranslation();
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);

  const [redemptionCode, setRedemptionCode] = useState('');
  const [paymentQuote, setPaymentQuote] = useState(EMPTY_PAYMENT_QUOTE);
  const [minTopUp, setMinTopUp] = useState(statusState?.status?.min_topup || 1);
  const [topUpCount, setTopUpCount] = useState(
    statusState?.status?.min_topup || 1,
  );
  const [topUpLink, setTopUpLink] = useState(
    statusState?.status?.top_up_link || '',
  );
  const [enableOnlineTopUp, setEnableOnlineTopUp] = useState(
    statusState?.status?.enable_online_topup || false,
  );

  const [enableStripeTopUp, setEnableStripeTopUp] = useState(
    statusState?.status?.enable_stripe_topup || false,
  );
  const [statusLoading, setStatusLoading] = useState(true);
  const [topupInfoLoading, setTopupInfoLoading] = useState(true);
  const [topupInfoError, setTopupInfoError] = useState(false);

  // Creem 状态
  const [creemProducts, setCreemProducts] = useState([]);
  const [enableCreemTopUp, setEnableCreemTopUp] = useState(false);
  const [creemOpen, setCreemOpen] = useState(false);
  const [selectedCreemProduct, setSelectedCreemProduct] = useState(null);
  const [enableWaffoTopUp, setEnableWaffoTopUp] = useState(false);
  const [waffoMinTopUp, setWaffoMinTopUp] = useState(1);

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [open, setOpen] = useState(false);
  const [payWay, setPayWay] = useState('');
  const [amountLoading, setAmountLoading] = useState(false);
  const [paymentLoading, setPaymentLoading] = useState(false);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [payMethods, setPayMethods] = useState([]);

  const aliveRef = useRef(true);
  const quotaRequestRef = useRef(0);
  const [balanceLoading, setBalanceLoading] = useState(true);
  const [balanceError, setBalanceError] = useState(false);
  const [walletRevision, setWalletRevision] = useState(0);
  const paymentBusyRef = useRef(false);
  const paymentSessionRef = useRef(0);
  const redeemBusyRef = useRef(false);
  const [confirmedPayment, setConfirmedPayment] = useState(null);
  const currentPaymentRef = useRef(null);
  currentPaymentRef.current = { count: topUpCount, way: payWay };
  const affOriginRef = useRef('');
  const amountRequestRef = useRef(0);

  const resetPaymentQuote = useCallback(() => {
    amountRequestRef.current += 1;
    setPaymentQuote(EMPTY_PAYMENT_QUOTE);
    setAmountLoading(false);
  }, []);

  // 邀请状态
  const [affLink, setAffLink] = useState('');
  const [affCode, setAffCode] = useState('');
  const [openTransfer, setOpenTransfer] = useState(false);
  const [openWithdrawal, setOpenWithdrawal] = useState(false);
  const [openInviteCommissions, setOpenInviteCommissions] = useState(false);
  const [openWalletRedemption, setOpenWalletRedemption] = useState(false);
  const [withdrawalSubmitting, setWithdrawalSubmitting] = useState(false);
  const [alipayAccount, setAlipayAccount] = useState('');
  const [alipayName, setAlipayName] = useState('');
  const [transferAmount, setTransferAmount] = useState(() => {
    const minTransferAmount = quotaToDisplayAmount(getQuotaPerUnit());
    return getCurrencyConfig().type === 'TOKENS'
      ? minTransferAmount
      : roundCurrencyAmountUp(minTransferAmount);
  });
  const [withdrawalAmount, setWithdrawalAmount] = useState(() => {
    const minWithdrawalAmount = quotaToDisplayAmount(
      AFF_WITHDRAWAL_MINIMUM_QUOTA,
    );
    return getCurrencyConfig().type === 'TOKENS'
      ? minWithdrawalAmount
      : roundCurrencyAmountUp(minWithdrawalAmount);
  });
  const [historyInitialTab, setHistoryInitialTab] = useState('records');

  // 计费弹窗状态
  const [openHistory, setOpenHistory] = useState(false);
  const [redeemTargetModalOpen, setRedeemTargetModalOpen] = useState(false);
  const [redeemTargetOptions, setRedeemTargetOptions] = useState([]);
  const [redeemTargetPlanTitle, setRedeemTargetPlanTitle] = useState('');
  const [selectedRenewTargetId, setSelectedRenewTargetId] = useState(0);

  // 套餐兑换方式选择弹窗状态
  const [purchaseModeModalOpen, setPurchaseModeModalOpen] = useState(false);
  const [purchaseModePlanTitle, setPurchaseModePlanTitle] = useState('');
  const [selectedPurchaseMode, setSelectedPurchaseMode] = useState('stack');
  const [pendingSubscriptionIssuances, setPendingSubscriptionIssuances] =
    useState([]);
  const [subscriptionIssuanceId, setSubscriptionIssuanceId] = useState(0);
  const [subscriptionIssuanceVisible, setSubscriptionIssuanceVisible] =
    useState(false);

  // 订阅状态
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [subscriptionLoading, setSubscriptionLoading] = useState(true);
  const [billingPreference, setBillingPreference] =
    useState('subscription_first');
  const [activeSubscriptions, setActiveSubscriptions] = useState([]);
  const [activeQuantityByPlan, setActiveQuantityByPlan] = useState({});
  const [allSubscriptions, setAllSubscriptions] = useState([]);

  // 预设充值金额选项
  const [presetAmounts, setPresetAmounts] = useState([]);
  const [selectedPreset, setSelectedPreset] = useState(null);

  // 充值配置数据
  const [topupInfo, setTopupInfo] = useState({
    amount_options: [],
    discount: {},
  });

  const isPayMethodEnabled = (paymentMethod) => {
    if (!paymentMethod) return false;
    if (paymentMethod === 'stripe') return enableStripeTopUp;
    if (paymentMethod === 'waffo' || paymentMethod.startsWith('waffo:')) {
      return enableWaffoTopUp;
    }
    return enableOnlineTopUp;
  };

  const canUsePayMethodForAmount = (paymentMethod, amountValue) => {
    if (!paymentMethod || !isPayMethodEnabled(paymentMethod)) return false;
    const payMethod = payMethods.find((item) => item.type === paymentMethod);
    if (!payMethod) return false;
    const minTopupVal = Number(payMethod.min_topup) || 0;
    return Number(amountValue || 0) >= minTopupVal;
  };

  const pickAvailablePayMethod = (amountValue) =>
    payMethods.find((item) => canUsePayMethodForAmount(item.type, amountValue));

  const topUp = async (renewTargetSubscriptionId = 0, purchaseMode = '') => {
    if (!aliveRef.current || redeemBusyRef.current) return;
    if (redemptionCode === '') {
      showInfo(t('请输入兑换码！'));
      return;
    }
    redeemBusyRef.current = true;
    setIsSubmitting(true);
    try {
      const payload = {
        key: redemptionCode,
        renew_target_subscription_id: renewTargetSubscriptionId || 0,
      };
      if (purchaseMode) {
        payload.purchase_mode = purchaseMode;
      }
      const res = await API.post('/api/user/redeem', payload, {
        skipErrorHandler: true,
      });
      if (!aliveRef.current) return;
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('兑换成功！'));
        if (data?.benefit_type === 'subscription') {
          setSubscriptionIssuanceId(Number(data?.issuance_id || 0));
          setSubscriptionIssuanceVisible(true);
          Modal.success({
            title: t('套餐待发放已创建'),
            content:
              data?.action_summary ||
              t('已生成套餐待发放记录，请继续选择叠加或续费方式。'),
            centered: true,
          });
          await getSubscriptionSelf();
        } else {
          Modal.success({
            title: t('兑换成功！'),
            content: t('成功兑换额度：') + renderQuota(data?.quota_added || 0),
            centered: true,
          });
          // Refresh both quota fields from the server; a redeemed code may not be transferable.
          await getUserQuota();
          setWalletRevision((revision) => revision + 1);
        }
        setRedemptionCode('');
        setRedeemTargetModalOpen(false);
        setRedeemTargetOptions([]);
        setRedeemTargetPlanTitle('');
        setSelectedRenewTargetId(0);
        setPurchaseModeModalOpen(false);
        setPurchaseModePlanTitle('');
        setSelectedPurchaseMode('stack');
      } else {
        if (data?.code === 'redeem_select_purchase_mode') {
          setPurchaseModePlanTitle(data?.plan_title || '');
          setSelectedPurchaseMode('stack');
          setPurchaseModeModalOpen(true);
          return;
        }
        if (data?.code === 'redeem_select_renew_target') {
          const options = (data?.options || [])
            .map((item) => item?.subscription)
            .filter(Boolean);
          setRedeemTargetOptions(options);
          setRedeemTargetPlanTitle(data?.plan_title || '');
          setSelectedRenewTargetId(Number(options?.[0]?.id || 0));
          setRedeemTargetModalOpen(true);
          return;
        }
        showError(message);
      }
    } catch (err) {
      const status = err?.response?.status;
      const body = err?.response?.data;
      if (status === 429 || body?.error?.code === 'rate_limited') {
        const retry = Number(body?.error?.retry_after) || 60;
        const scope = body?.error?.scope;
        if (scope === 'RDF') {
          showError(t('兑换失败次数过多，{{sec}} 秒后再试', { sec: retry }));
        } else {
          showError(t('兑换次数过于频繁，{{sec}} 秒后再试', { sec: retry }));
        }
      } else {
        showError(t('请求失败'));
      }
    } finally {
      redeemBusyRef.current = false;
      if (aliveRef.current) setIsSubmitting(false);
    }
  };

  const openTopUpLink = () => {
    if (!topUpLink) {
      showError(t('超级管理员未设置充值链接！'));
      return;
    }
    window.open(topUpLink, '_blank');
  };

  const fetchQuotedAmount = async (
    value,
    paymentMethod = payWay,
    options = {},
  ) => {
    const { showErrorToast = false } = options;
    const normalizedValue = Number(value ?? topUpCount);
    if (!paymentMethod) {
      resetPaymentQuote();
      return null;
    }

    const requestId = amountRequestRef.current + 1;
    amountRequestRef.current = requestId;
    setAmountLoading(true);

    try {
      const endpoint =
        paymentMethod === 'stripe'
          ? '/api/user/stripe/amount'
          : paymentMethod === 'waffo' || paymentMethod.startsWith('waffo:')
            ? '/api/user/waffo/amount'
            : '/api/user/amount';
      const res = await API.post(endpoint, {
        amount: parseFloat(normalizedValue),
      });
      if (requestId !== amountRequestRef.current) return null;
      if (res?.data?.message === 'success') {
        const quote = createPaymentQuote(
          res.data.data,
          res.data.currency,
          paymentMethod,
          res.data.estimated,
        );
        if (quote) {
          setPaymentQuote(quote);
          return quote;
        }
      }
      setPaymentQuote(EMPTY_PAYMENT_QUOTE);
      if (showErrorToast) showError(res?.data?.data || t('获取金额失败'));
      return null;
    } catch (err) {
      if (requestId === amountRequestRef.current) {
        setPaymentQuote(EMPTY_PAYMENT_QUOTE);
      }
      return null;
    } finally {
      if (requestId === amountRequestRef.current) setAmountLoading(false);
    }
  };

  const debouncedGetAmount = useDebouncedCallback((value, options = {}) => {
    fetchQuotedAmount(value, options.paymentMethod || payWay, options);
  }, 400);

  const selectPayMethod = (payment) => {
    if (!payment || !canUsePayMethodForAmount(payment, topUpCount)) return;
    setPayWay(payment);
  };

  const openPaymentConfirm = async () => {
    if (!payWay) {
      showError(t('请选择支付方式'));
      return;
    }
    if (topUpCount < minTopUp) {
      showError(t('充值数量不能小于') + minTopUp);
      return;
    }
    if (!canUsePayMethodForAmount(payWay, topUpCount)) {
      const payMethod = payMethods.find((item) => item.type === payWay);
      const minTopupVal = Number(payMethod?.min_topup) || 0;
      if (minTopupVal > Number(topUpCount || 0)) {
        showError(t('此支付方式最低充值金额为') + ' ' + minTopupVal);
      } else {
        showError(t('请选择支付方式'));
      }
      return;
    }

    setPaymentLoading(true);
    try {
      debouncedGetAmount.cancel();
      const quote = await fetchQuotedAmount(topUpCount, payWay, {
        showErrorToast: true,
      });
      if (
        aliveRef.current &&
        quote?.amount > 0 &&
        currentPaymentRef.current.count === topUpCount &&
        currentPaymentRef.current.way === payWay
      ) {
        paymentSessionRef.current++;
        setConfirmedPayment({ count: topUpCount, way: payWay, quote });
        setOpen(true);
      }
    } catch (error) {
    } finally {
      setPaymentLoading(false);
    }
  };

  const creemPreTopUp = (product) => {
    if (!product?.productId) {
      showError(t('产品配置错误'));
      return;
    }
    paymentSessionRef.current++;
    setSelectedCreemProduct(product);
    setCreemOpen(true);
  };

  const onlineTopUp = async () => {
    if (
      !aliveRef.current ||
      paymentBusyRef.current ||
      !confirmedPayment ||
      !open ||
      confirmedPayment.count !== topUpCount ||
      confirmedPayment.way !== payWay ||
      !confirmedPayment.quote?.amount
    )
      return;
    paymentBusyRef.current = true;
    const paymentSession = paymentSessionRef.current;
    setConfirmLoading(true);
    try {
      const isWaffo = payWay === 'waffo' || payWay.startsWith('waffo:');
      const endpoint =
        payWay === 'stripe'
          ? '/api/user/stripe/pay'
          : isWaffo
            ? '/api/user/waffo/pay'
            : '/api/user/pay';
      const payload = { amount: parseInt(topUpCount), payment_method: payWay };
      if (payWay.startsWith('waffo:')) {
        const payMethodIndex = Number(payWay.split(':')[1]);
        if (Number.isFinite(payMethodIndex))
          payload.pay_method_index = payMethodIndex;
      }
      const res = await API.post(endpoint, payload);
      if (!aliveRef.current || paymentSession !== paymentSessionRef.current)
        return;
      if (res?.data?.message === 'success') {
        let redirected;
        if (payWay === 'stripe') {
          redirected = redirectToPayment(res.data.data?.pay_link);
        } else if (isWaffo) {
          redirected = redirectToPayment(res.data.data?.payment_url);
        } else {
          redirected = submitPaymentForm(res.data.url, res.data.data);
        }
        if (!redirected) showError(t('支付失败'));
      } else {
        showError(res?.data?.message || t('支付失败'));
      }
    } catch (err) {
      showError(err?.response?.data?.message || err?.message || t('支付失败'));
    } finally {
      if (paymentSession === paymentSessionRef.current) setOpen(false);
      paymentBusyRef.current = false;
      setConfirmLoading(false);
    }
  };

  const onlineCreemTopUp = async () => {
    if (
      !aliveRef.current ||
      paymentBusyRef.current ||
      !selectedCreemProduct?.productId
    )
      return;
    paymentBusyRef.current = true;
    const paymentSession = paymentSessionRef.current;
    setConfirmLoading(true);
    try {
      const res = await API.post('/api/user/creem/pay', {
        product_id: selectedCreemProduct.productId,
        payment_method: 'creem',
      });
      if (!aliveRef.current || paymentSession !== paymentSessionRef.current)
        return;
      if (res?.data?.message === 'success') {
        if (!redirectToPayment(res.data.data?.checkout_url)) {
          showError(t('支付失败'));
        }
      } else {
        showError(res?.data?.message || t('支付失败'));
      }
    } catch (err) {
      showError(err?.response?.data?.message || err?.message || t('支付失败'));
    } finally {
      paymentBusyRef.current = false;
      if (paymentSession === paymentSessionRef.current) setCreemOpen(false);
      setConfirmLoading(false);
    }
  };

  const getUserQuota = async () => {
    if (!aliveRef.current) return;
    const request = ++quotaRequestRef.current;
    setBalanceLoading(true);
    setBalanceError(false);
    try {
      const res = await API.get('/api/user/self', {
        disableDuplicate: true,
        skipErrorHandler: true,
      });
      if (!aliveRef.current || request !== quotaRequestRef.current) return;
      if (!res.data?.success || !res.data.data)
        throw new Error('Missing account');
      userDispatch({ type: 'login', payload: res.data.data });
      setUserData(res.data.data);
    } catch {
      if (aliveRef.current && request === quotaRequestRef.current)
        setBalanceError(true);
    } finally {
      if (aliveRef.current && request === quotaRequestRef.current)
        setBalanceLoading(false);
    }
  };
  const refreshWallet = () => {
    getUserQuota();
    getSubscriptionSelf();
    setWalletRevision((revision) => revision + 1);
  };

  const getSubscriptionPlans = async () => {
    setSubscriptionLoading(true);
    try {
      const res = await API.get('/api/subscription/plans');
      if (res.data?.success) setSubscriptionPlans(res.data.data || []);
    } catch {
      if (aliveRef.current) showError(t('请求失败，请重试'));
    } finally {
      setSubscriptionLoading(false);
    }
  };

  const getSubscriptionSelf = async () => {
    if (!aliveRef.current) return;
    try {
      const res = await API.get('/api/subscription/self');
      if (!aliveRef.current) return;
      if (res.data?.success) {
        const d = res.data.data;
        setBillingPreference(d?.billing_preference || 'subscription_first');
        setActiveSubscriptions(d?.subscriptions || []);
        setActiveQuantityByPlan(d?.active_quantity_by_plan || {});
        setAllSubscriptions(d?.all_subscriptions || []);
        setPendingSubscriptionIssuances(d?.pending_issuances || []);
      }
    } catch (e) {}
  };

  const updateBillingPreference = async (pref) => {
    const previousPref = billingPreference;
    setBillingPreference(pref);
    try {
      const res = await API.put('/api/subscription/self/preference', {
        billing_preference: pref,
      });
      if (!res.data?.success) setBillingPreference(previousPref);
    } catch (e) {
      setBillingPreference(previousPref);
    }
  };

  const getTopupInfo = async () => {
    setTopupInfoLoading(true);
    setTopupInfoError(false);
    try {
      const res = await API.get('/api/user/topup/info');
      if (!res.data?.success) {
        setTopupInfoError(true);
        return;
      }
      const data = res.data.data;
      setTopupInfo({
        amount_options: data.amount_options || [],
        discount: data.discount || {},
      });
      let pMethods = Array.isArray(data.pay_methods)
        ? data.pay_methods
        : JSON.parse(data.pay_methods || '[]');
      const enableWaffo = !!data.enable_waffo_topup;
      const waffoMethods = Array.isArray(data.waffo_pay_methods)
        ? data.waffo_pay_methods
        : [];
      if (enableWaffo && waffoMethods.length > 0) {
        pMethods = pMethods.filter((m) => m.type !== 'waffo');
        pMethods = [
          ...pMethods,
          ...waffoMethods.map((method, index) => ({
            ...method,
            name: method.name || `Waffo ${index + 1}`,
            type: `waffo:${index}`,
            min_topup: data.waffo_min_topup || 1,
            color: method.color || 'rgba(var(--semi-blue-5), 1)',
          })),
        ];
      }
      setPayMethods(pMethods.filter((m) => m.name && m.type));
      setEnableStripeTopUp(!!data.enable_stripe_topup);
      setEnableOnlineTopUp(!!data.enable_online_topup);
      setEnableCreemTopUp(!!data.enable_creem_topup);
      setEnableWaffoTopUp(enableWaffo);
      setWaffoMinTopUp(data.waffo_min_topup || 1);
      const mTopUp = data.enable_online_topup
        ? data.min_topup
        : data.enable_stripe_topup
          ? data.stripe_min_topup
          : enableWaffo
            ? data.waffo_min_topup
            : 1;
      setMinTopUp(mTopUp);
      setTopUpCount(mTopUp);
      setCreemProducts(JSON.parse(data.creem_products || '[]'));
      if (data.amount_options?.length > 0) {
        setPresetAmounts(
          data.amount_options.map((v) => ({
            value: v,
            discount: data.discount[v] || 1.0,
          })),
        );
      } else {
        setPresetAmounts(
          [1, 5, 10, 30, 50, 100, 300, 500].map((m) => ({
            value: mTopUp * m,
          })),
        );
      }
    } catch (e) {
      setTopupInfoError(true);
    } finally {
      setTopupInfoLoading(false);
    }
  };

  const getAffLink = async (
    serverAddress = statusState?.status?.server_address,
  ) => {
    const res = await API.get('/api/user/aff');
    if (res.data?.success) {
      const inviteOrigin = getInviteOrigin(serverAddress);
      setAffCode(res.data.data);
      setAffLink(`${inviteOrigin}/i/${res.data.data}`);
      affOriginRef.current = inviteOrigin;
    }
  };

  const saveAffCode = async (code) => {
    const res = await API.put('/api/user/aff', { aff_code: code });
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('专属邀请码已更新'));
      await getAffLink();
    } else {
      showError(message);
    }
    return success;
  };

  const handleAffLinkClick = async () => {
    if (!affLink) {
      showError(t('查询失败，请稍后重试'));
      return;
    }
    await handleCopyUrl(affLink, t);
  };

  const transfer = async () => {
    const transferQuota = displayAmountToQuota(transferAmount);
    if (transferQuota < getQuotaPerUnit()) return;
    const res = await API.post(`/api/user/aff_transfer`, {
      quota: transferQuota,
    });
    if (res.data?.success) {
      getUserQuota();
      setOpenTransfer(false);
    }
  };

  const submitWithdrawal = async () => {
    const withdrawalQuota = displayAmountToQuota(withdrawalAmount);
    const quotaPerUnit = Number(getQuotaPerUnit() || 0);
    const topUpPrice = Number(statusState?.status?.price || 0);
    if (!meetsAffWithdrawalMinimum(withdrawalQuota, quotaPerUnit, topUpPrice)) {
      showError(
        `${t('提现额度不能低于最低额度')}：${renderQuota(
          AFF_WITHDRAWAL_MINIMUM_QUOTA,
        )} / ¥${AFF_WITHDRAWAL_MINIMUM_PAYMENT_CNY.toFixed(2)}`,
      );
      return;
    }
    if (!alipayAccount.trim()) {
      showError(t('请输入支付宝账号'));
      return;
    }
    if (!alipayName.trim()) {
      showError(t('请输入支付宝实名姓名'));
      return;
    }

    setWithdrawalSubmitting(true);
    try {
      const res = await API.post('/api/user/aff-withdrawals', {
        quota: withdrawalQuota,
        alipay_account: alipayAccount.trim(),
        alipay_name: alipayName.trim(),
      });
      if (res.data?.success) {
        showSuccess(t('提现申请已提交'));
        setOpenWithdrawal(false);
        setAlipayAccount('');
        setAlipayName('');
        await getUserQuota();
      } else {
        showError(res.data?.message || t('提交提现失败'));
      }
    } catch (e) {
      const msg = e?.response?.data?.message || e?.message || t('提交提现失败');
      showError(msg);
    } finally {
      setWithdrawalSubmitting(false);
    }
  };

  const openHistoryTab = (tab) => {
    setHistoryInitialTab(tab);
    setOpenHistory(true);
  };

  const [searchParams, setSearchParams] = useSearchParams();
  useEffect(() => {
    const tab = searchParams.get('tab');
    if (['withdrawals', 'my-withdrawals', 'invoices'].includes(tab)) {
      openHistoryTab(tab);
      const next = new URLSearchParams(searchParams);
      next.delete('tab');
      setSearchParams(next, { replace: true });
    }
  }, [searchParams]);

  const tryOpenTransfer = () => {
    const minQuota = getQuotaPerUnit();
    const currentQuota = Number(userState?.user?.aff_quota || 0);
    if (currentQuota < minQuota) {
      showError(
        t('待使用收益不足最低划转额度 {{min}}', {
          min: renderQuota(minQuota),
        }),
      );
      return;
    }
    const isTokens = getCurrencyConfig().type === 'TOKENS';
    const minDisplay = isTokens
      ? minQuota
      : roundCurrencyAmountUp(quotaToDisplayAmount(minQuota));
    const maxDisplay = isTokens
      ? currentQuota
      : roundCurrencyAmountDown(quotaToDisplayAmount(currentQuota));
    const initial = Math.min(minDisplay, maxDisplay) || minDisplay;
    setTransferAmount(initial);
    setOpenTransfer(true);
  };

  const tryOpenWithdrawal = () => {
    const minQuota = AFF_WITHDRAWAL_MINIMUM_QUOTA;
    const currentQuota = Number(userState?.user?.aff_quota || 0);
    if (currentQuota < minQuota) {
      showError(
        t('待使用收益不足最低提现额度 {{min}}', {
          min: renderQuota(minQuota),
        }),
      );
      return;
    }
    const isTokens = getCurrencyConfig().type === 'TOKENS';
    const minDisplay = isTokens
      ? minQuota
      : roundCurrencyAmountUp(quotaToDisplayAmount(minQuota));
    const maxDisplay = isTokens
      ? currentQuota
      : roundCurrencyAmountDown(quotaToDisplayAmount(currentQuota));
    const initial = Math.min(minDisplay, maxDisplay) || minDisplay;
    setWithdrawalAmount(initial);
    setOpenWithdrawal(true);
  };

  const selectPresetAmount = (preset) => {
    setTopUpCount(preset.value);
    setSelectedPreset(preset.value);
  };

  const formatLargeNumber = (num) => num.toString();

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
      quotaRequestRef.current++;
      amountRequestRef.current++;
      debouncedGetAmount.cancel();
    };
  }, []);

  useEffect(() => {
    getUserQuota();
    getTopupInfo();
    getSubscriptionPlans();
    getSubscriptionSelf();
  }, []);

  useEffect(() => {
    const inviteOrigin = getInviteOrigin(statusState?.status?.server_address);
    if (affOriginRef.current === inviteOrigin) return;
    getAffLink(statusState?.status?.server_address);
  }, [statusState?.status?.server_address]);

  useEffect(() => {
    if (statusState?.status) {
      setTopUpLink(statusState.status.top_up_link || '');
      setStatusLoading(false);
    }
  }, [statusState?.status]);

  useEffect(() => {
    if (!payMethods.length) {
      if (payWay) setPayWay('');
      resetPaymentQuote();
      return;
    }

    if (payWay && canUsePayMethodForAmount(payWay, topUpCount)) return;

    const nextPayMethod = pickAvailablePayMethod(topUpCount);
    if (nextPayMethod) {
      if (nextPayMethod.type !== payWay) setPayWay(nextPayMethod.type);
      return;
    }

    if (payWay) setPayWay('');
    resetPaymentQuote();
  }, [
    payMethods,
    payWay,
    topUpCount,
    enableOnlineTopUp,
    enableStripeTopUp,
    enableWaffoTopUp,
    resetPaymentQuote,
  ]);

  useEffect(() => {
    debouncedGetAmount.cancel();
    paymentSessionRef.current++;
    setOpen(false);
    setConfirmedPayment(null);
    resetPaymentQuote();
    if (!payWay || !canUsePayMethodForAmount(payWay, topUpCount)) {
      return;
    }

    debouncedGetAmount(topUpCount, { paymentMethod: payWay });
    return () => debouncedGetAmount.cancel();
  }, [payWay, topUpCount, resetPaymentQuote]);

  const renderAmount = (value = paymentQuote.amount) =>
    Number.isFinite(Number(value)) && Number(value) > 0
      ? formatPaymentMoney(
          value,
          paymentQuote.currency,
          i18n.resolvedLanguage || i18n.language || 'zh-CN',
        )
      : '—';

  const updateWalletQuota = (remainingQuota, remainingTransferableQuota) => {
    if (!aliveRef.current || !userState.user) return;
    quotaRequestRef.current++;
    setBalanceError(false);
    setBalanceLoading(false);
    setWalletRevision((revision) => revision + 1);
    const updatedUser = {
      ...userState.user,
      quota: Number(remainingQuota || 0),
      transferable_quota: Number(remainingTransferableQuota || 0),
    };
    userDispatch({ type: 'login', payload: updatedUser });
    setUserData(updatedUser);
  };

  return (
    <div className='wallet-page' data-onboarding='wallet-management-panel'>
      <TransferModal
        t={t}
        openTransfer={openTransfer}
        transfer={transfer}
        handleTransferCancel={() => setOpenTransfer(false)}
        userState={userState}
        renderQuota={renderQuota}
        getQuotaPerUnit={getQuotaPerUnit}
        transferAmount={transferAmount}
        setTransferAmount={setTransferAmount}
      />
      <WithdrawalModal
        t={t}
        visible={openWithdrawal}
        onOk={submitWithdrawal}
        onCancel={() => setOpenWithdrawal(false)}
        confirmLoading={withdrawalSubmitting}
        userState={userState}
        renderQuota={renderQuota}
        getQuotaPerUnit={getQuotaPerUnit}
        topUpPrice={Number(statusState?.status?.price || 0)}
        withdrawalAmount={withdrawalAmount}
        setWithdrawalAmount={setWithdrawalAmount}
        alipayAccount={alipayAccount}
        setAlipayAccount={setAlipayAccount}
        alipayName={alipayName}
        setAlipayName={setAlipayName}
      />
      <InviteCommissionModal
        visible={openInviteCommissions}
        onCancel={() => setOpenInviteCommissions(false)}
        t={t}
      />
      <WalletRedemptionModal
        visible={openWalletRedemption}
        onCancel={() => setOpenWalletRedemption(false)}
        walletQuota={userState?.user?.quota || 0}
        transferableQuota={userState?.user?.transferable_quota || 0}
        isAdmin={Number(userState?.user?.role || 0) >= 10}
        onQuotaChanged={updateWalletQuota}
        t={t}
      />
      <PaymentConfirmModal
        t={t}
        open={open}
        onlineTopUp={onlineTopUp}
        handleCancel={() => {
          paymentSessionRef.current++;
          setOpen(false);
        }}
        confirmLoading={confirmLoading}
        topUpCount={confirmedPayment?.count ?? topUpCount}
        renderQuotaWithAmount={renderQuotaWithAmount}
        amountLoading={amountLoading}
        renderAmount={(value = confirmedPayment?.quote?.amount) =>
          formatPaymentMoney(
            value,
            confirmedPayment?.quote?.currency,
            i18n.resolvedLanguage || i18n.language,
          )
        }
        payWay={confirmedPayment?.way || payWay}
        payMethods={payMethods}
        amountNumber={confirmedPayment?.quote?.amount}
        discountRate={topupInfo?.discount?.[confirmedPayment?.count] || 1.0}
      />
      <TopupHistoryModal
        visible={openHistory}
        onCancel={() => setOpenHistory(false)}
        t={t}
        initialTab={historyInitialTab}
      />
      <SubscriptionIssuanceModal
        visible={subscriptionIssuanceVisible}
        issuanceId={subscriptionIssuanceId}
        onCancel={() => {
          setSubscriptionIssuanceVisible(false);
          setSubscriptionIssuanceId(0);
        }}
        onSuccess={() => {
          setSubscriptionIssuanceVisible(false);
          setSubscriptionIssuanceId(0);
          getSubscriptionSelf();
        }}
      />

      <WalletOverview
        user={userState?.user}
        loading={balanceLoading}
        error={balanceError}
        onRefresh={refreshWallet}
        onCreate={() => setOpenWalletRedemption(true)}
        onHistory={() => openHistoryTab('records')}
        onInvoices={() => openHistoryTab('invoices')}
        t={t}
      />
      <div className='wallet-content-grid'>
        <div>
          <RechargeCard
            t={t}
            enableOnlineTopUp={enableOnlineTopUp}
            enableStripeTopUp={enableStripeTopUp}
            enableCreemTopUp={enableCreemTopUp}
            enableWaffoTopUp={enableWaffoTopUp}
            waffoMinTopUp={waffoMinTopUp}
            creemProducts={creemProducts}
            creemPreTopUp={creemPreTopUp}
            presetAmounts={presetAmounts}
            selectedPreset={selectedPreset}
            selectPresetAmount={selectPresetAmount}
            formatLargeNumber={formatLargeNumber}
            topUpCount={topUpCount}
            minTopUp={minTopUp}
            renderQuotaWithAmount={renderQuotaWithAmount}
            setTopUpCount={setTopUpCount}
            setSelectedPreset={setSelectedPreset}
            renderAmount={renderAmount}
            amountLoading={amountLoading}
            payMethods={payMethods}
            selectPayMethod={selectPayMethod}
            openPaymentConfirm={openPaymentConfirm}
            paymentLoading={paymentLoading}
            payWay={payWay}
            userState={userState}
            renderQuota={renderQuota}
            statusLoading={statusLoading || topupInfoLoading}
            topupInfo={topupInfo}
            topupInfoError={topupInfoError}
            onRetryTopupInfo={getTopupInfo}
            onOpenHistory={() => openHistoryTab('records')}
            onOpenInvoices={() => openHistoryTab('invoices')}
            subscriptionLoading={subscriptionLoading}
            subscriptionPlans={subscriptionPlans}
            billingPreference={billingPreference}
            onChangeBillingPreference={updateBillingPreference}
            activeSubscriptions={activeSubscriptions}
            activeQuantityByPlan={activeQuantityByPlan}
            allSubscriptions={allSubscriptions}
            reloadSubscriptionSelf={getSubscriptionSelf}
          />
        </div>
        <aside className='wallet-secondary'>
          {/* 待发放动作卡片 - 移至侧边栏顶部 */}
          {pendingSubscriptionIssuances.length > 0 && (
            <Card
              className='!rounded-xl border border-orange-100 bg-orange-50/10'
              bodyStyle={{ padding: '12px' }}
            >
              <div className='flex items-center gap-2 mb-3'>
                <BellRing size={16} className='text-orange-500' />
                <Text strong size='small'>
                  {t('待完成动作')}
                </Text>
              </div>
              <div className='space-y-2'>
                {pendingSubscriptionIssuances.length > 0 && (
                  <div className='flex items-center justify-between gap-3 p-2 rounded-lg bg-white border border-purple-100'>
                    <div className='flex items-center gap-2 overflow-hidden'>
                      <div className='flex h-6 w-6 items-center justify-center rounded-full bg-purple-50 text-purple-600 flex-shrink-0'>
                        <Sparkles size={12} />
                      </div>
                      <Text size='small' className='truncate'>
                        {t('套餐待发放')} ({pendingSubscriptionIssuances.length}
                        )
                      </Text>
                    </div>
                    <Button
                      theme='solid'
                      type='primary'
                      size='extra-small'
                      className='!rounded-full'
                      onClick={() => {
                        const nextId = Number(
                          pendingSubscriptionIssuances?.[0]?.id || 0,
                        );
                        setSubscriptionIssuanceId(nextId);
                        setSubscriptionIssuanceVisible(true);
                      }}
                    >
                      {t('继续')}
                    </Button>
                  </div>
                )}
              </div>
            </Card>
          )}

          <Card
            className='!rounded-xl shadow-sm border border-slate-200'
            title={
              <Text type='tertiary' strong size='small'>
                {t('兑换码充值')}
              </Text>
            }
            bodyStyle={{ padding: '12px' }}
          >
            <Form initValues={{ redemptionCode }}>
              <Form.Input
                field='redemptionCode'
                noLabel
                placeholder={t('请输入兑换码')}
                value={redemptionCode}
                onChange={setRedemptionCode}
                prefix={<IconGift />}
                suffix={
                  <Button
                    type='primary'
                    theme='solid'
                    size='small'
                    onClick={() => topUp()}
                    loading={isSubmitting}
                  >
                    {t('立即兑换')}
                  </Button>
                }
                showClear
                style={{ width: '100%' }}
                extraText={
                  topUpLink && (
                    <Text type='tertiary' size='extra-small'>
                      {t('在找兑换码？')}
                      <Text
                        type='secondary'
                        underline
                        className='cursor-pointer'
                        onClick={openTopUpLink}
                      >
                        {t('购买兑换码')}
                      </Text>
                    </Text>
                  )
                }
              />
            </Form>
          </Card>
          <InvitationCard
            t={t}
            userState={userState}
            renderQuota={renderQuota}
            onOpenTransfer={tryOpenTransfer}
            onOpenWithdrawal={tryOpenWithdrawal}
            openWithdrawalHistory={() => openHistoryTab('my-withdrawals')}
            affLink={affLink}
            affCode={affCode}
            onSaveAffCode={saveAffCode}
            handleAffLinkClick={handleAffLinkClick}
            onOpenInviteCommissions={() => setOpenInviteCommissions(true)}
          />
        </aside>
      </div>
      <WalletRecentPayments
        revision={walletRevision}
        onHistory={() => openHistoryTab('records')}
        onInvoices={() => openHistoryTab('invoices')}
        t={t}
      />

      <Modal
        title={t('选择续费目标')}
        visible={redeemTargetModalOpen}
        onOk={() =>
          topUp(selectedRenewTargetId, selectedPurchaseMode || 'renew')
        }
        onCancel={() => {
          setRedeemTargetModalOpen(false);
          setSelectedRenewTargetId(0);
        }}
        size='small'
        centered
        confirmLoading={isSubmitting}
      >
        <p>
          {t('当前套餐存在多条可续费订阅，请选择要续费的目标')}：
          {redeemTargetPlanTitle || '-'}
        </p>
        <Select
          value={selectedRenewTargetId}
          onChange={(v) => setSelectedRenewTargetId(Number(v || 0))}
          style={{ width: '100%' }}
          optionList={redeemTargetOptions.map((sub) => ({
            label: `${t('订阅')} #${sub.id} · ${t('到期时间')} ${timestamp2string(sub.end_time)}`,
            value: sub.id,
          }))}
        />
      </Modal>
      <Modal
        title={t('选择兑换方式')}
        visible={purchaseModeModalOpen}
        onOk={() => {
          setPurchaseModeModalOpen(false);
          topUp(0, selectedPurchaseMode);
        }}
        onCancel={() => setPurchaseModeModalOpen(false)}
        size='small'
        centered
        confirmLoading={isSubmitting}
      >
        <p>
          {t('您正在兑换套餐')}：{purchaseModePlanTitle || '-'}
        </p>
        <p>{t('请选择兑换方式')}：</p>
        <Select
          value={selectedPurchaseMode}
          onChange={setSelectedPurchaseMode}
          style={{ width: '100%' }}
          optionList={[
            { label: t('叠加（新增一条订阅）'), value: 'stack' },
            { label: t('续费（延长现有订阅）'), value: 'renew' },
          ]}
        />
      </Modal>
      <Modal
        title={t('确定要充值 $')}
        visible={creemOpen}
        onOk={onlineCreemTopUp}
        onCancel={() => {
          paymentSessionRef.current++;
          setCreemOpen(false);
        }}
        maskClosable={false}
        size='small'
        centered
        confirmLoading={confirmLoading}
      >
        {selectedCreemProduct && (
          <div className='space-y-2'>
            <p>
              {t('产品名称')}：{selectedCreemProduct.name}
            </p>
            <p>
              {t('价格')}：{selectedCreemProduct.currency === 'EUR' ? '€' : '$'}
              {selectedCreemProduct.price}
            </p>
            <p>
              {t('充值额度')}：{selectedCreemProduct.quota}
            </p>
            <p>{t('是否确认充值？')}</p>
          </div>
        )}
      </Modal>
    </div>
  );
};

const TopUp = () => {
  const [userState] = useContext(UserContext);
  return (
    <TopUpContent key={`${userState?.user?.id}:${userState?.user?.role}`} />
  );
};
export default TopUp;
