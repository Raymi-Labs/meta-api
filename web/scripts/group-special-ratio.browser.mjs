// Exercise the real editor with synthetic rules; no production API is contacted.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import react from '@vitejs/plugin-react';
import { stop } from 'esbuild';
import puppeteer from 'puppeteer-core';
const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.group-ratio-qa-'));
let server, browser;
const errors = [];
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.jsx'),
    `
import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import i18n from 'i18next';
import {initReactI18next} from 'react-i18next';
import Rules,{serializeGroupGroupRatio} from '../src/pages/Setting/Ratio/components/GroupGroupRatioRules';
import '@douyinfe/semi-ui/dist/css/semi.min.css';
await i18n.use(initReactI18next).init({lng:'zh',resources:{zh:{translation:{}}},fallbackLng:false});
window.serializeRules=serializeGroupGroupRatio;
function Fixture(){const [value,setValue]=useState('{"vip":{"paid":0.0001}}');return <><Rules value={value} groupNames={['vip','paid']} onChange={setValue}/><pre id="value">{value}</pre><button id="save" onClick={()=>window.saved={key:'GroupGroupRatio',value}}>Save</button></>}
createRoot(document.getElementById('root')).render(<Fixture/>);`,
  );
  server = await createServer({
    configFile: false,
    root,
    plugins: [react()],
    resolve: { alias: { '@': path.join(root, 'src') } },
    optimizeDeps: { entries: [path.join(fixture, 'index.html')] },
    server: { host: '127.0.0.1', port: 0 },
  });
  await server.listen();
  const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setRequestInterception(true);
  page.on('request', (r) =>
    new URL(r.url()).origin === origin ? r.continue() : r.abort(),
  );
  await page.goto(`${origin}/${path.basename(fixture)}/index.html`, {
    waitUntil: 'networkidle0',
  });
  await page.waitForSelector('.semi-icon-delete');
  // Remove the only group, confirm the actual Semi popover, then save its emitted value.
  await page.click('.semi-icon-delete');
  await page.waitForSelector('.semi-popconfirm-popover');
  await page.evaluate(() =>
    document
      .querySelector('.semi-popconfirm-popover .semi-button-primary')
      .click(),
  );
  await page.waitForFunction(
    () => document.querySelector('#value').textContent === '{}',
  );
  await page.click('#save');
  assert.deepEqual(await page.evaluate(() => window.saved), {
    key: 'GroupGroupRatio',
    value: '{}',
  });
  assert(
    await page.evaluate(() => document.body.textContent.includes('暂无规则')),
  );
  assert.equal(await page.evaluate(() => window.serializeRules([])), '{}');
  assert.deepEqual(
    JSON.parse(
      await page.evaluate(() =>
        window.serializeRules([
          { userGroup: 'vip', usingGroup: 'paid', ratio: 0.0001 },
          { userGroup: 'vip', usingGroup: 'free', ratio: 0 },
          { userGroup: 'vip', usingGroup: '', ratio: 1 },
        ]),
      ),
    ),
    { vip: { paid: 0.0001, free: 0 } },
  );
  assert.deepEqual(errors, []);
  console.log(
    'Group special ratio editor passed: delete last rule, save {}, zero/decimal preservation, incomplete rows.',
  );
} finally {
  await browser?.close();
  await server?.close();
  stop();
  await fs.rm(fixture, { recursive: true, force: true });
}
