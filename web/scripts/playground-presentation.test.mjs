import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  playgroundDefaultGroup,
  playgroundGroupOptions,
  playgroundMoney,
  compactPrice,
} from '../src/helpers/playgroundPresentation.js';
import { getPlaygroundEndpoint } from '../src/helpers/playgroundCode.js';

test('group labels never show descriptions and welfare never replaces a saved valid selection', () => {
  const groups = [
    { value: 'Claude · 0注入', label: '0注入可蒸' },
    { value: '福利', label: '福利' },
    { value: 'OpenAI · 福利', label: '福利' },
  ];
  assert.deepEqual(
    playgroundGroupOptions(groups).map((g) => g.label),
    groups.map((g) => g.value),
  );
  assert.equal(playgroundDefaultGroup(groups, ''), '福利');
  assert.equal(
    playgroundDefaultGroup(groups, 'Claude · 0注入'),
    'Claude · 0注入',
  );
  assert.equal(playgroundDefaultGroup(groups, 'revoked'), '福利');
  assert.equal(playgroundDefaultGroup([{ value: 'private' }], ''), 'private');
  assert.equal(playgroundDefaultGroup([], ''), '');
});
test('lightning and RMB share raw historical quote, no zeroing tiny prices or double conversion', () => {
  const status = {
    custom_currency_exchange_rate: 1,
    custom_currency_symbol: '⚡',
    price: 0.2,
  };
  assert.deepEqual(playgroundMoney(198, status), {
    primary: '⚡198',
    cny: '≈¥39.6',
  });
  assert.deepEqual(
    playgroundMoney(198, { ...status, custom_currency_exchange_rate: 5 }),
    { primary: '⚡990', cny: '≈¥39.6' },
  );
  assert.deepEqual(playgroundMoney(0, status), { primary: '⚡0', cny: '≈¥0' });
  assert.notEqual(compactPrice(0.000000014), '0');
  assert.equal(playgroundMoney(1, { ...status, price: undefined }).cny, null);
  for (const value of [null, undefined, NaN, Infinity, -1])
    assert.equal(playgroundMoney(value, status), null);
});
test('code uses canonical Meta origin without rewriting other sites or lookalike domains', () => {
  assert.equal(
    getPlaygroundEndpoint('https://meta-api.vip/v1'),
    'https://meta-api.com/v1/chat/completions',
  );
  assert.equal(
    getPlaygroundEndpoint('https://cn.meta-api.vip'),
    'https://meta-api.com/v1/chat/completions',
  );
  assert.equal(
    getPlaygroundEndpoint('https://meta-api.vip.evil.test/v1'),
    'https://meta-api.vip.evil.test/v1/chat/completions',
  );
  assert.equal(
    getPlaygroundEndpoint('http://localhost:1234'),
    'http://localhost:1234/v1/chat/completions',
  );
});
