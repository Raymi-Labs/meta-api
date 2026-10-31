// Uses isolated local content and blocks external requests; no production API calls.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { createServer } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
import puppeteer from 'puppeteer-core';

const root = fileURLToPath(new URL('..', import.meta.url));
const require = createRequire(path.join(root, 'package.json'));
const directPurifier = require.resolve('dompurify');
const mermaidPurifier = createRequire(require.resolve('mermaid')).resolve(
  'dompurify',
);
assert.equal(
  mermaidPurifier,
  directPurifier,
  'Mermaid must resolve the same patched DOMPurify; remove stale node_modules or reinstall cleanly',
);
const playgroundRequire = createRequire(
  path.join(root, 'apps/image-playground/package.json'),
);
const playgroundMermaidPath = playgroundRequire.resolve('mermaid');
const playgroundPurifier = createRequire(playgroundMermaidPath).resolve(
  'dompurify',
);
const expectedVersion = JSON.parse(
  await fs.readFile(path.join(root, 'package.json'), 'utf8'),
).dependencies.dompurify;
for (const resolved of [directPurifier, playgroundPurifier]) {
  const metadata = JSON.parse(
    await fs.readFile(
      path.resolve(path.dirname(resolved), '../package.json'),
      'utf8',
    ),
  );
  assert.equal(
    metadata.version,
    expectedVersion,
    'every renderer must use the pinned sanitizer',
  );
}
const fixture = await fs.mkdtemp(path.join(root, '.rich-content-qa-'));
let server, browser;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><body><div id="rendered"></div><script type="module" src="./entry.js"></script></body></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.js'),
    `
import DOMPurify from 'dompurify';
import mermaid from 'mermaid';
import playgroundMermaid from ${JSON.stringify('/' + path.relative(root, playgroundMermaidPath).split(path.sep).join('/'))};
import {sanitizeHtmlContent, renderMarkdownContent} from '/src/helpers/richContent.js';
window.qa = {DOMPurify, mermaid, playgroundMermaid, sanitizeHtmlContent, renderMarkdownContent};
`,
  );
  server = await createServer({
    configFile: false,
    root,
    cacheDir: path.join(fixture, '.vite'),
    server: { host: '127.0.0.1', port: 0, open: false },
    optimizeDeps: {
      entries: [path.join(fixture, 'index.html')],
      include: ['dompurify', 'mermaid', 'marked'],
    },
  });
  await server.listen();
  const base = 'http://127.0.0.1:' + server.httpServer.address().port;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.setRequestInterception(true);
  page.on('request', (request) =>
    new URL(request.url()).origin === base
      ? request.continue()
      : request.abort(),
  );
  await page.goto(base + '/' + path.basename(fixture) + '/index.html');
  await page.waitForFunction(() => Boolean(window.qa), { timeout: 60000 });
  const result = await page.evaluate(async () => {
    const {
      DOMPurify,
      mermaid,
      playgroundMermaid,
      sanitizeHtmlContent,
      renderMarkdownContent,
    } = window.qa;
    const host = document.createElement('div');
    const shadow = host.attachShadow({ mode: 'open' });
    shadow.append(
      document.createTextNode('$'),
      document.createElement('object'),
      document.createTextNode('{7*7}'),
    );
    DOMPurify.sanitize(host, { IN_PLACE: true, SAFE_FOR_TEMPLATES: true });
    shadow.normalize();
    const regression = shadow.textContent;
    const container = document.querySelector('#rendered');
    container.innerHTML = sanitizeHtmlContent(
      '<p onclick="window.qaInjected=true">正常中文<script>window.qaInjected=true</script><a href="javascript:alert(1)">bad</a><a href="https://example.test/docs">safe</a></p>',
    );
    const rich = {
      scripts: container.querySelectorAll('script').length,
      events: container.querySelectorAll('[onclick]').length,
      badHref: container.querySelector('a').getAttribute('href'),
      text: container.textContent,
      safe: container.querySelectorAll('a')[1].outerHTML,
    };
    const markdown = document.createElement('div');
    markdown.innerHTML = renderMarkdownContent(
      '# 文档\n\n| 模型 | 状态 |\n| --- | --- |\n| 测试 | 正常 |\n\n[帮助](https://example.test/help)',
    );
    const documentHTML = sanitizeHtmlContent(
      '<!doctype html><html><head><style>.doc{color:red}</style></head><body><p class="doc">正文</p><script>window.qaInjected=true</script></body></html>',
    );
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict' });
    const { svg } = await mermaid.render(
      'securityDiagram',
      'flowchart LR\n A["安全文本"] --> B["完成"]',
    );
    const diagram = document.createElement('div');
    diagram.innerHTML = svg;
    playgroundMermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
    });
    const imageDiagram = document.createElement('div');
    imageDiagram.innerHTML = (
      await playgroundMermaid.render(
        'playgroundSecurityDiagram',
        'flowchart LR\n A["图片工作台"] --> B["正常"]',
      )
    ).svg;
    const afterDiagram = sanitizeHtmlContent(
      '<p onclick="alert(1)"><a href="javascript:alert(1)">unsafe</a><a href="https://example.test">safe</a><script>alert(1)</script></p>',
    );
    return {
      version: DOMPurify.version,
      regression,
      rich,
      heading: markdown.querySelector('h1')?.textContent,
      cells: markdown.querySelectorAll('td').length,
      documentHTML,
      playgroundSvg: !!imageDiagram.querySelector('svg'),
      playgroundText: imageDiagram.textContent,
      afterDiagram,
      diagramText: diagram.textContent,
      svg: !!diagram.querySelector('svg'),
      diagramScripts: diagram.querySelectorAll('script').length,
      injected: !!window.qaInjected,
    };
  });
  assert.deepEqual(errors, []);
  assert.equal(result.rich.scripts, 0);
  assert.equal(result.rich.events, 0);
  assert.equal(result.rich.badHref, null);
  assert.match(result.rich.text, /正常中文/);
  assert.match(result.rich.safe, /noopener noreferrer/);
  assert.match(result.rich.safe, /target="_blank"/);
  assert.equal(result.heading, '文档');
  assert.equal(result.cells, 2);
  assert.match(result.documentHTML, /<style>/);
  assert.match(result.documentHTML, /正文/);
  assert.doesNotMatch(result.documentHTML, /<script/i);
  assert.equal(result.playgroundSvg, true);
  assert.match(result.playgroundText, /图片工作台/);
  assert.doesNotMatch(result.afterDiagram, /onclick|javascript:|<script/i);
  assert.match(result.afterDiagram, /noopener noreferrer/);
  assert.equal(result.svg, true);
  assert.match(result.diagramText, /安全文本/);
  assert.equal(result.diagramScripts, 0);
  assert.equal(result.injected, false);
  assert.ok(
    !result.regression.includes('$' + '{'),
    'shadow-root split template expression must be scrubbed: ' +
      result.regression,
  );
  console.log(
    JSON.stringify({
      version: result.version,
      richContent: 'passed',
      markdown: 'passed',
      wholeDocument: 'passed',
      mermaid: 'passed',
      playgroundMermaid: 'passed',
      shadowRootRegression: 'passed',
    }),
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
