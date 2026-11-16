import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.model-discovery-qa-'));
let server, browser;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.jsx'),
    `import React from 'react';
import {createRoot} from 'react-dom/client';
import i18n from 'i18next';
import {API} from '/src/helpers';
import EditChannelModal from '/src/components/table/channels/modals/EditChannelModal';
import '/src/index.css';
i18n.init({lng:'zh',resources:{zh:{translation:{}}}});
localStorage.setItem('user',JSON.stringify({id:1,role:100}));
window.calls=[];window.mode='upstream';
API.get=async url=>({data:{success:true,data:url==='/api/channel/901'?{id:901,type:24,name:'Fixture',base_url:'https://saved.example/api',models:'existing-model',group:'default',status:1,setting:'{}',settings:'{}',model_mapping:'{}',param_override:'',header_override:''}:url==='/api/group/'?['default']:url.includes('quota_usage')?{}:[]}});
API.post=async (url,body)=>{window.calls.push({url,body});if(window.mode==='pending')return new Promise(resolve=>window.release=()=>resolve({data:{success:true,data:['stale-model'],source:'upstream'}}));return {data:{success:true,data:window.mode==='upstream'?['gemini-4-carbon','gemini-4-hydrogen']:['official-only-model'],source:window.mode==='upstream'?'upstream':'official_reference'}}};
API.put=async ()=>{throw Error('Preview must not save');};
createRoot(document.getElementById('root')).render(<EditChannelModal visible editingChannel={new URLSearchParams(location.search).has('new')?{}:{id:901}} handleClose={()=>{}} refresh={()=>{}}/>);`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    server: { host: '127.0.0.1', port: 0, open: false, proxy: {} },
    plugins: config.plugins
      .flat(Infinity)
      .filter((p) => p?.name !== '@code-inspector/vite'),
  });
  await server.listen();
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 1000 });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(
    `http://127.0.0.1:${server.httpServer.address().port}/${path.basename(fixture)}/`,
    { waitUntil: 'networkidle0' },
  );
  await page.waitForSelector('input[field="base_url"],input[id="base_url"]', {
    timeout: 30000,
  });
  const selector = 'input[field="base_url"],input[id="base_url"]';
  async function change(value) {
    await page.$eval(
      selector,
      (el, value) => {
        Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          'value',
        ).set.call(el, value);
        el.dispatchEvent(new Event('input', { bubbles: true }));
      },
      value,
    );
  }
  async function click(text) {
    await page.evaluate((text) => {
      const button = [...document.querySelectorAll('button')].find(
        (b) => b.textContent.trim() === text,
      );
      if (!button) throw Error('Missing button ' + text);
      button.click();
    }, text);
  }
  await change('https://saved.example/draft');
  await click('获取模型列表');
  await page.waitForFunction(() =>
    document.body.innerText.includes('模型列表来自当前填写的上游地址。'),
  );
  const call = await page.evaluate(() => window.calls.at(-1));
  assert.equal(call.body.base_url, 'https://saved.example/draft');
  assert.equal(call.body.channel_id, 901);
  assert.equal(call.body.key, '');
  assert.equal(call.url, '/api/channel/fetch_models');
  await click('取消');
  await page.evaluate(() => (window.mode = 'official'));
  await click('获取模型列表');
  await page.waitForFunction(() =>
    document.body.innerText.includes(
      '上游列表获取失败，已显示官方参考模型；不代表该渠道支持。',
    ),
  );
  await click('取消');
  await page.evaluate(() => (window.mode = 'pending'));
  await click('获取模型列表');
  await page.waitForFunction(() => typeof window.release === 'function');
  await change('https://saved.example/changed');
  await page.evaluate(() => window.release());
  await new Promise((r) => setTimeout(r, 100));
  assert.equal(
    await page.evaluate(() => document.body.innerText.includes('stale-model')),
    false,
  );
  await page.goto(
    `http://127.0.0.1:${server.httpServer.address().port}/${path.basename(fixture)}/?new=1`,
    { waitUntil: 'networkidle0' },
  );
  await page.waitForSelector(selector);
  await change('https://new.example/api');
  await click('获取模型列表');
  await page.waitForFunction(() => window.calls.length > 0);
  const created = await page.evaluate(() => window.calls.at(-1));
  assert.equal(created.body.base_url, 'https://new.example/api');
  assert.equal('channel_id' in created.body, false);
  assert.deepEqual(errors, []);
  console.log(
    'PASS: actual edit form draft URL, saved-key omission, source notice, fallback and stale response isolation',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
