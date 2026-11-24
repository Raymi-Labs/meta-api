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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Badge,
  Button,
  Card,
  Divider,
  Select,
  Skeleton,
  Space,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, renderQuota } from '../../helpers';
import { getPaymentCurrencySymbol } from '../../helpers/render';
import { ChevronDown, ChevronUp, RefreshCw, Sparkles } from 'lucide-react';
import SubscriptionPurchaseModal from './modals/SubscriptionPurchaseModal';
import {
  getPlanPurchaseQuantityConfig,
  getRenewableSubscriptionsByPlan,
} from '../../helpers/subscriptionPurchase';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../helpers/subscriptionFormat';

const { Text } = Typography;

// 过滤 EPay 支付方式。
function getEpayMethods(payMethods = []) {
  return (payMethods || []).filter(
    (m) => m?.type && m.type !== 'stripe' && m.type !== 'creem',
  );
}

// 提交 EPay 表单。
function submitEpayForm({ url, params }) {
  const form = document.createElement('form');
  form.action = url;
  form.method = 'POST';
  const isSafari =
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1;
  if (!isSafari) form.target = '_blank';
  Object.keys(params || {}).forEach((key) => {
    // Sanitize key and value to prevent parameter injection
    if (typeof key !== 'string' || typeof params[key] === 'undefined') return;
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = key;
    input.value = String(params[key]);
    form.appendChild(input);
  });
  document.body.appendChild(form);
  form.submit();
  document.body.removeChild(form);
}

