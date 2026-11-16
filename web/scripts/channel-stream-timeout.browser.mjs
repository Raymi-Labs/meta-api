import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.stream-timeout-qa-'));
const captures = path.resolve(root, '../outputs/channel-stream-timeout-ui');
let server, browser;
try {
  await fs.mkdir(captures, { recursive: true });
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
window.calls=[];
const original={id:901,type:24,name:'Gemini timeout fixture',base_url:'https://upstream.invalid',models:'gemini-4-argon',group:'default',status:1,setting:JSON.stringify({nonstream_response_header_timeout_sec:120,stream_response_header_timeout_sec:180,future_extension:{keep:true}}),settings:'{}',model_mapping:'{}',param_override:'',header_override:''};
window.fixtureChannel=JSON.parse(sessionStorage.getItem('stream-timeout-fixture')||JSON.stringify(original));
if(new URLSearchParams(location.search).has('legacy'))window.fixtureChannel.setting=JSON.stringify({nonstream_response_header_timeout_sec:120,future_extension:{keep:true}});
API.get=async url=>({data:{success:true,data:url==='/api/channel/901'?window.fixtureChannel:url==='/api/group/'?['default']:url.includes('quota_usage')?{}:[]}});
API.post=async ()=>{throw Error('Unexpected POST');};
API.put=async(url,body)=>{window.calls.push({url,body});window.fixtureChannel={...window.fixtureChannel,...body};sessionStorage.setItem('stream-timeout-fixture',JSON.stringify(window.fixtureChannel));return {data:{success:true,data:window.fixtureChannel}};};
createRoot(document.getElementById('root')).render(<EditChannelModal visible editingChannel={{id:901}} handleClose={()=>{}} refresh={()=>{}}/>);
`,
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
  page.setDefaultTimeout(20000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setViewport({ width: 1440, height: 1000 });
  const url = `http://127.0.0.1:${server.httpServer.address().port}/${path.basename(fixture)}/`;
  const field =
    'input[id="stream_response_header_timeout_sec"],input[field="stream_response_header_timeout_sec"]';
  const nonstream =
    'input[id="nonstream_response_header_timeout_sec"],input[field="nonstream_response_header_timeout_sec"]';
  async function button(text) {
    await page.evaluate((text) => {
      const el = [...document.querySelectorAll('button')].find(
        (b) => b.textContent.trim() === text,
      );
      if (!el) throw Error('Missing button ' + text);
      el.click();
    }, text);
  }
  async function open(query = '') {
    await page.goto(url + query, { waitUntil: 'networkidle0' });
    await page.waitForSelector('input[id="name"]');
    if (!(await page.$(field))) await button('展开');
    await page.waitForSelector(field, { visible: true });
    await page.$eval(field, (el) => el.scrollIntoView({ block: 'center' }));
  }
  async function change(value) {
    await page.$eval(
      field,
      (el, value) => {
        Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          'value',
        ).set.call(el, value);
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
      },
      value,
    );
  }
  async function save(expected) {
    const before = await page.evaluate(() => window.calls.length);
    await button('提交');
    await page.waitForFunction((n) => window.calls.length > n, {}, before);
    const call = await page.evaluate(() => window.calls.at(-1));
    assert.equal(call.url, '/api/channel/');
    assert.equal(call.body.group, 'default');
    const setting = JSON.parse(call.body.setting);
    assert.equal(setting.stream_response_header_timeout_sec, expected);
    assert.equal(setting.nonstream_response_header_timeout_sec, 120);
    assert.deepEqual(setting.future_extension, { keep: true });
    assert.equal('stream_response_header_timeout_sec' in call.body, false);
  }
  await open();
  assert.equal(await page.$eval(field, (e) => e.value), '180');
  assert.equal(await page.$eval(nonstream, (e) => e.value), '120');
  await save(180);
  await change('300');
  await save(300);
  await open();
  assert.equal(await page.$eval(field, (e) => e.value), '300');
  await page.screenshot({ path: path.join(captures, 'desktop.png') });
  const help = 'button[aria-label="流式响应头超时（秒）：说明"]';
  await page.focus(help);
  await page.keyboard.press('Enter');
  await page.waitForFunction(() =>
    document.body.innerText.includes('0 继承全局；用于流式 Chat'),
  );
  assert.equal(
    await page.evaluate(() => window.calls.length),
    0,
    'help must not submit',
  );
  await page.click(help);
  await change('');
  await save(0);
  await open();
  assert.equal(await page.$eval(field, (e) => e.value), '0');
  await open('?legacy=1');
  assert.equal(await page.$eval(field, (e) => e.value), '0');
  await save(0);
  await page.setViewport({ width: 390, height: 844 });
  await open();
  await page.$eval(field, (e) => e.scrollIntoView({ block: 'center' }));
  const box = await page.$eval(field, (e) => {
    const r = e.getBoundingClientRect();
    return { left: r.left, right: r.right, width: innerWidth };
  });
  assert.ok(
    box.left >= 0 && box.right <= box.width,
    'mobile field must fit viewport',
  );
  await page.screenshot({ path: path.join(captures, 'mobile.png') });
  assert.deepEqual(errors, []);
  console.log(
    'PASS: actual channel form preserves both waits and unknown settings, saves/reopens 300, clears to 0, inherits legacy settings, keyboard help does not submit, desktop/mobile fit.',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
