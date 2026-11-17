import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  createEnterpriseOperationStore,
  listPendingEnterpriseReservations,
} from '../src/helpers/enterpriseOperation.js';
import { parseEnterpriseQuota } from '../src/helpers/enterpriseQuota.js';
const storage = () => {
  const data = {};
  return Object.defineProperties(data, {
    getItem: { value: (k) => data[k] ?? null },
    setItem: {
      value: (k, v) => {
        data[k] = v;
      },
    },
    removeItem: {
      value: (k) => {
        delete data[k];
      },
    },
  });
};
test('a lost response and remount preserve original key, target and amount', () => {
  const s = storage();
  let keys = 0;
  const first = createEnterpriseOperationStore(
    s,
    'actor1:enterprise1',
    () => `key-${++keys}`,
  );
  const intent = first.begin('/allocate', { amount: 299, reason: 'budget' });
  const reopened = createEnterpriseOperationStore(
    s,
    'actor1:enterprise1',
    () => `key-${++keys}`,
  );
  assert.deepEqual(reopened.read(), intent);
  assert.deepEqual(reopened.begin('/allocate', { amount: 999 }), intent);
  assert.throws(() => reopened.begin('/other', { amount: 999 }), /无法恢复/);
  assert.equal(keys, 1);
  reopened.clear('foreign-key');
  assert.deepEqual(reopened.read(), intent);
  reopened.clear(intent.key);
  assert.equal(reopened.read(), null);
  assert.notEqual(reopened.begin('/allocate', { amount: 299 }).key, intent.key);
});
test('pending operations cannot leak to another actor or enterprise', () => {
  const s = storage();
  const a = createEnterpriseOperationStore(s, '1:10', () => 'key-a');
  a.begin('/a', { amount: 1 });
  for (const scope of ['2:10', '1:20'])
    assert.equal(
      createEnterpriseOperationStore(s, scope, () => 'b').read(),
      null,
    );
});
test('storage failure occurs before a request can be started', () => {
  const s = {
    getItem: () => null,
    setItem: () => {
      throw new Error('storage denied');
    },
    removeItem: () => {},
  };
  assert.throws(
    () =>
      createEnterpriseOperationStore(s, 'a', () => 'key').begin('/allocate', {
        amount: 1,
      }),
    /storage denied/,
  );
});
test('quota parsing handles exact micro amounts, raw 299 and rejects blank or overflow', () => {
  const usd = { type: 'USD', quotaPerUnit: 500000 };
  assert.equal(parseEnterpriseQuota('0.000002', usd), 1);
  assert.equal(parseEnterpriseQuota('0.000598', usd), 299);
  assert.equal(
    parseEnterpriseQuota('0.1', { ...usd, type: 'CNY', rate: '6.6' }),
    7576,
  );
  assert.equal(parseEnterpriseQuota('299', { raw: true }), 299);
  assert.equal(parseEnterpriseQuota('1000000000000000', { raw: true }), 1e15);
  for (const v of [
    '',
    null,
    undefined,
    '-1',
    'NaN',
    'Infinity',
    '1e20',
    '1000000000000001',
  ])
    assert.equal(parseEnterpriseQuota(v, { raw: true }), null);
  assert.equal(parseEnterpriseQuota('1.5', { raw: true }), null);
  assert.equal(parseEnterpriseQuota('0', usd), 0);
});

test('a legacy server response may only be queried, never posted again', () => {
  const s = storage();
  const a = createEnterpriseOperationStore(s, '1:10', () => 'key-a');
  const intent = a.begin('/a', { amount: 1 });
  a.queryOnly(intent.key);
  const remount = createEnterpriseOperationStore(s, '1:10', () => 'key-b');
  assert.equal(remount.begin('/a', { amount: 2 }).queryOnly, true);
  assert.equal(remount.read().data.amount, 1);
});

test('logout cleanup only removes financial drafts, preserving other tabs state', async () => {
  const { clearEnterpriseOperationDrafts } = await import(
    '../src/helpers/enterpriseOperation.js'
  );
  const s = {
    'enterprise-operation:v1:1:10': 'pending',
    'other-preference': 'keep',
    removeItem(k) {
      delete this[k];
    },
  };
  clearEnterpriseOperationDrafts(s);
  assert.equal(s['enterprise-operation:v1:1:10'], undefined);
  assert.equal(s['other-preference'], 'keep');
  assert.doesNotThrow(() =>
    clearEnterpriseOperationDrafts(
      new Proxy(
        {},
        {
          ownKeys() {
            throw new Error('disabled');
          },
        },
      ),
    ),
  );
});

test('a reservation missing from the review list still has a recovery target after reload', () => {
  const s = storage();
  const original = {
    id: 44,
    enterprise_id: 11,
    enterprise_name: 'Example',
    quota: 299,
    request_id: 'fixture',
  };
  const store = createEnterpriseOperationStore(
    s,
    '1:reservation:44',
    () => 'fixture-operation-44',
  );
  store.begin(
    '/api/enterprise/admin/reservations/44/resolve',
    { settle: false, amount: 0, reason: 'reviewed' },
    { reservation: original },
  );
  // A new balance/name must not change the original recovery context.
  store.begin(
    '/api/enterprise/admin/reservations/44/resolve',
    { settle: true, amount: 9 },
    { reservation: { ...original, quota: 9 } },
  );
  assert.deepEqual(listPendingEnterpriseReservations(s, 1), [original]);
  assert.deepEqual(listPendingEnterpriseReservations(s, 2), []);
  store.clear('fixture-operation-44');
  assert.deepEqual(listPendingEnterpriseReservations(s, 1), []);
});

test('reservation recovery rejects mismatched targets and URLs without hiding valid drafts', () => {
  const s = storage();
  const valid = {
    key: 'fixture-operation-44',
    url: '/api/enterprise/admin/reservations/44/resolve',
    data: { settle: true, amount: 1 },
    context: { reservation: { id: 44, enterprise_id: 11, quota: 1 } },
  };
  const prefix = 'enterprise-operation:v1:1:reservation:';
  s.setItem(prefix + '44', JSON.stringify(valid));
  s.setItem(prefix + '45', '{broken');
  s.setItem(
    prefix + '46',
    JSON.stringify({
      ...valid,
      context: { reservation: { id: 46, enterprise_id: 11, quota: 1 } },
    }),
  );
  s.setItem(
    prefix + '47',
    JSON.stringify({
      ...valid,
      url: 'https://example.invalid/resolve',
      context: { reservation: { id: 47, enterprise_id: 11, quota: 1 } },
    }),
  );
  s.setItem(
    prefix + '48',
    JSON.stringify({
      ...valid,
      url: '/api/enterprise/admin/reservations/48/resolve',
      context: {
        reservation: {
          id: 48,
          enterprise_id: 11,
          quota: Number.MAX_SAFE_INTEGER + 1,
        },
      },
    }),
  );
  s.setItem(
    prefix + '49',
    JSON.stringify({
      ...valid,
      url: '/api/enterprise/admin/reservations/49/resolve',
      context: {
        reservation: {
          id: 49,
          enterprise_id: 11,
          quota: 1,
          enterprise_name: { invalid: 'label' },
        },
      },
    }),
  );
  assert.deepEqual(listPendingEnterpriseReservations(s, 1), [
    valid.context.reservation,
  ]);
  for (const actorID of [null, undefined, 0, '1', -1])
    assert.deepEqual(listPendingEnterpriseReservations(s, actorID), []);
});
