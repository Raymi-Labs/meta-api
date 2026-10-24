// Exercise the real usage-log model cell and Semi popover with synthetic data.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.usage-model-copy-qa-'));
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
import {getLogsColumns} from '/src/components/table/usage-logs/UsageLogsColumnDefs';
import {copy} from '/src/helpers/clipboard';
import '/src/index.css';
i18n.init({lng:'zh',resources:{zh:{translation:{}}}});
window.copied=[];
Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>window.copied.push(text)}});
const params=new URLSearchParams(location.search);
const returned=params.get('returned') || 'returned-fixture';
const record={type:2,model_name:'requested-fixture',other:JSON.stringify({is_model_mapped:true,upstream_model_name:'mapped-fixture',response_model:{requested_model:'requested-fixture',upstream_model:'mapped-fixture',returned_model:returned}})};
const columns=getLogsColumns({t:v=>v,COLUMN_KEYS:{},isAdminUser:params.get('admin')==='true',copyText:async(e,text)=>{e.stopPropagation();await copy(text)}});
const cell=columns.find(c=>c.dataIndex==='model_name');
createRoot(document.getElementById('root')).render(<main style={{padding:24}}><h1>使用日志</h1><section data-testid="model-cell">{cell.render(record.model_name,record)}</section></main>);`,
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
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  for (const admin of [false, true]) {
    for (const dark of [false, true]) {
      const page = await browser.newPage();
      const errors = [];
      page.on('pageerror', (error) => errors.push(error.message));
      await page.setViewport({ width: dark ? 390 : 1440, height: 900 });
      await page.setRequestInterception(true);
      page.on('request', (request) => {
        const url = new URL(request.url());
        if (url.origin !== base || /^\/(api|v1)\//.test(url.pathname))
          return request.abort();
        return request.continue();
      });
      for (const returned of ['returned-fixture', 'mapped-fixture-20261024']) {
        await page.goto(
          `${base}/${path.basename(fixture)}/index.html?admin=${admin}&returned=${returned}`,
        );
        await page.waitForSelector('[data-testid="model-cell"] .semi-tag');
        await page.evaluate((dark) => {
          document.documentElement.classList.toggle('dark', dark);
          if (dark) document.body.setAttribute('theme-mode', 'dark');
        }, dark);
        await page.hover('[data-testid="model-cell"] .semi-tag');
        await page.waitForSelector('.semi-popover-wrapper', { visible: true });
        for (const model of ['requested-fixture', 'mapped-fixture', returned]) {
          const tags = await page.$$('.semi-popover-wrapper .semi-tag');
          let target;
          for (const tag of tags) {
            if ((await tag.evaluate((el) => el.textContent)) === model)
              target = tag;
          }
          assert.ok(target, `missing ${model}`);
          await target.click();
          assert.equal(await page.evaluate(() => window.copied.at(-1)), model);
        }
        const text = await page.$eval(
          '.semi-popover-wrapper',
          (el) => el.textContent,
        );
        assert.ok(
          text.includes(
            returned === 'returned-fixture'
              ? '仅供排查，不影响计费'
              : '不能据此验证实际模型身份',
          ),
        );
        assert.deepEqual(await page.evaluate(() => window.copied), [
          'requested-fixture',
          'mapped-fixture',
          returned,
        ]);
      }
      assert.deepEqual(errors, []);
      await page.close();
    }
  }
  console.log(
    'PASS: usage model copy, requested/mapped/returned, mismatch/match, user/admin, desktop/mobile dark',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
