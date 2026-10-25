import assert from 'node:assert/strict';
import test from 'node:test';
import { matchRoutes } from 'react-router-dom';
import {
  MODEL_DETAIL_ROUTE,
  buildModelDetailPath,
  getModelNameFromDetailPath,
  isModelDetailPath,
  sanitizeModelCatalogSnapshot,
} from '../src/helpers/modelDetailNavigation.js';
import { getPublicShellPage } from '../src/helpers/publicRoutes.js';

test('encoded detail links preserve exact model IDs, including literal percent sequences', () => {
  for (const name of [
    'kimi-k3.1-pro',
    'vendor/model',
    'vendor%2Fmodel',
    'name?query#hash%',
    '中文 模型',
  ]) {
    const path = buildModelDetailPath(name);
    assert.ok(matchRoutes([{ path: MODEL_DETAIL_ROUTE }], path), name);
    assert.equal(getModelNameFromDetailPath(path), name);
    assert.equal(getModelNameFromDetailPath(path + '/'), name);
    assert.equal(getPublicShellPage(path), null);
  }
  assert.notEqual(
    buildModelDetailPath('vendor/model'),
    buildModelDetailPath('vendor%2Fmodel'),
  );
});

test('malformed and unrelated URLs cannot resolve another model', () => {
  for (const path of [
    '/model',
    '/models/name',
    '/model/',
    '/model/a/b',
    '/model/%',
    '/model/%E0%A4%A',
    '/console/model/name',
  ]) {
    assert.equal(getModelNameFromDetailPath(path), null, path);
  }
  assert.equal(isModelDetailPath('/model/unknown-name'), true);
  assert.equal(
    getModelNameFromDetailPath('/model/unknown-name'),
    'unknown-name',
  );
  for (const value of [null, undefined, '', 123, '\uD800']) {
    assert.equal(buildModelDetailPath(value), '/pricing');
  }
});

test('catalog history restores filters and display choices without model or account data', () => {
  const original = {
    searchValue: 'kimi',
    filterVendor: 'Moonshot',
    filterTag: 'tools',
    filterEndpointType: 'openai',
    filterQuotaType: 0,
    filterGroup: 'private',
    currentPage: 3,
    pageSize: 50,
    sortOrder: 'input-asc',
    tokenUnit: 'K',
    currency: 'CUSTOM',
    showWithRecharge: false,
    priceConvertMode: 'recharge',
    selectedPlanId: 12,
    showRatio: true,
    viewMode: 'table',
    models: [{ model_name: 'private-model' }],
    groupRatio: { private: 0.1 },
    user: { id: 7 },
    selectedModel: { model_name: 'private-model' },
  };
  const snapshot = sanitizeModelCatalogSnapshot(original);
  const detailsState = { catalogState: snapshot };
  const backState = { modelCatalogSnapshot: detailsState.catalogState };
  assert.deepEqual(
    sanitizeModelCatalogSnapshot(backState.modelCatalogSnapshot),
    {
      searchValue: 'kimi',
      filterVendor: 'Moonshot',
      filterTag: 'tools',
      filterEndpointType: 'openai',
      filterQuotaType: 0,
      filterGroup: 'all',
      currentPage: 3,
      pageSize: 50,
      sortOrder: 'input-asc',
      tokenUnit: 'K',
      currency: 'CUSTOM',
      showWithRecharge: false,
      priceConvertMode: 'recharge',
      selectedPlanId: 12,
      showRatio: true,
      viewMode: 'table',
    },
  );
  assert.equal(original.filterGroup, 'private');
});

test('unknown history values fall back safely and cannot restore hidden group filtering', () => {
  const defaults = sanitizeModelCatalogSnapshot();
  assert.deepEqual(
    sanitizeModelCatalogSnapshot({
      filterVendor: {},
      filterTag: '',
      filterQuotaType: 9,
      filterEndpointType: [],
      currentPage: -2,
      pageSize: 5000,
      sortOrder: 'invented',
      tokenUnit: 'B',
      currency: 'EUR',
      showWithRecharge: 'false',
      priceConvertMode: 'discount',
      selectedPlanId: '12',
      showRatio: 1,
      viewMode: 'unknown',
      filterGroup: 'private',
    }),
    defaults,
  );
  assert.equal(
    sanitizeModelCatalogSnapshot({ currentPage: Infinity }).currentPage,
    1,
  );
  assert.equal(
    sanitizeModelCatalogSnapshot({ searchValue: 'a'.repeat(1001) }).searchValue
      .length,
    1000,
  );
});
