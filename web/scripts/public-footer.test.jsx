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
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom/server';
import PublicFooter from '../src/components/layout/PublicFooter';

const defaultModules = {
  home: true,
  console: true,
  pricing: true,
  docs: true,
  about: true,
  usage: true,
};
const defaultProps = {
  systemName: '元衡测试',
  logo: '/logo.png',
  modules: defaultModules,
  docsLink: '/docs',
  user: null,
  status: { user_agreement_enabled: true, privacy_policy_enabled: true },
  customLanding: false,
  isDownloadPage: false,
  isLegalPage: false,
  isEnterprisePage: false,
  t: (key) => key,
};
const render = (props = {}) =>
  renderToStaticMarkup(
    <StaticRouter location='/'>
      <PublicFooter {...defaultProps} {...props} />
    </StaticRouter>,
  );
const textContent = (html) => html.replace(/<[^>]*>/g, '').trim();
// Read the actual server-rendered links, including React Router's final hrefs.
const linksFrom = (html) =>
  new Map(
    [...html.matchAll(/<a\b([^>]*)>([\s\S]*?)<\/a>/g)].map(
      ([, attributes, body]) => [
        textContent(body),
        attributes.match(/\bhref="([^"]*)"/)?.[1].replaceAll('&amp;', '&'),
      ],
    ),
  );
const links = (props) => linksFrom(render(props));
const loginPath = (path) => `/login?next=${encodeURIComponent(path)}`;
const policyLinks = {
  'Cookie 政策': '/legal/cookies',
  数据隐私: '/legal/data-privacy',
  支付须知: '/legal/payment',
  'ZDR 条款': '/legal/zdr',
  异步生成条款: '/legal/asynchronous-generation',
  你的隐私选择: '/privacy-choices',
};

test('renders four footer columns with real product, developer, support and policy destinations', () => {
  const html = render();
  assert.deepEqual(
    [...html.matchAll(/<h2\b[^>]*>([\s\S]*?)<\/h2>/g)].map(([, body]) =>
      textContent(body),
    ),
    ['产品', '开发者', '关于我们', '条款与政策'],
  );
  assert.deepEqual(Object.fromEntries(linksFrom(html)), {
    元衡测试: '/',
    模型广场: '/pricing',
    智能路由: loginPath('/console/token'),
    模型体验: loginPath('/console/playground'),
    'AI 图像创作': loginPath('/console/image-playground'),
    下载客户端: '/download',
    企业定制: '/enterprise',
    Skills: '/docs/image-generation',
    文档: '/docs',
    'API 参考': '/docs/api-examples',
    接入指南: '/docs/clients',
    'Agent 接入': '/docs/clients/openclaw',
    快速开始: '/#quick-start',
    控制台: loginPath('/console'),
    用量查询: '/usage',
    关于: '/about',
    支持: '/docs/troubleshooting',
    常见问题: '/#faq-title',
    服务条款: '/user-agreement',
    隐私政策: '/privacy-policy',
    ...policyLinks,
  });
  assert.match(html, /aria-label="页脚导航"/);
  assert.match(html, /class="public-footer-privacy-choice"/);
});

test('anonymous protected links preserve their destination while signed-in users go directly there', () => {
  const modules = {
    ...defaultModules,
    pricing: { enabled: true, requireAuth: true },
  };
  const guest = links({ modules });
  const signedIn = links({ modules, user: { id: 1 } });
  for (const [label, path] of Object.entries({
    模型广场: '/pricing',
    智能路由: '/console/token',
    模型体验: '/console/playground',
    'AI 图像创作': '/console/image-playground',
    控制台: '/console',
  })) {
    assert.equal(guest.get(label), loginPath(path), label);
    assert.equal(signedIn.get(label), path, label);
  }
  assert.equal(links().get('模型广场'), '/pricing');
  assert.equal(
    links({
      modules: { ...modules, pricing: { enabled: true, requireAuth: false } },
    }).get('模型广场'),
    '/pricing',
  );
});

test('disabled navigation modules hide their corresponding footer entries', () => {
  const entries = links({
    modules: Object.fromEntries(
      Object.keys(defaultModules).map((key) => [key, false]),
    ),
  });
  assert.deepEqual(Object.fromEntries(entries), {
    元衡测试: '/',
    下载客户端: '/download',
    企业定制: '/enterprise',
    服务条款: '/user-agreement',
    隐私政策: '/privacy-policy',
    ...policyLinks,
  });
  for (const pricing of [false, { enabled: false, requireAuth: true }]) {
    assert.equal(
      links({ modules: { ...defaultModules, pricing } }).has('模型广场'),
      false,
    );
  }
});

test('documentation links require the module and configured docs URL, without rewriting that URL', () => {
  const labels = [
    '文档',
    'Skills',
    'API 参考',
    '接入指南',
    'Agent 接入',
    '支持',
  ];
  for (const props of [
    { docsLink: '' },
    { modules: { ...defaultModules, docs: false } },
  ]) {
    const entries = links(props);
    for (const label of labels) assert.equal(entries.has(label), false, label);
  }
  const customDocs = 'https://docs.example.test/guide?lang=zh&from=footer';
  const html = render({ docsLink: customDocs });
  const entries = linksFrom(html);
  assert.equal(entries.get('文档'), customDocs);
  assert.equal(entries.get('接入指南'), '/docs/clients');
  assert.match(html, /target="_blank" rel="noopener noreferrer"/);
});

test('configured agreement and privacy flags are independent of the new public policy links', () => {
  for (const agreement of [false, true]) {
    for (const privacy of [false, true]) {
      const entries = links({
        status: {
          user_agreement_enabled: agreement,
          privacy_policy_enabled: privacy,
        },
      });
      assert.equal(entries.has('服务条款'), agreement);
      assert.equal(entries.has('隐私政策'), privacy);
      for (const [label, path] of Object.entries(policyLinks)) {
        assert.equal(entries.get(label), path, label);
      }
    }
  }
});

test('a custom homepage keeps the compact copyright footer', () => {
  const html = render({ customLanding: true });
  assert.match(html, /public-footer--compact/);
  assert.match(html, /元衡测试\. 版权所有/);
  assert.doesNotMatch(html, /<nav\b/);
  assert.equal(linksFrom(html).size, 0);
});

test('download, legal and enterprise pages retain the directory even when the homepage is customized', () => {
  for (const page of [
    { isDownloadPage: true },
    { isLegalPage: true },
    { isEnterprisePage: true },
  ]) {
    const html = render({ customLanding: true, ...page });
    const entries = linksFrom(html);
    assert.match(html, /public-footer--directory/);
    assert.equal(entries.get('你的隐私选择'), '/privacy-choices');
    assert.equal(entries.get('下载客户端'), '/download');
    assert.equal(entries.get('企业定制'), '/enterprise');
    assert.equal(entries.has('快速开始'), false);
    assert.equal(entries.has('常见问题'), false);
  }
});

test('home section links follow the home module without hiding other destinations', () => {
  const entries = links({ modules: { ...defaultModules, home: false } });
  assert.equal(entries.has('快速开始'), false);
  assert.equal(entries.has('常见问题'), false);
  assert.equal(entries.get('文档'), '/docs');
  assert.equal(entries.get('支持'), '/docs/troubleshooting');
  assert.equal(entries.get('关于'), '/about');
});
