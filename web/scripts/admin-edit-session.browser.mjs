// Real drawers and list hooks, synthetic APIs only; never connects to production.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || 'playwright'
);
const root = fileURLToPath(new URL('..', import.meta.url));
const prefix = `.admin-edit-session-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${prefix}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'admin-edit-session-qa-'),
);
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
let server, browser;
try {
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${prefix}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter} from 'react-router-dom';
import {LocaleProvider,Toast} from '@douyinfe/semi-ui';import zh from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';
import i18n from 'i18next';import {initReactI18next} from 'react-i18next';
import EditModelModal from './src/components/table/models/modals/EditModelModal';
import EditRedemptionModal from './src/components/table/redemptions/modals/EditRedemptionModal';
import {useModelsData} from './src/hooks/models/useModelsData';
import {useRedemptionsData} from './src/hooks/redemptions/useRedemptionsData';
import './src/index.css';
i18n.use(initReactI18next).init({lng:'zh',resources:{zh:{translation:{}}},interpolation:{escapeValue:false}});
const toastOriginal=Symbol.for('admin-edit-session.toast-error');Toast[toastOriginal] ||= Toast.error;Toast.error=(...args)=>{window.errorToasts.push(args[0]);return Toast[toastOriginal](...args)};
function App(){const kind=new URLSearchParams(location.search).get('kind');const models=useModelsData(),redemptions=useRedemptionsData();
const data=kind==='models'?models:redemptions;const edit=kind==='models'?data.editingModel:data.editingRedemption;const setEdit=kind==='models'?data.setEditingModel:data.setEditingRedemption;
const open=(record)=>{setEdit(record);data.setShowEdit(true)};const refresh=()=>{window.refreshCount=(window.refreshCount||0)+1;data.refresh()};
return <><header style={{padding:20,display:'flex',gap:12}}><button id="open-a" onClick={()=>open({id:1})}>编辑 A</button><button id="open-b" onClick={()=>open({id:2})}>编辑 B</button><button id="open-new" onClick={()=>open({})}>新建</button><button id="filter-vendor" onClick={()=>models.setActiveVendorKey('11')}>厂商 A</button><button id="page-three" onClick={()=>data.handlePageChange(3)}>第三页</button>{kind==='models'&&<button id="open-prefill" onClick={()=>open({model_name:'prefilled-model'})}>配置缺失模型</button>}</header><output id="state" data-open={data.showEdit} data-id={edit.id??'new'} data-page={data.activePage} data-vendor={models.activeVendorKey}/>
{kind==='models'?<EditModelModal visiable={data.showEdit} editingModel={edit} handleClose={data.closeEdit} refresh={refresh}/>:<EditRedemptionModal visiable={data.showEdit} editingRedemption={edit} handleClose={data.closeEdit} refresh={refresh}/>}</>};
createRoot(document.getElementById('root')).render(<BrowserRouter><LocaleProvider locale={zh}><App/></LocaleProvider></BrowserRouter>);`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    plugins: config.plugins
      .flat(Infinity)
      .filter((plugin) => !String(plugin?.name).includes('inspector')),
    server: { host: '127.0.0.1', port: 0, proxy: {} },
  });
  await server.listen();
  const baseURL = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome',
  });
  const checked = [];
  for (const kind of ['models', 'redemption']) {
    const context = await browser.newContext({
      viewport: { width: 1280, height: 1000 },
    });
    const page = await context.newPage();
    page.setDefaultTimeout(8000);
    const errors = [];
    const listRequests = [];
    await context.addInitScript(() => {
      localStorage.setItem('quota_per_unit', '500000');
      if (location.pathname !== '/login') {
        localStorage.setItem('user', JSON.stringify({ id: 1, role: 100 }));
      }
      window.errorToasts = [];
    });
    const writes = [];
    const counts = new Map();
    const pending = [];
    const holds = new Set();
    let failLoad = '',
      failSave = '',
      holdSave = false,
      releaseSave;
    const records =
      kind === 'models'
        ? {
            1: {
              id: 1,
              model_name: 'model-A',
              description: 'description-A',
              display_name: 'Display A',
              context_length: 256000,
              input_modalities: ['text', 'image'],
              output_modalities: ['text'],
              icon: 'Claude.Color',
              tags: 'alpha,beta',
              vendor_id: 11,
              endpoints:
                '{"openai":{"path":"/v1/chat/completions","method":"POST"}}',
              name_rule: 1,
              status: 1,
              sync_official: 0,
            },
            2: {
              id: 2,
              model_name: 'model-B',
              description: 'description-B',
              icon: '',
              tags: '',
              vendor_id: 0,
              endpoints: '',
              name_rule: 0,
              status: 1,
              sync_official: 1,
            },
          }
        : {
            1: {
              id: 1,
              name: 'code-A',
              benefit_type: 'subscription',
              quota: 0,
              plan_id: 7,
              subscription_purchase_mode: 'renew_extend',
              subscription_purchase_quantity: 3,
              expired_time: 1900000000,
            },
            2: {
              id: 2,
              name: 'code-B',
              benefit_type: 'quota',
              quota: 123456,
              plan_id: 0,
              subscription_purchase_mode: 'stack',
              subscription_purchase_quantity: 1,
              expired_time: 0,
            },
          };
    page.on('pageerror', (error) => errors.push(error.message));
    await page.route('**/*', async (route) => {
      const request = route.request(),
        url = new URL(request.url());
      if (url.origin !== baseURL) return route.abort();
      if (url.pathname === '/login')
        return route.fulfill({
          contentType: 'text/html',
          body: '<h1>Login fixture</h1>',
        });
      if (!url.pathname.startsWith('/api/')) return route.continue();
      const reply = (body, status = 200) =>
        route.fulfill({
          status,
          contentType: 'application/json',
          body: JSON.stringify(body),
        });
      const detail = url.pathname.match(new RegExp(`^/api/${kind}/(\\d+)$`));
      if (detail) {
        const id = Number(detail[1]);
        counts.set(id, (counts.get(id) || 0) + 1);
        const body = {
          success: !failLoad,
          message: 'fixture load failure',
          data: structuredClone(records[id]),
        };
        const status =
          failLoad === 'http' ? 500 : failLoad === 'unauthorized' ? 401 : 200;
        if (holds.has(id)) {
          pending.push({ id, reply: () => reply(body, status) });
          return;
        }
        return reply(body, status);
      }
      if (
        url.pathname === `/api/${kind}/` &&
        ['POST', 'PUT'].includes(request.method())
      ) {
        writes.push({ method: request.method(), body: request.postDataJSON() });
        const body = {
          success: !failSave,
          message: 'fixture save failure',
          data:
            kind === 'redemption' && request.method() === 'POST'
              ? ['synthetic-code-only']
              : {},
        };
        const status = failSave === 'http' ? 500 : 200;
        if (holdSave) {
          releaseSave = () => reply(body, status);
          return;
        }
        return reply(body, status);
      }
      if (url.pathname === '/api/vendors/')
        return reply({
          success: true,
          data: { items: [{ id: 11, name: 'Vendor A' }] },
        });
      if (url.pathname === '/api/subscription/admin/plans')
        return reply({
          success: true,
          data: [{ plan: { id: 7, title: 'Plan Seven' } }],
        });
      if (
        url.pathname === '/api/models/' ||
        url.pathname === '/api/models/search' ||
        url.pathname === '/api/redemption/'
      ) {
        listRequests.push(url);
        return reply({
          success: true,
          data: {
            items: [],
            total: 100,
            page: Number(url.searchParams.get('p')) || 1,
          },
        });
      }
      return reply({ success: true, data: [] });
    });
    await page.goto(`${baseURL}/${prefix}.html?kind=${kind}`);
    const name = () =>
      page.locator(kind === 'models' ? 'input#model_name' : 'input#name');
    const submit = () =>
      page.locator('.semi-sidesheet-footer button').filter({ hasText: '提交' });
    const cancel = () =>
      page.locator('.semi-sidesheet-footer button').filter({ hasText: '取消' });
    const open = async (id) => page.locator(`#open-${id}`).click();
    const waitValue = async (value) => {
      await page.waitForFunction(
        ({ selector, value }) =>
          document.querySelector(selector)?.value === value,
        {
          selector: kind === 'models' ? 'input#model_name' : 'input#name',
          value,
        },
      );
    };
    const waitReady = async () =>
      page.waitForFunction(() =>
        [...document.querySelectorAll('button')].some(
          (b) => b.textContent.trim() === '提交' && !b.disabled,
        ),
      );
    const waitClosed = async () =>
      page.waitForFunction(
        () => document.querySelector('#state')?.dataset.open === 'false',
      );
    const waitCount = async (id, count) => {
      for (let i = 0; i < 100 && (counts.get(id) || 0) < count; i++)
        await delay(20);
      assert.equal(counts.get(id), count);
    };
    const release = async (id) => {
      const index = pending.findIndex((item) => item.id === id);
      assert.notEqual(index, -1);
      await pending.splice(index, 1)[0].reply();
    };
    const closeAndOpen = async (target) => {
      await cancel().click();
      // Reopen while the previous 500ms close cleanup would still be pending.
      await page
        .locator(`#open-${target}`)
        .evaluate((button) => button.click());
    };
    const a = kind === 'models' ? 'model-A' : 'code-A';
    const b = kind === 'models' ? 'model-B' : 'code-B';

    // Older A detail arrives after closing A and opening B. The old timer must not clear B.
    holds.add(1);
    await open('a');
    await waitCount(1, 1);
    assert.equal(await submit().isDisabled(), true);
    await closeAndOpen('b');
    await waitValue(b);
    await waitReady();
    await release(1);
    holds.delete(1);
    await delay(650);
    assert.equal(await name().inputValue(), b);
    assert.equal(await page.locator('#state').getAttribute('data-id'), '2');
    await submit().click();
    await waitClosed();
    assert.equal(writes.at(-1).body.id, 2);
    assert.equal(
      writes.at(-1).body[kind === 'models' ? 'model_name' : 'name'],
      b,
    );
    checked.push(
      `${kind}: delayed A / A-close-B-open / 500ms cleanup / save B`,
    );

    // Reopening the same id must issue a fresh GET, not share an old in-flight response.
    holds.add(1);
    const previousCount = counts.get(1);
    await open('a');
    await waitCount(1, previousCount + 1);
    await closeAndOpen('a');
    await waitCount(1, previousCount + 2);
    const latest = pending.pop();
    await latest.reply();
    await waitValue(a);
    await name().fill('current-user-input');
    await release(1);
    holds.delete(1);
    await delay(100);
    assert.equal(await name().inputValue(), 'current-user-input');
    await cancel().click();
    await waitClosed();
    checked.push(
      `${kind}: same-id independent GET / stale response cannot erase input`,
    );

    // A retired HTTP failure is silent; active failures still get one error toast.
    holds.add(1);
    failLoad = 'http';
    const oldCount = counts.get(1);
    await open('a');
    await waitCount(1, oldCount + 1);
    failLoad = '';
    await closeAndOpen('b');
    await waitValue(b);
    const oldToastCount = await page.evaluate(() => window.errorToasts.length);
    await release(1);
    holds.delete(1);
    await delay(150);
    assert.equal(
      await page.evaluate(() => window.errorToasts.length),
      oldToastCount,
    );
    await cancel().click();
    await waitClosed();
    checked.push(`${kind}: retired HTTP detail failure is silent`);

    // Both HTTP and logical failures keep submit disabled and do not reuse A's values.
    for (const mode of ['business', 'http']) {
      await open('a');
      await waitValue(a);
      const toastCount = await page.evaluate(() => window.errorToasts.length);
      failLoad = mode;
      const count = counts.get(2) || 0;
      await closeAndOpen('b');
      await waitCount(2, count + 1);
      await delay(200);
      assert.equal(
        await page.evaluate(() => window.errorToasts.length),
        toastCount + 1,
      );
      assert.equal(await name().inputValue(), '');
      assert.equal(await submit().isDisabled(), true);
      const before = writes.length;
      await name().fill('cannot-save-failed-load');
      await name().press('Enter');
      await delay(100);
      assert.equal(writes.length, before);
      await cancel().click();
      await waitClosed();
      failLoad = '';
    }
    checked.push(
      `${kind}: HTTP/business load failure blocks button and Enter submission`,
    );

    // Original special fields survive editing; failed saves retain all input and may be retried.
    await open('a');
    await waitValue(a);
    await waitReady();
    await name().fill('edited-A');
    for (const mode of ['business', 'http']) {
      failSave = mode;
      const before = writes.length;
      await submit().click();
      await page.waitForFunction(
        () =>
          ![...document.querySelectorAll('button')]
            .find((b) => b.textContent.trim() === '提交')
            ?.classList.contains('semi-button-loading'),
      );
      for (let i = 0; i < 100 && writes.length === before; i++) await delay(20);
      assert.equal(writes.length, before + 1);
      await delay(100);
      assert.equal(await name().inputValue(), 'edited-A');
      assert.equal(
        await page.locator('#state').getAttribute('data-open'),
        'true',
      );
    }
    failSave = '';
    holdSave = true;
    const before = writes.length;
    await submit().evaluate((button) => {
      button.click();
      button.click();
    });
    for (let i = 0; i < 100 && !releaseSave; i++) await delay(20);
    assert.ok(releaseSave);
    assert.equal(writes.length, before + 1);
    const payload = writes.at(-1).body;
    if (kind === 'models') {
      assert.equal(payload.display_name, 'Display A');
      assert.equal(payload.context_length, 256000);
      assert.deepEqual(payload.input_modalities, ['text', 'image']);
      assert.deepEqual(payload.output_modalities, ['text']);
      assert.equal(payload.vendor_id, 11);
      assert.equal(payload.tags, 'alpha,beta');
      assert.equal(payload.sync_official, 0);
      assert.equal(payload.endpoints, records[1].endpoints);
    } else {
      assert.equal(payload.benefit_type, 'subscription');
      assert.equal(payload.plan_id, 7);
      assert.equal(payload.subscription_purchase_mode, 'renew');
      assert.equal(payload.subscription_purchase_quantity, 3);
      assert.equal(payload.quota, 0);
      assert.equal(payload.expired_time, 1900000000);
    }
    await cancel().click();
    await waitClosed();
    if (kind === 'models') {
      await page.locator('#filter-vendor').evaluate((button) => button.click());
      await page.waitForFunction(
        () => document.querySelector('#state').dataset.vendor === '11',
      );
      await delay(100);
    }
    await page.locator('#page-three').evaluate((button) => button.click());
    await page.waitForFunction(
      () => document.querySelector('#state').dataset.page === '3',
    );
    await page.locator('#open-b').evaluate((button) => button.click());
    await waitValue(b);
    await releaseSave();
    releaseSave = undefined;
    holdSave = false;
    await delay(150);
    assert.equal(await name().inputValue(), b);
    const refreshed = listRequests
      .filter((url) => url.pathname.startsWith(`/api/${kind}/`))
      .at(-1);
    assert.equal(refreshed.searchParams.get('p'), '3');
    if (kind === 'models')
      assert.equal(refreshed.searchParams.get('vendor'), '11');
    assert.equal(await page.locator('#state').getAttribute('data-page'), '3');
    assert.equal(
      await page.locator('#state').getAttribute('data-open'),
      'true',
    );
    await cancel().click();
    await waitClosed();
    checked.push(
      `${kind}: failed save retains input / special fields / duplicate submit / stale save leaves B open`,
    );
    checked.push(
      `${kind}: late save refresh uses current pagination/vendor filter`,
    );

    await open('a');
    await waitValue(a);
    await waitReady();
    failSave = 'http';
    holdSave = true;
    await submit().click();
    for (let i = 0; i < 100 && !releaseSave; i++) await delay(20);
    assert.ok(releaseSave);
    await closeAndOpen('b');
    await waitValue(b);
    const saveToastCount = await page.evaluate(() => window.errorToasts.length);
    await releaseSave();
    releaseSave = undefined;
    holdSave = false;
    failSave = '';
    await delay(150);
    assert.equal(
      await page.evaluate(() => window.errorToasts.length),
      saveToastCount,
    );
    assert.equal(await name().inputValue(), b);
    await cancel().click();
    await waitClosed();
    checked.push(`${kind}: retired HTTP save failure is silent`);

    // New -> edit -> new resets every field; a late edit response cannot replace a new form.
    holds.add(1);
    const count = counts.get(1);
    await open('a');
    await waitCount(1, count + 1);
    await closeAndOpen('new');
    await waitReady();
    await release(1);
    holds.delete(1);
    await delay(100);
    assert.equal(await name().inputValue(), '');
    await name().fill('new-record');
    if (kind === 'models') {
      await page.locator('#name_rule').click();
      await page.getByText('精确名称匹配', { exact: true }).last().click();
      await delay(100);
      assert.equal(await page.locator('textarea#description').inputValue(), '');
    } else {
      assert.equal(
        await page
          .locator('[x-field-id="expired_time"] input')
          .first()
          .inputValue(),
        '',
      );
    }
    await submit().click();
    await waitClosed();
    assert.equal(writes.at(-1).method, 'POST');
    assert.equal(writes.at(-1).body.id, undefined);
    if (kind === 'models') {
      assert.equal(writes.at(-1).body.display_name, '');
      assert.equal(writes.at(-1).body.context_length, 0);
      assert.deepEqual(writes.at(-1).body.input_modalities, []);
      assert.deepEqual(writes.at(-1).body.output_modalities, []);
      assert.equal(writes.at(-1).body.vendor_id, 0);
      assert.equal(writes.at(-1).body.tags, '');
      assert.equal(writes.at(-1).body.sync_official, 1);
      await open('prefill');
      await waitValue('prefilled-model');
      await waitReady();
      await submit().click();
      await waitClosed();
      assert.equal(writes.at(-1).body.name_rule, 0);
    } else {
      assert.equal(writes.at(-1).body.benefit_type, 'quota');
      assert.equal(writes.at(-1).body.plan_id, 0);
      assert.equal(writes.at(-1).body.expired_time, 0);
      assert.equal(writes.at(-1).body.subscription_purchase_quantity, 1);
      await page
        .locator('.semi-modal-footer button')
        .filter({ hasText: '取消' })
        .click();
    }
    checked.push(
      `${kind}: new/edit reset / late edit ignored by new / POST contract`,
    );

    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate(() => {
      document.body.setAttribute('theme-mode', 'dark');
      document.documentElement.classList.add('dark');
    });
    await open('b');
    await waitValue(b);
    await waitReady();
    await page.waitForFunction(() => {
      const bounds = document
        .querySelector('.semi-sidesheet-inner')
        ?.getBoundingClientRect();
      return (
        bounds &&
        bounds.left >= -1 &&
        bounds.right <= innerWidth + 1 &&
        bounds.width >= innerWidth - 1
      );
    });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      true,
    );
    await page.screenshot({
      path: path.join(artifacts, `${kind}-mobile-dark.png`),
      fullPage: true,
    });
    await submit().click();
    await waitClosed();
    assert.equal(writes.at(-1).body.id, 2);
    checked.push(`${kind}: mobile dark display and edit save`);
    assert.deepEqual(errors, []);
    failLoad = 'unauthorized';
    await open('a');
    await page.waitForURL('**/login?expired=true');
    assert.equal(await page.evaluate(() => localStorage.getItem('user')), null);
    checked.push(`${kind}: current HTTP 401 retains login-expiry handling`);
    await context.close();
  }
  console.log(
    JSON.stringify(
      { passed: checked.length, scenarios: checked, artifacts },
      null,
      2,
    ),
  );
} catch (error) {
  for (const context of browser?.contexts() || []) {
    for (const page of context.pages()) {
      console.error(await page.locator('body').innerText());
      console.error(await page.evaluate(() => window.errorToasts));
      await page.screenshot({
        path: path.join(artifacts, 'failure.png'),
        fullPage: true,
      });
    }
  }
  console.error(`Artifacts: ${artifacts}`);
  throw error;
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  for (const file of files) fs.rmSync(file, { force: true });
}
