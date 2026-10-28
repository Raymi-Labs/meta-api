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

export const MODEL_DETAIL_ROUTE = '/model/:modelName';

export const buildModelDetailPath = (modelName) => {
  if (typeof modelName !== 'string' || !modelName) return '/pricing';
  try {
    return `/model/${encodeURIComponent(modelName)}`;
  } catch {
    return '/pricing';
  }
};

export const isModelDetailPath = (pathname) =>
  /^\/model\/[^/]+\/?$/.test(pathname);

// Read the raw pathname once: React Router params decode literal "%2F" a
// second time, which would conflate "vendor/model" and "vendor%2Fmodel".
export const getModelNameFromDetailPath = (pathname) => {
  if (typeof pathname !== 'string' || !isModelDetailPath(pathname)) return null;
  try {
    return decodeURIComponent(
      pathname.slice('/model/'.length).replace(/\/$/, ''),
    );
  } catch {
    return null;
  }
};

const enumValue = (value, allowed, fallback) =>
  allowed.includes(value) ? value : fallback;
const filterValue = (value) =>
  typeof value === 'string' && value.length > 0 && value.length <= 512
    ? value
    : 'all';

// Only presentation state travels through history. Pricing, user identity,
// model records and permissions are always fetched for the current account.
export const sanitizeModelCatalogSnapshot = (value) => {
  const state = value && typeof value === 'object' ? value : {};
  const showWithRecharge =
    typeof state.showWithRecharge === 'boolean' ? state.showWithRecharge : true;
  return {
    searchValue:
      typeof state.searchValue === 'string'
        ? state.searchValue.slice(0, 1000)
        : '',
    filterGroup: 'all',
    filterQuotaType: enumValue(state.filterQuotaType, ['all', 0, 1, 2], 'all'),
    filterEndpointType: filterValue(state.filterEndpointType),
    filterVendor: filterValue(state.filterVendor),
    filterTag: filterValue(state.filterTag),
    pageSize: enumValue(state.pageSize, [10, 20, 50, 100], 20),
    currentPage:
      Number.isSafeInteger(state.currentPage) && state.currentPage > 0
        ? state.currentPage
        : 1,
    sortOrder: enumValue(
      state.sortOrder,
      [
        'release-desc',
        'version-desc',
        'default',
        'input-asc',
        'input-desc',
        'output-asc',
        'output-desc',
        'request-asc',
        'request-desc',
      ],
      'release-desc',
    ),
    tokenUnit: enumValue(state.tokenUnit, ['K', 'M'], 'M'),
    currency: enumValue(
      state.currency,
      ['USD', 'CNY', 'CUSTOM'],
      showWithRecharge ? 'CNY' : 'USD',
    ),
    showWithRecharge,
    priceConvertMode: enumValue(
      state.priceConvertMode,
      ['package', 'recharge'],
      'package',
    ),
    selectedPlanId:
      Number.isSafeInteger(state.selectedPlanId) && state.selectedPlanId > 0
        ? state.selectedPlanId
        : null,
    showRatio: state.showRatio === true,
    viewMode: enumValue(state.viewMode, ['card', 'table'], 'card'),
  };
};
