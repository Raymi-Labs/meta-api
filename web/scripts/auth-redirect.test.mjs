import assert from 'node:assert/strict';
import {
  buildAuthPathWithNext,
  consumeOAuthLoginRedirect,
  getSafeInternalRedirectPath,
  getSafeLoginRedirectPath,
  navigateToLoginRedirect,
  rememberOAuthLoginRedirect,
} from '../src/helpers/authRedirect.js';

class MemoryStorage {
  constructor() {
    this.values = new Map();
  }

  getItem(key) {
    return this.values.get(key) ?? null;
  }

  setItem(key, value) {
    this.values.set(key, value);
  }

  removeItem(key) {
    this.values.delete(key);
  }
}

const forumStart = '/api/forum/sso/start';

assert.equal(
  getSafeLoginRedirectPath('?next=%2Fconsole%2Foverview%3Ftab%3Drewards'),
  '/console/overview?tab=rewards',
  '合法站内路径应保留',
);
assert.equal(
  getSafeLoginRedirectPath('?next=https%3A%2F%2Fevil.test', '/console'),
  '/console',
  '外部 URL 必须拒绝',
);
assert.equal(
  getSafeLoginRedirectPath('?next=%2F%2Fevil.test', '/console'),
  '/console',
  'protocol-relative URL 必须拒绝',
);
assert.equal(
  getSafeInternalRedirectPath('/%255c%255cevil.test', '/console'),
  '/console',
  '重复编码的反斜杠路径必须拒绝',
);
assert.equal(
  getSafeLoginRedirectPath(
    '?next=%2Fconsole&next=%2Fapi%2Fforum%2Fsso%2Fstart',
  ),
  '/console',
  '重复 next 参数必须回退',
);
assert.equal(
  buildAuthPathWithNext('/login', forumStart),
  '/login?next=%2Fapi%2Fforum%2Fsso%2Fstart',
  '注册成功后应完整保留登录回跳参数',
);
assert.equal(
  buildAuthPathWithNext('/register', 'https://evil.test'),
  '/register',
  '注册链接不得携带外部地址',
);

const clientNavigations = [];
const documentNavigations = [];
assert.equal(
  navigateToLoginRedirect(forumStart, (path) => clientNavigations.push(path), {
    assign: (path) => documentNavigations.push(path),
  }),
  'document',
  '论坛 SSO 后端路由必须使用完整浏览器跳转',
);
assert.deepEqual(clientNavigations, []);
assert.deepEqual(documentNavigations, [forumStart]);
assert.equal(
  navigateToLoginRedirect('/console/overview', (path) =>
    clientNavigations.push(path),
  ),
  'client',
);
assert.deepEqual(clientNavigations, ['/console/overview']);

const storage = new MemoryStorage();
assert.equal(
  rememberOAuthLoginRedirect('state-a', forumStart, storage, 1000),
  true,
);
assert.equal(
  consumeOAuthLoginRedirect('state-a', '/console/token', storage, 2000),
  forumStart,
  'OAuth 回跳应绑定 state 并一次性消费',
);
assert.equal(
  consumeOAuthLoginRedirect('state-a', '/console/token', storage, 2000),
  '/console/token',
);
assert.equal(
  rememberOAuthLoginRedirect('state-b', forumStart, storage, 1000),
  true,
);
assert.equal(
  consumeOAuthLoginRedirect(
    'state-b',
    '/console/token',
    storage,
    16 * 60 * 1000,
  ),
  '/console/token',
  '过期 OAuth 回跳不得继续使用',
);

console.log('auth redirect tests passed');
