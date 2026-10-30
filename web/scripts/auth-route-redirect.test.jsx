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

import assert from 'node:assert/strict';
import { afterEach, beforeAll, test, mock } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import * as router from 'react-router-dom';
import { StaticRouter } from 'react-router-dom/server';

// Keep the real router context/useLocation. Only expose Navigate's destination
// instead of performing its client effect during a server render.
const routerExports = { ...router };
mock.module('react-router-dom', () => ({
  ...routerExports,
  Navigate: ({ to, replace }) => <a href={to} data-replace={String(replace)} />,
}));
mock.module('../src/helpers/history', () => ({ history: { location: {} } }));
let AuthRedirect;
let PrivateRoute;
beforeAll(async () => {
  ({ AuthRedirect, PrivateRoute } = await import('../src/helpers/auth'));
});

const originalWindow = globalThis.window;
const originalStorage = globalThis.localStorage;
afterEach(() => {
  if (originalWindow === undefined) delete globalThis.window;
  else globalThis.window = originalWindow;
  if (originalStorage === undefined) delete globalThis.localStorage;
  else globalThis.localStorage = originalStorage;
});

const renderGuard = (route, browserSearch, loggedIn = true) => {
  globalThis.window = { location: { search: browserSearch } };
  globalThis.localStorage = {
    getItem: (key) =>
      key === 'user' && loggedIn ? JSON.stringify({ id: 1 }) : null,
  };
  return renderToStaticMarkup(
    <StaticRouter location={route}>
      <AuthRedirect>
        <span>Login form</span>
      </AuthRedirect>
    </StaticRouter>,
  );
};

test('login guard retains next while browser URL advances before router commit', () => {
  assert.equal(
    renderGuard('/login?next=%2Fenterprise', ''),
    '<a href="/enterprise" data-replace="true"></a>',
  );
});

test('register guard uses the rendered route rather than another browser query', () => {
  assert.equal(
    renderGuard('/register?next=%2Fconsole%2Fplayground', '?next=%2Fconsole'),
    '<a href="/console/playground" data-replace="true"></a>',
  );
});

test('backend SSO keeps login visible despite a stored user and advanced history', () => {
  assert.equal(
    renderGuard('/login?next=%2Fapi%2Fforum%2Fsso%2Fstart', ''),
    '<span>Login form</span>',
  );
});

test('unsafe, duplicate and missing next still fall back to console', () => {
  for (const route of [
    '/login',
    '/login?next=https%3A%2F%2Fevil.test',
    '/login?next=%2F%2Fevil.test',
    '/login?next=%2Fenterprise&next=%2Fconsole',
  ]) {
    assert.equal(
      renderGuard(route, '?next=%2Fenterprise'),
      '<a href="/console" data-replace="true"></a>',
    );
  }
});

test('anonymous visitors retain the login form', () => {
  assert.equal(
    renderGuard('/login?next=%2Fenterprise', '', false),
    '<span>Login form</span>',
  );
});

test('playground direct entry carries the canonical URL through login and back', () => {
  const target = '/playground?model=test#chat';
  const loginPath = `/login?next=${encodeURIComponent(target)}`;
  globalThis.localStorage = { getItem: () => null };
  assert.equal(
    renderToStaticMarkup(
      <StaticRouter location={target}>
        <PrivateRoute loginPath={loginPath}>
          <span>Playground</span>
        </PrivateRoute>
      </StaticRouter>,
    ),
    `<a href="${loginPath}" data-replace="undefined"></a>`,
  );
  assert.equal(
    renderGuard(loginPath, '', true),
    '<a href="/playground?model=test#chat" data-replace="true"></a>',
  );
  assert.equal(
    renderToStaticMarkup(
      <StaticRouter location={target}>
        <PrivateRoute loginPath={loginPath}>
          <span>Playground</span>
        </PrivateRoute>
      </StaticRouter>,
    ),
    '<span>Playground</span>',
  );
});
