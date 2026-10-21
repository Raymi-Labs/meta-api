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

import { sortPricingModels } from '../../helpers/modelPriceSorting';
import { groupForPricingVendor } from '../../helpers/pricingVendorSelection';
import { calculateModelPrice } from '../../helpers/utils';

import { useState, useEffect, useContext, useRef, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { API, copy, showError, showSuccess } from '../../helpers';
import { Modal } from '@douyinfe/semi-ui';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { getQuotaPerUnit } from '../../helpers/quota';
import {
  resolveModelPricingCurrency,
  resolveModelPricingRate,
  resolveModelPricingSymbol,
} from '../../helpers/modelPricingCurrency';

const normalizeSubscriptionPlans = (items) => {
  if (!Array.isArray(items)) return [];
  return items.map((item) => item?.plan || item).filter(Boolean);
};

const pickBestValuePlan = (plans, quotaPerUnit, usdExchangeRate) => {
  if (!Array.isArray(plans) || plans.length === 0) return null;

  let bestPlan = null;
  let bestRate = Infinity;

  for (const plan of plans) {
    const rate = computePackageEffectiveRate(
      plan,
      quotaPerUnit,
      usdExchangeRate,
    );
    if (rate != null && rate < bestRate) {
      bestRate = rate;
      bestPlan = plan;
    }
  }

  return bestPlan || plans[0];
};

// 计算套餐的实际汇率（CNY/USD）
const computePackageEffectiveRate = (plan, quotaPerUnit, usdExchangeRate) => {
  const totalAmount = Number(plan?.total_amount || 0);
  if (!plan || totalAmount <= 0) return null;

  const quotaPerResetUSD = totalAmount / quotaPerUnit;

  // 套餐时长（秒）
  let durationSeconds = 0;
  switch (plan.duration_unit) {
    case 'year':
      durationSeconds = plan.duration_value * 365 * 86400;
      break;
    case 'month':
      durationSeconds = plan.duration_value * 30 * 86400;
      break;
    case 'day':
      durationSeconds = plan.duration_value * 86400;
      break;
    case 'hour':
      durationSeconds = plan.duration_value * 3600;
      break;
    case 'custom':
      durationSeconds = plan.custom_seconds || 0;
      break;
  }

  let totalQuotaUSD;
  if (plan.quota_reset_period === 'never') {
    totalQuotaUSD = quotaPerResetUSD;
  } else {
    let resetSeconds;
    switch (plan.quota_reset_period) {
      case 'daily':
        resetSeconds = 86400;
        break;
      case 'weekly':
        resetSeconds = 7 * 86400;
        break;
      case 'monthly':
        resetSeconds = 30 * 86400;
        break;
      case 'custom':
        resetSeconds = plan.quota_reset_custom_seconds || durationSeconds;
        break;
      default:
        resetSeconds = durationSeconds;
        break;
    }
    const numPeriods =
      resetSeconds > 0
        ? Math.max(1, Math.floor(durationSeconds / resetSeconds))
        : 1;
    totalQuotaUSD = quotaPerResetUSD * numPeriods;
  }

  if (totalQuotaUSD <= 0) return null;

  // 统一转为 CNY
  let planPriceCNY = Number(plan?.price_amount || 0);
  if (plan.currency === 'USD') {
    planPriceCNY *= usdExchangeRate;
  }

  return planPriceCNY / totalQuotaUSD;
};

export const useModelPricingData = () => {
  const { t } = useTranslation();
  const [searchValue, setSearchValue] = useState('');
  const compositionRef = useRef({ isComposition: false });
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [modalImageUrl, setModalImageUrl] = useState('');
  const [isModalOpenurl, setIsModalOpenurl] = useState(false);
  const [selectedGroup, setSelectedGroup] = useState('all');
  const [sortOrder, setSortOrder] = useState('version-desc');
  const changeSortOrder = (value) => {
    setSortOrder(value);
    setCurrentPage(1);
  };
  const [showModelDetail, setShowModelDetail] = useState(false);
  const [selectedModel, setSelectedModel] = useState(null);
  const [filterGroup, setFilterGroup] = useState('all'); // 用于 Table 的可用分组筛选，"all" 表示不过滤
  const [filterQuotaType, setFilterQuotaType] = useState('all'); // 计费类型筛选: 'all' | 0 | 1
  const [filterEndpointType, setFilterEndpointType] = useState('all'); // 端点类型筛选: 'all' | string
  const [filterVendor, setFilterVendor] = useState('all'); // 供应商筛选: 'all' | 'unknown' | string
  const [filterTag, setFilterTag] = useState('all'); // 模型标签筛选: 'all' | string
  const [pageSize, setPageSize] = useState(20);
  const [currentPage, setCurrentPage] = useState(1);
  const [currency, setCurrency] = useState('USD');
  const [showWithRecharge, setShowWithRecharge] = useState(true);
  const [tokenUnit, setTokenUnit] = useState('M');
  const [models, setModels] = useState([]);
  const [vendorsMap, setVendorsMap] = useState({});
  const [loading, setLoading] = useState(true);
  const [groupRatio, setGroupRatio] = useState({});
  const [timeRatioMap, setTimeRatioMap] = useState({});
  const [timeRatioAt, setTimeRatioAt] = useState('');
  const [usableGroup, setUsableGroup] = useState({});
  const [endpointMap, setEndpointMap] = useState({});
  const [autoGroups, setAutoGroups] = useState([]);

  // 价格转换模式：'recharge'=充值汇率, 'package'=套餐汇率
  const [priceConvertMode, setPriceConvertMode] = useState('package');
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [subscriptionPlansLoaded, setSubscriptionPlansLoaded] = useState(false);
  const [selectedPlanId, setSelectedPlanId] = useState(null);

  const [statusState] = useContext(StatusContext);
  const [userState] = useContext(UserContext);

  // 充值汇率（price）与美元兑人民币汇率（usd_exchange_rate）
  const priceRate = useMemo(
    () => statusState?.status?.price ?? 1,
    [statusState],
  );
  const usdExchangeRate = useMemo(
    () => statusState?.status?.usd_exchange_rate ?? priceRate,
    [statusState, priceRate],
  );
  const customExchangeRate = useMemo(
    () => statusState?.status?.custom_currency_exchange_rate ?? 1,
    [statusState],
  );
  const customCurrencySymbol = useMemo(
    () => statusState?.status?.custom_currency_symbol ?? '¤',
    [statusState],
  );
  const quotaDisplayType = useMemo(
    () => statusState?.status?.quota_display_type ?? 'USD',
    [statusState],
  );

  useEffect(() => {
    setCurrency(
      resolveModelPricingCurrency({ showWithRecharge, quotaDisplayType }),
    );
  }, [showWithRecharge, quotaDisplayType]);

  // 可用套餐（过滤掉无限额度的）
  const availablePlans = useMemo(
    () => subscriptionPlans.filter((p) => Number(p?.total_amount || 0) > 0),
    [subscriptionPlans],
  );

  // 当前选中的套餐
  const selectedPlan = useMemo(
    () =>
      availablePlans.find((p) => Number(p.id) === Number(selectedPlanId)) ||
      null,
    [availablePlans, selectedPlanId],
  );

  // 套餐加载后自动选中第一个（仅在 package 模式下且尚未选择时）
  useEffect(() => {
    if (priceConvertMode !== 'package' || availablePlans.length === 0) {
      return;
    }

    const currentPlanStillAvailable = availablePlans.some(
      (plan) => Number(plan.id) === Number(selectedPlanId),
    );
    if (currentPlanStillAvailable) {
      return;
    }

    const quotaPerUnit = getQuotaPerUnit();
    const bestPlan = pickBestValuePlan(
      availablePlans,
      quotaPerUnit,
      usdExchangeRate,
    );

    if (bestPlan?.id != null) {
      setSelectedPlanId(bestPlan.id);
    }
  }, [priceConvertMode, selectedPlanId, availablePlans, usdExchangeRate]);

  useEffect(() => {
    if (!subscriptionPlansLoaded) {
      return;
    }
    if (availablePlans.length > 0) return;
    if (priceConvertMode === 'package') {
      setPriceConvertMode('recharge');
    }
    if (selectedPlanId != null) {
      setSelectedPlanId(null);
    }
  }, [
    subscriptionPlansLoaded,
    availablePlans,
    priceConvertMode,
    selectedPlanId,
  ]);

  // 套餐实际汇率（CNY/USD）
  const packageEffectiveRate = useMemo(() => {
    const quotaPerUnit = getQuotaPerUnit();
    return computePackageEffectiveRate(
      selectedPlan,
      quotaPerUnit,
      usdExchangeRate,
    );
  }, [selectedPlan, usdExchangeRate]);

  const filteredModels = useMemo(() => {
    let result = models;

    // 分组筛选
    if (filterGroup !== 'all') {
      result = result.filter((model) =>
        model.enable_groups.includes(filterGroup),
      );
    }

    // 计费类型筛选
    if (filterQuotaType !== 'all') {
      result = result.filter((model) => model.quota_type === filterQuotaType);
    }

    // 端点类型筛选
    if (filterEndpointType !== 'all') {
      result = result.filter(
        (model) =>
          model.supported_endpoint_types &&
          model.supported_endpoint_types.includes(filterEndpointType),
      );
    }

    // 供应商筛选
    if (filterVendor !== 'all') {
      if (filterVendor === 'unknown') {
        result = result.filter((model) => !model.vendor_name);
      } else {
        result = result.filter((model) => model.vendor_name === filterVendor);
      }
    }

    // 标签筛选
    if (filterTag !== 'all') {
      const tagLower = filterTag.toLowerCase();
      result = result.filter((model) => {
        if (!model.tags) return false;
        const tagsArr = model.tags
          .toLowerCase()
          .split(/[,;|]+/)
          .map((tag) => tag.trim())
          .filter(Boolean);
        return tagsArr.includes(tagLower);
      });
    }

    // 搜索筛选
    if (searchValue.length > 0) {
      const searchTerm = searchValue.toLowerCase();
      result = result.filter(
        (model) =>
          (model.model_name &&
            model.model_name.toLowerCase().includes(searchTerm)) ||
          (model.description &&
            model.description.toLowerCase().includes(searchTerm)) ||
          (model.tags && model.tags.toLowerCase().includes(searchTerm)) ||
          (model.vendor_name &&
            model.vendor_name.toLowerCase().includes(searchTerm)),
      );
    }

    return sortPricingModels(
      result,
      sortOrder,
      (record) =>
        calculateModelPrice({
          record,
          selectedGroup,
          groupRatio,
          timeRatioMap,
          tokenUnit: 'M',
          currency: 'USD',
          displayPrice: (value) => `$${value}`,
        }).sortPrices,
    );
  }, [
    sortOrder,
    selectedGroup,
    groupRatio,
    timeRatioMap,
    models,
    searchValue,
    filterGroup,
    filterQuotaType,
    filterEndpointType,
    filterVendor,
    filterTag,
  ]);

  const rowSelection = useMemo(
    () => ({
      selectedRowKeys,
      onChange: (keys) => {
        setSelectedRowKeys(keys);
      },
    }),
    [selectedRowKeys],
  );

  const convertDisplayPriceAmount = (usdPrice) => {
    const numericUSDPrice = Number(usdPrice || 0);
    if (!Number.isFinite(numericUSDPrice) || numericUSDPrice <= 0) {
      return 0;
    }
    const rate = resolveModelPricingRate({
      showWithRecharge,
      currency,
      priceConvertMode,
      priceRate,
      packageEffectiveRate,
      customExchangeRate,
    });
    return numericUSDPrice * rate;
  };

  const displayPrice = (usdPrice) => {
    const amount = convertDisplayPriceAmount(usdPrice);
    const symbol = resolveModelPricingSymbol(currency, customCurrencySymbol);
    return `${symbol}${amount.toFixed(3)}`;
  };
  displayPrice.toAmount = convertDisplayPriceAmount;
  displayPrice.currencySymbol = resolveModelPricingSymbol(
    currency,
    customCurrencySymbol,
  );

  const setModelsFormat = (models, groupRatio, vendorMap) => {
    for (let i = 0; i < models.length; i++) {
      const m = models[i];
      m.key = m.model_name;
      m.group_ratio = groupRatio[m.model_name];

      if (m.vendor_id && vendorMap[m.vendor_id]) {
        const vendor = vendorMap[m.vendor_id];
        m.vendor_name = vendor.name;
        m.vendor_icon = vendor.icon;
        m.vendor_description = vendor.description;
      }
    }
    models.sort((a, b) => {
      return a.quota_type - b.quota_type;
    });

    models.sort((a, b) => {
      if (a.model_name.startsWith('gpt') && !b.model_name.startsWith('gpt')) {
        return -1;
      } else if (
        !a.model_name.startsWith('gpt') &&
        b.model_name.startsWith('gpt')
      ) {
        return 1;
      } else {
        return a.model_name.localeCompare(b.model_name);
      }
    });

    setModels(models);
  };

  const loadPricing = async () => {
    setLoading(true);
    let url = '/api/pricing';
    const res = await API.get(url);
    const {
      success,
      message,
      data,
      vendors,
      group_ratio,
      usable_group,
      supported_endpoint,
      auto_groups,
      time_ratio,
      time_ratio_at,
    } = res.data;
    if (success) {
      setGroupRatio(group_ratio);
      setTimeRatioMap(time_ratio || {});
      setTimeRatioAt(time_ratio_at || '');
      setUsableGroup(usable_group);
      setSelectedGroup('all');
      // 构建供应商 Map 方便查找
      const vendorMap = {};
      if (Array.isArray(vendors)) {
        vendors.forEach((v) => {
          vendorMap[v.id] = v;
        });
      }
      setVendorsMap(vendorMap);
      setEndpointMap(supported_endpoint || {});
      setAutoGroups(auto_groups || []);
      setModelsFormat(data, group_ratio, vendorMap);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // 获取订阅套餐列表（需登录，静默失败）
  const loadSubscriptionPlans = async () => {
    try {
      const res = await API.get('/api/subscription/plans');
      if (res.data?.success) {
        setSubscriptionPlans(normalizeSubscriptionPlans(res.data.data));
      }
    } catch {
      // 未登录或接口不可用，静默忽略
    } finally {
      setSubscriptionPlansLoaded(true);
    }
  };

  const refresh = async () => {
    await loadPricing();
    // 套餐数据加载不阻塞主流程
    loadSubscriptionPlans();
  };

  const copyText = async (text) => {
    if (await copy(text)) {
      showSuccess(t('已复制：') + text);
    } else {
      Modal.error({ title: t('无法复制到剪贴板，请手动复制'), content: text });
    }
  };

  const handleChange = (value) => {
    const newSearchValue = value ? value : '';
    setSearchValue(newSearchValue);
  };

  const handleCompositionStart = () => {
    compositionRef.current.isComposition = true;
  };

  const handleCompositionEnd = (event) => {
    compositionRef.current.isComposition = false;
    const value = event.target.value;
    const newSearchValue = value ? value : '';
    setSearchValue(newSearchValue);
  };

  const handleGroupClick = (group) => {
    setSelectedGroup(group);
    setFilterGroup(group);
  };

  const handleVendorChange = (vendor) => {
    if (vendor === filterVendor) return;
    setFilterVendor(vendor);
    const group = groupForPricingVendor({
      models,
      usableGroup,
      group: filterGroup,
      vendor,
    });
    if (group !== filterGroup) {
      setFilterGroup(group);
      setSelectedGroup(group);
    }
    setCurrentPage(1);
  };

  const openModelDetail = (model) => {
    setSelectedModel(model);
    setShowModelDetail(true);
  };

  const closeModelDetail = () => {
    setShowModelDetail(false);
    setTimeout(() => {
      setSelectedModel(null);
    }, 300);
  };

  useEffect(() => {
    refresh().then();
  }, []);

  // 当筛选条件变化时重置到第一页
  useEffect(() => {
    setCurrentPage(1);
  }, [
    filterGroup,
    filterQuotaType,
    filterEndpointType,
    filterVendor,
    filterTag,
    searchValue,
  ]);

  return {
    // 状态
    sortOrder,
    setSortOrder: changeSortOrder,
    searchValue,
    setSearchValue,
    selectedRowKeys,
    setSelectedRowKeys,
    modalImageUrl,
    setModalImageUrl,
    isModalOpenurl,
    setIsModalOpenurl,
    selectedGroup,
    setSelectedGroup,
    showModelDetail,
    setShowModelDetail,
    selectedModel,
    setSelectedModel,
    filterGroup,
    setFilterGroup,
    filterQuotaType,
    setFilterQuotaType,
    filterEndpointType,
    setFilterEndpointType,
    filterVendor,
    setFilterVendor: handleVendorChange,
    filterTag,
    setFilterTag,
    pageSize,
    setPageSize,
    currentPage,
    setCurrentPage,
    currency,
    setCurrency,
    showWithRecharge,
    setShowWithRecharge,
    priceConvertMode,
    setPriceConvertMode,
    subscriptionPlans,
    availablePlans,
    selectedPlanId,
    setSelectedPlanId,
    tokenUnit,
    setTokenUnit,
    models,
    loading,
    groupRatio,
    timeRatioMap,
    timeRatioAt,
    usableGroup,
    endpointMap,
    autoGroups,

    // 计算属性
    priceRate,
    usdExchangeRate,
    filteredModels,
    rowSelection,

    // 供应商
    vendorsMap,

    // 用户和状态
    userState,
    statusState,

    // 方法
    displayPrice,
    refresh,
    copyText,
    handleChange,
    handleCompositionStart,
    handleCompositionEnd,
    handleGroupClick,
    openModelDetail,
    closeModelDetail,

    // 引用
    compositionRef,

    // 国际化
    t,
  };
};
