import { test } from 'bun:test';
import assert from 'node:assert/strict';
import {
  walletEstimate,
  validWalletSummary,
} from '../src/helpers/walletSummary.js';
const base = {
  user_id: 1,
  balance: 12555,
  as_of: 100000,
  usage: {
    status: 'ok',
    net_quota: 10853,
    start_timestamp: 13600,
    end_timestamp: 100000,
  },
};
test('wallet forecast uses only positive net usage and never rounds a low state into healthy', () => {
  assert(validWalletSummary(base, 1));
  assert.equal(walletEstimate(base).kind, 'low');
  for (const [balance, net, kind] of [
    [10, 20, 'urgent'],
    [30, 10, 'normal'],
    [0, 20, 'empty'],
    [-1, 20, 'empty'],
    [10, 0, 'idle'],
    [10, -20, 'idle'],
  ])
    assert.equal(
      walletEstimate({
        ...base,
        balance,
        usage: { ...base.usage, net_quota: net },
      }).kind,
      kind,
    );
  for (const status of ['unavailable', 'not_recorded'])
    assert.equal(
      walletEstimate({ ...base, usage: { status, net_quota: null } }).kind,
      'unknown',
    );
});
test('identity, window and incomplete totals are validated before display', () => {
  assert(!validWalletSummary(base, 2));
  for (const change of [
    { balance: null },
    { balance: Infinity },
    { as_of: 0 },
    { usage: { ...base.usage, start_timestamp: 0 } },
    { usage: { ...base.usage, status: 'ambiguous' } },
    { usage: { ...base.usage, net_quota: null } },
  ])
    assert(!validWalletSummary({ ...base, ...change }, 1));
  for (const status of ['unavailable', 'not_recorded'])
    assert(
      validWalletSummary(
        { ...base, usage: { ...base.usage, status, net_quota: null } },
        1,
      ),
    );
});