const SubscriptionPlansCard = ({
  t,
  loading = false,
  plans = [],
  payMethods = [],
  enableOnlineTopUp = false,
  enableStripeTopUp = false,
  enableCreemTopUp = false,
  billingPreference,
  onChangeBillingPreference,
  activeSubscriptions = [],
  activeQuantityByPlan = {},
  allSubscriptions = [],
  stripeRenewals = [],
  reloadSubscriptionSelf,
  withCard = true,
}) => {
  const [open, setOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [selectedPurchaseMode, setSelectedPurchaseMode] = useState('stack');
  const [selectedPurchaseQuantity, setSelectedPurchaseQuantity] = useState(1);
  // 在“续费当前套餐”模式下选中的目标订阅 ID。
  const [selectedRenewTargetSubscriptionId, setSelectedRenewTargetSubscriptionId] =
    useState(0);
  const [paying, setPaying] = useState(false);
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [subscriptionListCollapsed, setSubscriptionListCollapsed] =
    useState(true);

  const epayMethods = useMemo(() => getEpayMethods(payMethods), [payMethods]);

  // 按套餐统计可续费的生效订阅数量（仅统计生效订阅）。
  const planActiveCountMap = useMemo(() => {
    const map = new Map();
    (activeSubscriptions || []).forEach((sub) => {
      const planId = sub?.subscription?.plan_id;
      if (!planId) return;
      map.set(planId, (map.get(planId) || 0) + 1);
    });
    return map;
  }, [activeSubscriptions]);

  // 按套餐统计未过期份数，用于动态降低最大可购数量。
  const planActiveQuantityMap = useMemo(() => {
    const map = new Map();
    Object.entries(activeQuantityByPlan || {}).forEach(([planId, quantity]) => {
      const id = Number(planId);
      if (id <= 0) return;
      map.set(id, Math.max(0, Number(quantity || 0)));
    });
    return map;
  }, [activeQuantityByPlan]);

  const selectedPlanQuantityConfig = useMemo(() => {
    const planId = selectedPlan?.plan?.id;
    const activeQuantity = Number(planActiveQuantityMap.get(planId) || 0);
    return getPlanPurchaseQuantityConfig(selectedPlan?.plan, activeQuantity);
  }, [selectedPlan, planActiveQuantityMap]);
  const minPurchaseQuantity = selectedPlanQuantityConfig.min;
  const maxPurchaseQuantity = selectedPlanQuantityConfig.max;

  // 当前所选套餐下可续费订阅列表，按最早到期排序。
  const selectedPlanRenewableSubscriptions = useMemo(() => {
    const planId = Number(selectedPlan?.plan?.id || 0);
    return getRenewableSubscriptionsByPlan(activeSubscriptions, planId);
  }, [activeSubscriptions, selectedPlan]);
  const selectedPlanRenewable = selectedPlanRenewableSubscriptions.length > 0;

  useEffect(() => {
    setSelectedPurchaseQuantity((prev) => {
      const quantity = Number(prev || minPurchaseQuantity);
      if (maxPurchaseQuantity <= 0) return 0;
      if (quantity < minPurchaseQuantity) return minPurchaseQuantity;
      if (quantity > maxPurchaseQuantity) return maxPurchaseQuantity;
      return quantity;
    });
  }, [minPurchaseQuantity, maxPurchaseQuantity]);

  useEffect(() => {
    // "renew_extend" 仅在数量 > 1 时有效；数量回到 1 时切回 stack。
    if (selectedPurchaseMode === 'renew_extend' && Number(selectedPurchaseQuantity || 0) <= 1) {
      setSelectedPurchaseMode('stack');
    }
  }, [selectedPurchaseMode, selectedPurchaseQuantity]);

  useEffect(() => {
    if (!open || selectedPurchaseMode !== 'renew') return;
    if (selectedPlanRenewableSubscriptions.length === 0) {
      setSelectedRenewTargetSubscriptionId(0);
      return;
    }
    // 当存在多个目标时，默认选择最早到期的订阅。
    const currentSelectedId = Number(selectedRenewTargetSubscriptionId || 0);
    const selectedExists = selectedPlanRenewableSubscriptions.some(
      (sub) => Number(sub?.id || 0) === currentSelectedId,
    );
    if (!selectedExists) {
      setSelectedRenewTargetSubscriptionId(
        Number(selectedPlanRenewableSubscriptions[0]?.id || 0),
      );
    }
  }, [
    open,
    selectedPurchaseMode,
    selectedPlanRenewableSubscriptions,
    selectedRenewTargetSubscriptionId,
  ]);

  const openBuy = (p) => {
    const planId = p?.plan?.id;
    const activeQuantity = planId ? planActiveQuantityMap.get(planId) || 0 : 0;
    const planQuantityConfig = getPlanPurchaseQuantityConfig(
      p?.plan,
      activeQuantity,
    );
    setSelectedPlan(p);
    setSelectedPurchaseMode('stack');
    setSelectedPurchaseQuantity(
      planQuantityConfig.max > 0 ? planQuantityConfig.min : 0,
    );
    setSelectedRenewTargetSubscriptionId(0);
    setSelectedEpayMethod(epayMethods?.[0]?.type || '');
    setOpen(true);
  };

  const closeBuy = () => {
    setOpen(false);
    setSelectedPlan(null);
    setSelectedPurchaseMode('stack');
    setSelectedPurchaseQuantity(minPurchaseQuantity);
    setSelectedRenewTargetSubscriptionId(0);
    setPaying(false);
  };

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await reloadSubscriptionSelf?.();
    } finally {
      setRefreshing(false);
    }
  };

  const resolveRenewTargetSubscriptionId = () => {
    if (selectedPurchaseMode !== 'renew') return 0;
    if (!selectedPlanRenewable) return 0;
    const selectedId = Number(selectedRenewTargetSubscriptionId || 0);
    if (selectedId > 0) return selectedId;
    // 兜底：未设置时选最早到期；若无候选则保持 0，交由后端校验。
    return Number(selectedPlanRenewableSubscriptions[0]?.id || 0);
  };

  const ensureRenewTargetSelected = () => {
    if (selectedPurchaseMode !== 'renew') return true;
    if (!selectedPlanRenewable) return true;
    const renewTargetId = resolveRenewTargetSubscriptionId();
    if (renewTargetId > 0) return true;
    showError(t('请选择续费目标订阅'));
    return false;
  };

  const payStripe = async (autoRenew) => {
    if (!selectedPlan?.plan?.stripe_price_id) {
      showError(t('该套餐未配置 Stripe'));
      return;
    }
    if (!ensureRenewTargetSelected()) return;
    const renewTargetSubscriptionId = resolveRenewTargetSubscriptionId();
    setPaying(true);
    try {
      const res = await API.post('/api/subscription/stripe/pay', {
        auto_renew: autoRenew,
        plan_id: selectedPlan.plan.id,
        purchase_mode: selectedPurchaseMode,
        purchase_quantity: selectedPurchaseQuantity,
        renew_target_subscription_id: renewTargetSubscriptionId,
      });
      if (res.data?.message === 'success') {
        window.open(res.data.data?.pay_link, '_blank');
        showSuccess(t('已打开支付页面'));
        closeBuy();
      } else {
        const errorMsg =
          typeof res.data?.data === 'string'
            ? res.data.data
            : res.data?.message || t('支付失败');
        showError(errorMsg);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaying(false);
    }
  };

  const [cancellingRenewal, setCancellingRenewal] = useState(null);
  const cancelRenewal = async (id) => {
    setCancellingRenewal(id);
    try {
      const { data } = await API.post(`/api/subscription/stripe/renewals/${id}/cancel`);
      if (!data.success) { showError(data.message || t('请求失败，请重试')); return; }
      showSuccess(t('已设置到期停止续费'));
      await reloadSubscriptionSelf?.();
    } catch {
      showError(t('请求失败，请重试'));
    } finally {
      setCancellingRenewal(null);
    }
  };

  const payCreem = async () => {
    if (!selectedPlan?.plan?.creem_product_id) {
      showError(t('该套餐未配置 Creem'));
      return;
    }
    if (!ensureRenewTargetSelected()) return;
    const renewTargetSubscriptionId = resolveRenewTargetSubscriptionId();
    setPaying(true);
    try {
      const res = await API.post('/api/subscription/creem/pay', {
        plan_id: selectedPlan.plan.id,
        purchase_mode: selectedPurchaseMode,
        purchase_quantity: selectedPurchaseQuantity,
        renew_target_subscription_id: renewTargetSubscriptionId,
      });
      if (res.data?.message === 'success') {
        window.open(res.data.data?.checkout_url, '_blank');
        showSuccess(t('已打开支付页面'));
        closeBuy();
      } else {
        const errorMsg =
          typeof res.data?.data === 'string'
            ? res.data.data
            : res.data?.message || t('支付失败');
        showError(errorMsg);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaying(false);
    }
  };

  const payEpay = async () => {
    if (!selectedEpayMethod) {
      showError(t('请选择支付方式'));
      return;
    }
    if (!ensureRenewTargetSelected()) return;
    const renewTargetSubscriptionId = resolveRenewTargetSubscriptionId();
    setPaying(true);
    try {
      const res = await API.post('/api/subscription/epay/pay', {
        plan_id: selectedPlan.plan.id,
        payment_method: selectedEpayMethod,
        purchase_mode: selectedPurchaseMode,
        purchase_quantity: selectedPurchaseQuantity,
        renew_target_subscription_id: renewTargetSubscriptionId,
      });
      if (res.data?.message === 'success') {
        submitEpayForm({ url: res.data.url, params: res.data.data });
        showSuccess(t('已发起支付'));
        closeBuy();
      } else {
        const errorMsg =
          typeof res.data?.data === 'string'
            ? res.data.data
            : res.data?.message || t('支付失败');
        showError(errorMsg);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaying(false);
    }
  };

  // 当前订阅区块（支持多条订阅）。
  const hasActiveSubscription = activeSubscriptions.length > 0;
  const hasAnySubscription = allSubscriptions.length > 0;
  const disableSubscriptionPreference = !hasActiveSubscription;
  const isSubscriptionPreference =
    billingPreference === 'subscription_first' ||
    billingPreference === 'subscription_only';
  const displayBillingPreference =
    disableSubscriptionPreference && isSubscriptionPreference
      ? 'wallet_first'
      : billingPreference;
  const subscriptionPreferenceLabel =
    billingPreference === 'subscription_only' ? t('仅用订阅') : t('优先订阅');

  const planPurchaseCountMap = useMemo(() => {
    const map = new Map();
    (allSubscriptions || []).forEach((sub) => {
      const planId = sub?.subscription?.plan_id;
      if (!planId) return;
      map.set(planId, (map.get(planId) || 0) + 1);
    });
    return map;
  }, [allSubscriptions]);

  const planTitleMap = useMemo(() => {
    const map = new Map();
    (plans || []).forEach((p) => {
      const plan = p?.plan;
      if (!plan?.id) return;
      map.set(plan.id, plan.title || '');
    });
    return map;
  }, [plans]);

  const getPlanPurchaseCount = (planId) =>
    planPurchaseCountMap.get(planId) || 0;

  // 计算单条订阅剩余天数。
  const getRemainingDays = (sub) => {
    if (!sub?.subscription?.end_time) return 0;
    const now = Date.now() / 1000;
    const remaining = sub.subscription.end_time - now;
    return Math.max(0, Math.ceil(remaining / 86400));
  };

  // 计算单条订阅使用进度。
  const getUsagePercent = (sub) => {
    const total = Number(sub?.subscription?.amount_total || 0);
    const used = Number(sub?.subscription?.amount_used || 0);
    if (total <= 0) return 0;
    return Math.round((used / total) * 100);
  };

  const cardContent = (
    <>
      {/* 卡片头部 */}
      {loading ? (
        <div className='space-y-4'>
          {/* 我的订阅骨架屏 */}
          <Card className='!rounded-xl w-full' bodyStyle={{ padding: '12px' }}>
            <div className='flex items-center justify-between mb-3'>
              <Skeleton.Title active style={{ width: 100, height: 20 }} />
              <Skeleton.Button active style={{ width: 24, height: 24 }} />
            </div>
            <div className='space-y-2'>
              <Skeleton.Paragraph active rows={2} />
            </div>
          </Card>
          {/* 套餐列表骨架屏 */}
          <div className='grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 gap-5 w-full px-1'>
            {[1, 2, 3].map((i) => (
              <Card
                key={i}
                className='!rounded-xl w-full h-full'
                bodyStyle={{ padding: 16 }}
              >
                <Skeleton.Title
                  active
                  style={{ width: '60%', height: 24, marginBottom: 8 }}
                />
                <Skeleton.Paragraph
                  active
                  rows={1}
                  style={{ marginBottom: 12 }}
                />
                <div className='text-center py-4'>
                  <Skeleton.Title
                    active
                    style={{ width: '40%', height: 32, margin: '0 auto' }}
                  />
                </div>
                <Skeleton.Paragraph active rows={3} style={{ marginTop: 12 }} />
                <Skeleton.Button
                  active
                  block
                  style={{ marginTop: 16, height: 32 }}
                />
              </Card>
            ))}
          </div>
        </div>
      ) : (
        <Space vertical style={{ width: '100%' }} spacing={8}>
          {/* 当前订阅状态 */}
          <Card className='!rounded-xl w-full' bodyStyle={{ padding: '12px' }}>
            <div className='flex items-center justify-between mb-2 gap-3'>
              <div className='flex items-center gap-2 flex-1 min-w-0'>
                <Text strong>{t('我的订阅')}</Text>
                {hasActiveSubscription ? (
                  <Tag
                    color='white'
                    size='small'
                    shape='circle'
                    prefixIcon={<Badge dot type='success' />}
                  >
                    {activeSubscriptions.length} {t('个生效中')}
                  </Tag>
                ) : (
                  <Tag color='white' size='small' shape='circle'>
                    {t('无生效')}
                  </Tag>
                )}
                {allSubscriptions.length > activeSubscriptions.length && (
                  <Tag color='white' size='small' shape='circle'>
                    {allSubscriptions.length - activeSubscriptions.length}{' '}
                    {t('个已过期')}
                  </Tag>
                )}
              </div>
              <div className='flex items-center gap-2'>
                <Select
                  value={displayBillingPreference}
                  onChange={onChangeBillingPreference}
                  size='small'
                  optionList={[
                    {
                      value: 'subscription_first',
                      label: disableSubscriptionPreference
                        ? `${t('优先订阅')} (${t('无生效')})`
                        : t('优先订阅'),
                      disabled: disableSubscriptionPreference,
                    },
                    { value: 'wallet_first', label: t('优先钱包') },
                    {
                      value: 'subscription_only',
                      label: disableSubscriptionPreference
                        ? `${t('仅用订阅')} (${t('无生效')})`
                        : t('仅用订阅'),
                      disabled: disableSubscriptionPreference,
                    },
                    { value: 'wallet_only', label: t('仅用钱包') },
                  ]}
                />
                <Button
                  size='small'
                  theme='light'
                  type='tertiary'
                  icon={
                    <RefreshCw
                      size={12}
                      className={refreshing ? 'animate-spin' : ''}
                    />
                  }
                  onClick={handleRefresh}
                  loading={refreshing}
                />
              </div>
            </div>
            {disableSubscriptionPreference && isSubscriptionPreference && (
              <Text type='tertiary' size='small'>
                {t('已保存偏好为')}
                {subscriptionPreferenceLabel}
                {t('，当前无生效订阅，将自动使用钱包')}
              </Text>
            )}

            {stripeRenewals.map((renewal) => (
              <div key={renewal.id} className='flex flex-wrap items-center justify-between gap-2 my-2'>
                <Text>{renewal.title} · Stripe #{renewal.id} · {t('自动续费')}</Text>
                {renewal.cancel_at_period_end ? (
                  <Text type='tertiary'>{t('已设置到期停止续费')}</Text>
                ) : (
                  <Button size='small' loading={cancellingRenewal === renewal.id}
                    disabled={cancellingRenewal !== null} onClick={() => cancelRenewal(renewal.id)}>
                    {t('到期停止续费')}
                  </Button>
                )}
              </div>
            ))}
            {hasAnySubscription ? (
              <>
                <Divider margin={8} />
                <div className='flex items-center justify-between mb-2'>
                  <Text type='tertiary' size='small'>
                    {t('共')} {allSubscriptions.length} {t('条订阅')}
                  </Text>
                  <Button
                    size='small'
                    theme='borderless'
                    type='tertiary'
                    icon={
                      subscriptionListCollapsed ? (
                        <ChevronDown size={12} />
                      ) : (
                        <ChevronUp size={12} />
                      )
                    }
                    onClick={() =>
                      setSubscriptionListCollapsed((collapsed) => !collapsed)
                    }
                  >
                    {subscriptionListCollapsed ? t('展开') : t('收起')}
                  </Button>
                </div>
                {!subscriptionListCollapsed && (
                  <div className='max-h-64 overflow-y-auto pr-1 semi-table-body'>
                    {allSubscriptions.map((sub, subIndex) => {
                      const isLast = subIndex === allSubscriptions.length - 1;
                      const subscription = sub.subscription;
                      const totalAmount = Number(subscription?.amount_total || 0);
                      const usedAmount = Number(subscription?.amount_used || 0);
                      const remainAmount =
                        totalAmount > 0
                          ? Math.max(0, totalAmount - usedAmount)
                          : 0;
                      const planTitle =
                        planTitleMap.get(subscription?.plan_id) || '';
                      const remainDays = getRemainingDays(sub);
                      const usagePercent = getUsagePercent(sub);
                      const now = Date.now() / 1000;
                      const isExpired = (subscription?.end_time || 0) < now;
                      const isCancelled = subscription?.status === 'cancelled';
                      const isActive =
                        subscription?.status === 'active' && !isExpired;

                      return (
                        <div key={subscription?.id || subIndex}>
                          {/* 订阅摘要 */}
                          <div className='flex items-center justify-between text-xs mb-2'>
                            <div className='flex items-center gap-2'>
                              <span className='font-medium'>
                                {planTitle
                                  ? `${planTitle} · ${t('订阅')} #${subscription?.id}`
                                  : `${t('订阅')} #${subscription?.id}`}
                              </span>
                              {isActive ? (
                                <Tag
                                  color='white'
                                  size='small'
                                  shape='circle'
                                  prefixIcon={<Badge dot type='success' />}
                                >
                                  {t('生效')}
                                </Tag>
                              ) : isCancelled ? (
                                <Tag color='white' size='small' shape='circle'>
                                  {t('已作废')}
                                </Tag>
                              ) : (
                                <Tag color='white' size='small' shape='circle'>
                                  {t('已过期')}
                                </Tag>
                              )}
                            </div>
                            {isActive && (
                              <span className='text-gray-500'>
                                {t('剩余')} {remainDays} {t('天')}
                              </span>
                            )}
                          </div>
                          <div className='text-xs text-gray-500 mb-2'>
                            {isActive
                              ? t('至')
                              : isCancelled
                                ? t('作废于')
                                : t('过期于')}{' '}
                            {new Date(
                              (subscription?.end_time || 0) * 1000,
                            ).toLocaleString()}
                          </div>
                          <div className='text-xs text-gray-500 mb-2'>
                            {t('总额度')}:{' '}
                            {totalAmount > 0 ? (
                              <Tooltip
                                content={`${t('原生额度')}：${usedAmount}/${totalAmount} · ${t('剩余')} ${remainAmount}`}
                              >
                                <span>
                                  {renderQuota(usedAmount)}/
                                  {renderQuota(totalAmount)} · {t('剩余')}{' '}
                                  {renderQuota(remainAmount)}
                                </span>
                              </Tooltip>
                            ) : (
                              t('不限')
                            )}
                            {totalAmount > 0 && (
                              <span className='ml-2'>
                                {t('已用')} {usagePercent}%
                              </span>
                            )}
                          </div>
                          {!isLast && <Divider margin={12} />}
                        </div>
                      );
                    })}
                  </div>
                )}
              </>
            ) : (
              <div className='text-xs text-gray-500'>
                {t('购买套餐后即可享受模型权益')}
              </div>
            )}
          </Card>

          {/* 可购买套餐 - 标准定价卡片 */}
          {plans.length > 0 ? (
            <div className='grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 gap-5 w-full px-1'>
              {plans.map((p, index) => {
                const plan = p?.plan;
                const totalAmount = Number(plan?.total_amount || 0);
                const symbol = getPaymentCurrencySymbol();
                const price = Number(plan?.price_amount || 0);
                const convertedPrice = price;
                const displayPrice = convertedPrice.toFixed(
                  Number.isInteger(convertedPrice) ? 0 : 2,
                );
                const isPopular = index === 0 && plans.length > 1;
                const purchasedCount = getPlanPurchaseCount(plan?.id);
                const activeCount = Number(planActiveCountMap.get(plan?.id) || 0);
                const activeQuantity = Number(
                  planActiveQuantityMap.get(plan?.id) || 0,
                );
                const purchaseLimit = Number(plan?.max_purchase_per_user || 0);
                const stackLimit = Number(plan?.max_stack_per_user || 0);
                const quantityConfig = getPlanPurchaseQuantityConfig(
                  plan,
                  activeQuantity,
                );
                const remainPurchasable =
                  purchaseLimit > 0
                    ? Math.max(0, purchaseLimit - purchasedCount)
                    : -1;
                const remainStackable =
                  stackLimit > 0 ? Math.max(0, stackLimit - activeCount) : -1;
                const purchaseQuantityReached =
                  quantityConfig.max <= 0 ||
                  quantityConfig.max < quantityConfig.min;
                const purchaseLimitLabel =
                  purchaseLimit > 0
                    ? `${t('购买上限')}: ${purchaseLimit}${t('份')} (${t('剩余')}${remainPurchasable}${t('份')})`
                    : `${t('购买上限')}: ${t('不限')}`;
                const stackLimitLabel =
                  stackLimit > 0
                    ? `${t('叠加上限')}: ${stackLimit}${t('份')} (${t('剩余')}${remainStackable}${t('份')})`
                    : `${t('叠加上限')}: ${t('不限')}`;
                const quantityRuleLabel = purchaseQuantityReached
                  ? `${t('本次可买')}: 0${t('份')}`
                  : `${t('本次可买')}: ${quantityConfig.min}-${quantityConfig.max}${t('份')}`;
                const totalLabel =
                  totalAmount > 0
                    ? `${t('总额度')}: ${renderQuota(totalAmount)}`
                    : `${t('总额度')}: ${t('不限')}`;
                const upgradeLabel = plan?.upgrade_group
                  ? `${t('升级分组')}: ${plan.upgrade_group}`
                  : null;
                const resetLabel =
                  formatSubscriptionResetPeriod(plan, t) === t('不重置')
                    ? null
                    : `${t('额度重置')}: ${formatSubscriptionResetPeriod(plan, t)}`;
                const planBenefits = [
                  {
                    label: `${t('有效期')}: ${formatSubscriptionDuration(plan, t)}`,
                  },
                  resetLabel ? { label: resetLabel } : null,
                  totalAmount > 0
                    ? {
                        label: totalLabel,
                        tooltip: `${t('原生额度')}：${totalAmount}`,
                      }
                    : { label: totalLabel },
                  { label: purchaseLimitLabel },
                  { label: stackLimitLabel },
                  { label: quantityRuleLabel },
                  upgradeLabel ? { label: upgradeLabel } : null,
                ].filter(Boolean);

                return (
                  <Card
                    key={plan?.id}
                    className={`!rounded-xl transition-all hover:shadow-lg w-full h-full ${
                      isPopular ? 'ring-2 ring-purple-500' : ''
                    }`}
                    bodyStyle={{ padding: 0 }}
                  >
                    <div className='p-4 h-full flex flex-col'>
                      {/* 推荐标记 */}
                      {isPopular && (
                        <div className='mb-2'>
                          <Tag color='purple' shape='circle' size='small'>
                            <Sparkles size={10} className='mr-1' />
                            {t('推荐')}
                          </Tag>
                        </div>
                      )}
                      {/* 套餐标题 */}
                      <div className='mb-3'>
                        <Typography.Title
                          heading={5}
                          ellipsis={{ rows: 1, showTooltip: true }}
                          style={{ margin: 0 }}
                        >
                          {plan?.title || t('订阅套餐')}
                        </Typography.Title>
                        {plan?.subtitle && (
                          <Text
                            type='tertiary'
                            size='small'
                            ellipsis={{ rows: 1, showTooltip: true }}
                            style={{ display: 'block' }}
                          >
                            {plan.subtitle}
                          </Text>
                        )}
                      </div>

                      {/* 价格区域 */}
                      <div className='py-2'>
                        <div className='flex items-baseline justify-start'>
                          <span className='text-xl font-bold text-purple-600'>
                            {symbol}
                          </span>
                          <span className='text-3xl font-bold text-purple-600'>
                            {displayPrice}
                          </span>
                        </div>
                      </div>

                      {/* 权益说明 */}
                      <div className='flex flex-col items-start gap-1 pb-2'>
                        {planBenefits.map((item) => {
                          const content = (
                            <div className='flex items-center gap-2 text-xs text-gray-500'>
                              <Badge dot type='tertiary' />
                              <span>{item.label}</span>
                            </div>
                          );
                          if (!item.tooltip) {
                            return (
                              <div
                                key={item.label}
                                className='w-full flex justify-start'
                              >
                                {content}
                              </div>
                            );
                          }
                          return (
                            <Tooltip key={item.label} content={item.tooltip}>
                              <div className='w-full flex justify-start'>
                                {content}
                              </div>
                            </Tooltip>
                          );
                        })}
                      </div>

                      <div className='mt-auto'>
                        <Divider margin={12} />

                        {/* 购买按钮 */}
                        {(() => {
                          const canRenew = activeCount > 0;
                          const purchaseReached =
                            purchaseLimit > 0 && purchasedCount >= purchaseLimit;
                          const stackReached =
                            stackLimit > 0 && activeCount >= stackLimit;
                          const quantityReached = purchaseQuantityReached;
                          // 若无可续费订阅，触达任一上限即禁止购买；否则可在弹窗中选择续费目标。
                          const reached =
                            quantityReached || (!canRenew && (purchaseReached || stackReached));
                          const tip = quantityReached
                            ? t('当前可购买数量为 0，请等待部分订阅到期后再试')
                            : purchaseReached
                            ? t('已达到购买上限') +
                              ` (${purchasedCount}/${purchaseLimit})`
                            : stackReached
                              ? t('已达到叠加上限') + ` (${activeCount}/${stackLimit})`
                              : '';
                          const buttonEl = (
                            <Button
                              theme='outline'
                              type='primary'
                              block
                              disabled={reached}
                              onClick={() => {
                                if (!reached) openBuy(p);
                              }}
                            >
                              {reached ? t('已达上限') : t('立即订阅')}
                            </Button>
                          );
                          return reached ? (
                            <Tooltip content={tip} position='top'>
                              {buttonEl}
                            </Tooltip>
                          ) : (
                            buttonEl
                          );
                        })()}
                      </div>
                    </div>
                  </Card>
                );
              })}
            </div>
          ) : (
            <div className='text-center text-gray-400 text-sm py-4'>
              {t('暂无可购买套餐')}
            </div>
          )}
        </Space>
      )}
    </>
  );

  return (
    <>
      {withCard ? (
        <Card className='!rounded-2xl shadow-sm border-0'>{cardContent}</Card>
      ) : (
        <div className='space-y-3'>{cardContent}</div>
      )}

      {/* 购买确认弹窗 */}
      <SubscriptionPurchaseModal
        t={t}
        visible={open}
        onCancel={closeBuy}
        selectedPlan={selectedPlan}
        paying={paying}
        selectedEpayMethod={selectedEpayMethod}
        setSelectedEpayMethod={setSelectedEpayMethod}
        epayMethods={epayMethods}
        enableOnlineTopUp={enableOnlineTopUp}
        enableStripeTopUp={enableStripeTopUp}
        enableCreemTopUp={enableCreemTopUp}
        purchaseMode={selectedPurchaseMode}
        onChangePurchaseMode={setSelectedPurchaseMode}
        purchaseQuantity={selectedPurchaseQuantity}
        onChangePurchaseQuantity={setSelectedPurchaseQuantity}
        purchaseQuantityRange={selectedPlanQuantityConfig}
        canRenew={selectedPlanRenewable}
        renewableSubscriptions={selectedPlanRenewableSubscriptions}
        renewTargetSubscriptionId={selectedRenewTargetSubscriptionId}
        onChangeRenewTargetSubscriptionId={setSelectedRenewTargetSubscriptionId}
        purchaseLimitInfo={
          selectedPlan?.plan?.id
            ? {
                purchase_limit: Number(
                  selectedPlan?.plan?.max_purchase_per_user || 0,
                ),
                purchase_count: getPlanPurchaseCount(selectedPlan?.plan?.id),
                stack_limit: Number(selectedPlan?.plan?.max_stack_per_user || 0),
                stack_count: Number(
                  planActiveCountMap.get(selectedPlan?.plan?.id) || 0,
                ),
                quantity_max: Number(
                  selectedPlan?.plan?.purchase_quantity_max || 12,
                ),
                active_quantity: Number(
                  planActiveQuantityMap.get(selectedPlan?.plan?.id) || 0,
                ),
              }
            : null
        }
        onPayStripe={payStripe}
        onPayCreem={payCreem}
        onPayEpay={payEpay}
      />
    </>
  );
};

export default SubscriptionPlansCard;
