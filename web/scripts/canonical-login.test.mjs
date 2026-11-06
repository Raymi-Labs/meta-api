import test from 'node:test';
import assert from 'node:assert/strict';
import {
  canonicalLoginPage,
  canonicalOAuthEntry,
  loginOrigin,
} from '../src/helpers/canonicalLogin.js';
import {
  rememberOAuthLoginRedirect,
  consumeOAuthLoginRedirect,
} from '../src/helpers/authRedirect.js';
const primary = 'https://meta-api.com';
const aliases = [
  'https://meta-api.vip',
  'https://cn.meta-api.vip',
  'https://nan.meta-api.vip',
];
test('four origins share a login origin, local and unrelated installations do not', () => {
  for (const origin of [primary, ...aliases])
    assert.equal(loginOrigin(origin), primary);
  for (const origin of [
    'https://meta-api.vip.evil.test',
    'http://localhost:3000',
    'https://custom.test',
    'https://meta-api.com:444',
  ])
    assert.equal(loginOrigin(origin), origin);
  assert.equal(canonicalLoginPage(primary, '/login'), '');
});
test('desktop consent next and invitation survive without state, code or tokens', () => {
  const id = 'a'.repeat(43),
    next = '/desktop-login?request_id=' + id;
  for (const origin of aliases) {
    const target = new URL(
      canonicalLoginPage(
        origin,
        '/login/',
        '?next=' +
          encodeURIComponent(next) +
          '&aff=123&code=SECRET&state=SECRET&token=SECRET',
      ),
    );
    assert.equal(target.origin, primary);
    assert.equal(target.pathname, '/login');
    assert.equal(target.searchParams.get('next'), next);
    assert.equal(target.searchParams.get('aff'), '123');
    assert.equal([...target.searchParams].length, 2);
    assert.equal(
      canonicalLoginPage(
        origin,
        '/desktop-login',
        'request_id=' + id + '&token=SECRET',
      ),
      primary + '/desktop-login?request_id=' + id,
    );
  }
});
test('rejects unsafe next, repeated parameters and invalid request ids', () => {
  for (const q of [
    'next=https://evil.test',
    'next=%2F%2Fevil.test',
    'next=%2F%5Cevil.test',
    'next=%2Fconsole&next=%2Fbad',
    'next=%2Fconsole%0Aevil',
    'next=%2Fdesktop-login%3Frequest_id%3Dbad',
    'aff=x&aff=y',
    'aff=%3Cscript%3E',
  ])
    assert.equal(
      canonicalLoginPage(aliases[0], '/login', q),
      primary + '/login',
    );
  assert.equal(
    canonicalLoginPage(aliases[0], '/desktop-login', 'request_id=bad'),
    primary + '/desktop-login',
  );
});
test('never redirects APIs, callback codes, or existing application pages', () => {
  for (const p of [
    '/v1/responses',
    '/api/user/login',
    '/api/desktop-auth/exchange',
    '/oauth/google',
    '/console/personal',
  ])
    assert.equal(canonicalLoginPage(aliases[0], p, '?code=SECRET'), '');
});
test('binding requires a fresh primary-site personal-page confirmation', () => {
  for (const origin of aliases) {
    assert.equal(canonicalOAuthEntry(origin), primary + '/console/personal');
    assert.equal(
      canonicalOAuthEntry(origin, {
        shouldLogout: true,
        postLoginPath: 'https://evil.test',
      }),
      primary + '/login',
    );
  }
  assert.equal(canonicalOAuthEntry(primary), '');
});
test('same-tab callback preserves existing state-scoped desktop return', () => {
  const data = new Map();
  const storage = {
    setItem: (k, v) => data.set(k, v),
    getItem: (k) => data.get(k),
    removeItem: (k) => data.delete(k),
  };
  const next = '/desktop-login?request_id=' + 'a'.repeat(43);
  assert.equal(
    rememberOAuthLoginRedirect('synthetic-state', next, storage, 1000),
    true,
  );
  assert.equal(
    consumeOAuthLoginRedirect('synthetic-state', '/console', storage, 1001),
    next,
  );
  assert.equal(
    consumeOAuthLoginRedirect('synthetic-state', '/console', storage, 1002),
    '/console',
  );
});

test('browser navigation converges once for every login entry, independent of backend Host', () => {
  const requestId = 'a'.repeat(43);
  for (const origin of [primary, ...aliases]) {
    for (const path of [
      '/login?next=%2Fconsole&code=SECRET&state=SECRET&token=SECRET',
      '/register?aff=localtest&code=SECRET',
      '/desktop-login?request_id=' + requestId + '&token=SECRET',
    ]) {
      let visible = new URL(path, origin);
      const target = canonicalLoginPage(visible.origin, visible.pathname, visible.search);
      if (origin === primary) {
        assert.equal(target, '', 'primary page must not replace itself');
      } else {
        assert.ok(target);
        assert.ok(!target.includes('SECRET'));
        visible = new URL(target);
        assert.equal(visible.origin, primary);
        assert.equal(canonicalLoginPage(visible.origin, visible.pathname, visible.search), '');
        if (visible.pathname === '/register') assert.equal(visible.searchParams.get('aff'), 'localtest');
        if (visible.pathname === '/desktop-login') assert.equal(visible.searchParams.get('request_id'), requestId);
      }
    }
  }
});

test('query parameters cannot impersonate the browser origin or opt out of alias navigation', () => {
  const query = '?host=meta-api.com&origin=https%3A%2F%2Fmeta-api.com&redirected=1&next=%2F%2Fevil.test&aff=a&aff=b';
  for (const origin of aliases) {
    assert.equal(canonicalLoginPage(origin, '/login', query), primary + '/login');
  }
  assert.equal(canonicalLoginPage(primary, '/login', '?host=meta-api.vip'), '');
  for (const origin of ['https://meta-api.com.evil.test', 'http://localhost:3000', 'https://custom.test']) {
    assert.equal(canonicalLoginPage(origin, '/login', query), '');
  }
});
