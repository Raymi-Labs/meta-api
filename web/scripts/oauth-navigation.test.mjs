import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import {
  canonicalOAuthEntry,
  loginOrigin,
  META_LOGIN_ORIGIN,
} from '../src/helpers/canonicalLogin.js';
const source = readFileSync(
  new URL('../src/helpers/api.js', import.meta.url),
  'utf8',
);
const body = source
  .slice(
    source.indexOf('async function prepareOAuthState'),
    source.indexOf('let channelModels'),
  )
  .replaceAll('export async function', 'async function');
function setup(origin) {
  const calls = [],
    saved = [];
  let nextState = 0;
  const context = vm.createContext({
    URL,
    console,
    window: {
      location: { origin, assign: (u) => calls.push(['navigate', u]) },
      open: () => {
        throw Error('Unexpected popup');
      },
    },
    canonicalOAuthEntry,
    loginOrigin,
    META_LOGIN_ORIGIN,
    API: {
      get: async (path) => {
        calls.push(['api', path]);
        return {};
      },
    },
    localStorage: { removeItem: () => {} },
    updateAPI: () => {},
    getOAuthState: async () => {
      calls.push(['state']);
      return 'synthetic-' + ++nextState;
    },
    rememberOAuthLoginRedirect: (s, n) => saved.push([s, n]),
    showError: (e) => {
      throw Error(e);
    },
  });
  vm.runInContext(body, context);
  return { context, calls, saved };
}
test('aliases navigate before logout/state and never bind another account', async () => {
  for (const origin of [
    'https://meta-api.vip',
    'https://cn.meta-api.vip',
    'https://nan.meta-api.vip',
  ]) {
    const { context, calls } = setup(origin);
    await context.onGitHubOAuthClicked('id', {
      shouldLogout: true,
      postLoginPath: '/desktop-login?request_id=' + 'a'.repeat(43),
    });
    assert.equal(calls.length, 1);
    assert.equal(calls[0][0], 'navigate');
    assert.match(calls[0][1], /^https:\/\/meta-api.com\/login\?/);
    calls.length = 0;
    await context.onCustomOAuthClicked({ slug: 'google' });
    assert.equal(calls.length, 1);
    assert.equal(calls[0][1], 'https://meta-api.com/console/personal');
  }
});
test('primary GitHub and Google use same-tab canonical callbacks and retain next', async () => {
  const { context, calls, saved } = setup(META_LOGIN_ORIGIN);
  const next = '/desktop-login?request_id=' + 'a'.repeat(43);
  await context.onGitHubOAuthClicked('github-id', {
    shouldLogout: true,
    postLoginPath: next,
  });
  let url = new URL(calls.at(-1)[1]);
  assert.equal(url.origin, 'https://github.com');
  assert.equal(
    url.searchParams.get('redirect_uri'),
    META_LOGIN_ORIGIN + '/oauth/github',
  );
  assert.equal(saved[0][1], next);
  await context.onCustomOAuthClicked(
    {
      slug: 'google',
      client_id: 'google-id',
      authorization_endpoint: 'https://accounts.google.com/o/oauth2/v2/auth',
      scopes: 'openid profile email',
    },
    { shouldLogout: true, postLoginPath: next },
  );
  url = new URL(calls.at(-1)[1]);
  assert.equal(
    url.searchParams.get('redirect_uri'),
    META_LOGIN_ORIGIN + '/oauth/google',
  );
  assert.equal(url.searchParams.get('state'), saved[1][0]);
  assert.equal(saved[1][1], next);
});
