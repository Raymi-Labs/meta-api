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
import test, { before } from 'node:test';
import { mock } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom/server';
import { useNavigation } from '../src/hooks/common/useNavigation';
import {
  DEFAULT_HEADER_NAV_MODULES,
  getTopNavigationLinks,
  getTopNavigationTarget,
} from '../src/helpers/topNavigation';
import { getPublicShellPage } from '../src/helpers/publicRoutes';

// Semi's package entry starts browser-only animation code on import. Stub only
// its presentation primitives; the actual links, hook and logo render below.
const Presentation = ({ children }) => <>{children}</>;
mock.module('@douyinfe/semi-ui', () => ({
  Skeleton: Presentation,
  Typography: { Title: Presentation },
  Tag: Presentation,
}));
let Navigation;
let HeaderLogo;
before(async () => {
  ({ default: Navigation } = await import(
    '../src/components/layout/headerbar/Navigation'
  ));
  ({ default: HeaderLogo } = await import(
    '../src/components/layout/headerbar/HeaderLogo'
  ));
});

const t = (key) => key;
const login = (path) => `/login?next=${encodeURIComponent(path)}`;
const labels = ['模型广场', '模型体验', '企业定制', '文档', '控制台'];
const readLinks = (html) =>
  [...html.matchAll(/<a\b([^>]*)>([\s\S]*?)<\/a>/g)].map(
    ([, attributes, body]) => ({
      text: body.replace(/<[^>]*>/g, '').trim(),
      href: attributes.match(/\bhref="([^"]*)"/)?.[1].replaceAll('&amp;', '&'),
      target: attributes.match(/\btarget="([^"]*)"/)?.[1],
    }),
  );
const NavigationFixture = ({ modules, docsLink, user, isMobile }) => {
  const { mainNavLinks } = useNavigation(t, docsLink, modules);
  return (
    <Navigation
      mainNavLinks={mainNavLinks}
      userState={{ user }}
      isMobile={isMobile}
      isLoading={false}
    />
  );
};
const renderNavigation = (props = {}) =>
  readLinks(
    renderToStaticMarkup(
      <StaticRouter location='/pricing'>
        <NavigationFixture docsLink='/docs' user={null} {...props} />
      </StaticRouter>,
    ),
  );

test('desktop and mobile navigation render only the requested five entries in order', () => {
  for (const isMobile of [false, true]) {
    const links = renderNavigation({
      isMobile,
      modules: { ...DEFAULT_HEADER_NAV_MODULES, usage: true },
    });
    assert.deepEqual(
      links.map((link) => link.text),
      labels,
    );
    assert.deepEqual(
      links.map((link) => link.href),
      [
        '/pricing',
        login('/console/playground'),
        '/enterprise',
        '/docs',
        login('/console'),
      ],
    );
  }
});

test('public navigation data and rendered console navigation preserve the same protected destinations', () => {
  const modules = {
    ...DEFAULT_HEADER_NAV_MODULES,
    pricing: { enabled: true, requireAuth: true },
  };
  for (const user of [null, { id: 1 }]) {
    const publicLinks = getTopNavigationLinks(t, '/docs', modules);
    const consoleLinks = renderNavigation({ modules, user });
    assert.deepEqual(
      consoleLinks.map((link) => link.href),
      publicLinks.map((link) => getTopNavigationTarget(link, user)),
    );
    for (const [index, path] of [
      [0, '/pricing'],
      [1, '/console/playground'],
      [4, '/console'],
    ]) {
      assert.equal(consoleLinks[index].href, user ? path : login(path));
    }
    assert.equal(consoleLinks[2].href, '/enterprise');
  }
});

test('pricing flags and console visibility hide only their corresponding entries', () => {
  for (const pricing of [false, null, { enabled: false, requireAuth: true }]) {
    const links = renderNavigation({
      modules: { ...DEFAULT_HEADER_NAV_MODULES, pricing },
    });
    assert.deepEqual(
      links.map((link) => link.text),
      labels.slice(1),
    );
  }
  for (const pricing of [true, { enabled: true, requireAuth: false }]) {
    assert.equal(
      renderNavigation({
        modules: { ...DEFAULT_HEADER_NAV_MODULES, pricing },
      })[0].href,
      '/pricing',
    );
  }
  assert.deepEqual(
    renderNavigation({
      modules: { ...DEFAULT_HEADER_NAV_MODULES, console: false },
    }).map((link) => link.text),
    ['模型广场', '企业定制', '文档'],
  );
  assert.deepEqual(
    renderNavigation({
      modules: {
        home: false,
        about: false,
        usage: false,
        pricing: false,
        console: false,
        docs: false,
      },
    }).map((link) => ({ text: link.text, href: link.href })),
    [{ text: '企业定制', href: '/enterprise' }],
  );
});

test('docs obey the configured URL and module while internal docs stay in the current tab', () => {
  for (const props of [
    { docsLink: '' },
    { modules: { ...DEFAULT_HEADER_NAV_MODULES, docs: false } },
  ]) {
    assert.equal(
      renderNavigation(props).some((link) => link.text === '文档'),
      false,
    );
  }
  const external = 'https://docs.example.test/guide?lang=zh&from=header';
  assert.deepEqual(renderNavigation({ docsLink: external })[3], {
    text: '文档',
    href: external,
    target: '_blank',
  });
  assert.deepEqual(renderNavigation()[3], {
    text: '文档',
    href: '/docs',
    target: undefined,
  });
});

test('the logo remains a home link on mobile console pages after removing the home menu item', () => {
  const html = renderToStaticMarkup(
    <StaticRouter location='/console/playground'>
      <HeaderLogo
        isMobile={true}
        isConsoleRoute={true}
        logo='/logo.png'
        logoLoaded={true}
        isLoading={false}
        systemName='元衡测试'
        t={t}
      />
    </StaticRouter>,
  );
  assert.equal(readLinks(html)[0].href, '/');
  assert.match(html, /aria-label="首页"/);
  assert.match(html, /<img\b/);
});

test('enterprise selects the public shell without intercepting other pages or private routes', () => {
  for (const path of ['/enterprise', '/enterprise/']) {
    assert.equal(getPublicShellPage(path), 'enterprise');
  }
  for (const path of [
    '/enterprise/settings',
    '/enterprise-other',
    '/console/enterprise',
    '/pricing',
    '/docs',
    '/user-agreement',
    '/privacy-policy',
  ]) {
    assert.equal(getPublicShellPage(path), null, path);
  }
  assert.equal(getPublicShellPage('/'), 'home');
  assert.equal(getPublicShellPage('/download/'), 'download');
  assert.equal(getPublicShellPage('/legal/cookies'), 'legal');
  assert.equal(getPublicShellPage('/privacy-choices'), 'legal');
});
