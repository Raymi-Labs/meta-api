import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

// Load the actual storage implementation without the UI-only constants imports.
const source = (
  await readFile(
    new URL('../src/components/playground/configStorage.js', import.meta.url),
    'utf8',
  )
).replace(
  /import \{[\s\S]*?\} from '..\/..\/constants\/playground.constants';/,
  `const STORAGE_KEYS = { CONFIG: 'playground_config', MESSAGES: 'playground_messages' }; const DEFAULT_CONFIG = { inputs: {}, parameterEnabled: {}, showDebugPanel: false, customRequestMode: false, customRequestBody: '' };`,
);
const storage = new Map();
globalThis.localStorage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, value),
  removeItem: (key) => storage.delete(key),
};
const api = await import(
  `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
);
const login = (id) => localStorage.setItem('user', JSON.stringify({ id }));
storage.set(
  'playground_messages',
  JSON.stringify({ messages: ['legacy private'] }),
);
login(1);
api.loadConfig();
assert.equal(api.loadMessages(), null);
assert.equal(storage.has('playground_messages'), false);
api.saveMessages(['A']);
api.saveConfig({ inputs: { model: 'A' }, showDebugPanel: true });
login(2);
assert.equal(api.loadMessages(), null);
api.saveMessages(['B']);
api.saveMessages(['late A'], '1');
api.saveConfig({ inputs: { model: 'late A' } }, '1');
assert.deepEqual(api.loadMessages(), ['B']);
login(1);
assert.deepEqual(api.loadMessages(), ['A']);
assert.equal(api.loadConfig().inputs.model, 'A');
api.clearConfig();
assert.equal(api.loadMessages(), null);
login(2);
assert.deepEqual(api.loadMessages(), ['B']);
localStorage.removeItem('user');
api.saveMessages(['anonymous']);
assert.equal(api.loadMessages(), null);
assert.equal(storage.has('playground_messages:user:null'), false);
storage.set('user', 'malformed');
assert.equal(api.getPlaygroundOwner(), null);
console.log('Playground account isolation tests passed');
