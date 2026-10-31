import test from 'node:test';
import assert from 'node:assert/strict';
import { globSync, readFileSync } from 'node:fs';
import {
  createProviderIconRegistry,
  createLocalProviderIconRegistry,
  resolveProviderIcon,
} from '../src/helpers/providerIconRegistry.js';

test('POSIX and Windows paths register only supported JS icon components', () => {
  for (const root of [
    '/build/web/node_modules/',
    'D:\\a\\new-api\\web\\node_modules\\',
  ]) {
    const loader = () => {
      throw Error('must not import before rendering');
    };
    const files = [
      'OpenAI/components/Mono.js',
      'Claude/components/Color.js',
      'Gemma/components/Simple.js',
      'LobeHub/components/Morden.js',
      'OpenAI/index.js',
      'OpenAI/components/Mono.d.ts',
      'OpenAI/components/Unknown.js',
    ];
    const registry = createProviderIconRegistry(
      Object.fromEntries(
        files.map((file) => [root + '@lobehub/icons/es/' + file, loader]),
      ),
    );
    assert.deepEqual(
      [...registry.keys()],
      ['OpenAI.Mono', 'Claude.Color', 'Gemma.Simple', 'LobeHub.Morden'],
    );
    assert.equal(
      resolveProviderIcon(registry, 'OpenAI', 'Color'),
      'OpenAI.Mono',
    );
    assert.equal(resolveProviderIcon(registry, 'Unknown', 'Color'), null);
    assert.equal(resolveProviderIcon(registry, '../../OpenAI'), null);
  }
});

test('local registry only accepts checked-in component names', () => {
  const loader = () => {};
  const registry = createLocalProviderIconRegistry({
    '../assets/provider-icons/components/Inception.jsx': loader,
    '..\\assets\\provider-icons\\components\\ZAI.jsx': loader,
    '../assets/provider-icons/components/../../Injected.jsx': loader,
    'https://example.com/Inception.jsx': loader,
    '../assets/provider-icons/components/Unknown.js': loader,
  });
  assert.deepEqual([...registry.keys()], ['Inception.Mono', 'ZAI.Mono']);
  assert.equal(resolveProviderIcon(registry, 'ZAI', 'Color'), 'ZAI.Mono');
  assert.equal(
    resolveProviderIcon(registry, 'https://example.com/logo.svg'),
    null,
  );
});

test('every onboarded vendor resolves to a packaged component or local asset', () => {
  const registry = createProviderIconRegistry(
    Object.fromEntries(
      [...globSync('node_modules/@lobehub/icons/es/*/components/*.js')].map(
        (file) => [file, () => {}],
      ),
    ),
  );
  for (const [key, loader] of createLocalProviderIconRegistry(
    Object.fromEntries(
      [...globSync('src/assets/provider-icons/components/*.jsx')].map(
        (file) => [file, () => {}],
      ),
    ),
  ))
    registry.set(key, loader);
  const assets = JSON.parse(
    readFileSync('src/assets/provider-icons/assets.json', 'utf8'),
  );
  for (const [name, asset] of Object.entries(assets)) {
    assert.match(
      asset.file,
      /^[A-Za-z][A-Za-z0-9-]*\.(svg|png|jpg|jpeg|webp)$/,
    );
    const data = readFileSync(`src/assets/provider-icons/images/${asset.file}`);
    assert.ok(data.length > 100, name);
    assert.ok(['mono', 'image'].includes(asset.mode));
    registry.set(`${name}.Mono`, () => {});
  }
  // Xiaomi is an existing Simple Icons alias in render.jsx.
  registry.set('Xiaomi.Mono', () => {});
  const vendors = JSON.parse(
    readFileSync('../docs/channel/openrouter-vendor-icons.json', 'utf8'),
  );
  for (const vendor of vendors)
    assert.ok(resolveProviderIcon(registry, vendor.icon), vendor.name);
});
test('installed package covers every configured provider and special variant', () => {
  const files = [
    ...globSync('node_modules/@lobehub/icons/es/*/components/*.js'),
  ];
  const registry = createProviderIconRegistry(
    Object.fromEntries(files.map((file) => [file, () => {}])),
  );
  const source = readFileSync(
    new URL('../src/helpers/providerIcons.jsx', import.meta.url),
    'utf8',
  );
  for (const match of source.matchAll(/provider\('([^']+)'\)/g))
    assert.ok(resolveProviderIcon(registry, match[1]), match[1]);
  for (const [name, variant] of [
    ['Claude', 'Color'],
    ['OpenAI', 'Avatar'],
    ['Gemma', 'Simple'],
  ])
    assert.ok(registry.has(`${name}.${variant}`));
  // This installed version predates Morden; preserve the existing Mono fallback.
  assert.equal(
    resolveProviderIcon(registry, 'LobeHub', 'Morden'),
    'LobeHub.Mono',
  );
});
