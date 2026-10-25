import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'bun:test';

// Bun module mocks are process-scoped. Keep the real route rendering and its
// presentation mocks isolated from other test files, regardless of run order.
if (process.env.NEWAPI_MODEL_DETAIL_ROUTES_TEST === import.meta.url) {
  const { afterEach, beforeAll, mock } = await import('bun:test');
  const React = (await import('react')).default;
  const { renderToStaticMarkup } = await import('react-dom/server');
  const router = await import('react-router-dom');
  const { StaticRouter } = await import('react-router-dom/server');
  const { StatusContext } = await import('../src/context/Status');

  const routerExports = { ...router };
  mock.module('react-router-dom', () => ({
    ...routerExports,
    Navigate: ({ to }) => <a data-auth-redirect href={to} />,
  }));
  mock.module('../src/helpers/history', () => ({ history: { location: {} } }));
  mock.module('../src/components/layout/SetupCheck', () => ({
    default: ({ children }) => children,
  }));
  mock.module('../src/components/common/ui/Loading', () => ({
    default: () => <span>Loading page</span>,
  }));
  mock.module('../src/pages/NotFound', () => ({
    default: () => <span>Not found</span>,
  }));
  mock.module('../src/pages/Forbidden', () => ({
    default: () => <span>Forbidden</span>,
  }));
  mock.module('../src/pages/Pricing', () => ({
    default: () => <span>Catalog page</span>,
  }));
  mock.module('../src/pages/ModelDetail', () => ({
    default: () => <span>Detail page</span>,
  }));
  let App;
  beforeAll(async () => {
    ({ default: App } = await import('../src/App'));
  });
  const originalStorage = globalThis.localStorage;
  afterEach(() => {
    if (originalStorage === undefined) delete globalThis.localStorage;
    else globalThis.localStorage = originalStorage;
  });
  const renderPath = (path, pricing, user = null) => {
    globalThis.localStorage = { getItem: () => user && JSON.stringify(user) };
    return renderToStaticMarkup(
      <StatusContext.Provider
        value={[
          { status: { HeaderNavModules: JSON.stringify({ pricing }) } },
          () => {},
        ]}
      >
        <StaticRouter location={path}>
          <App />
        </StaticRouter>
      </StatusContext.Provider>,
    );
  };

  test('catalog and encoded model detail routes enforce the same configured authentication', () => {
    for (const path of [
      '/pricing',
      '/model/vendor%2Fmodel',
      '/model/unknown-model',
    ]) {
      assert.match(
        renderPath(path, { enabled: true, requireAuth: true }),
        /data-auth-redirect="true" href="\/login"/,
      );
      assert.doesNotMatch(
        renderPath(path, { enabled: true, requireAuth: true }, { id: 3 }),
        /data-auth-redirect/,
      );
      for (const pricing of [
        true,
        false,
        { enabled: true, requireAuth: false },
      ]) {
        assert.doesNotMatch(renderPath(path, pricing), /data-auth-redirect/);
      }
    }
  });
} else {
  test('model detail authentication routes pass in an isolated module context', () => {
    const result = spawnSync(
      process.execPath,
      ['test', fileURLToPath(import.meta.url)],
      {
        env: {
          ...process.env,
          NEWAPI_MODEL_DETAIL_ROUTES_TEST: import.meta.url,
        },
        encoding: 'utf8',
        timeout: 30000,
      },
    );
    assert.equal(
      result.status,
      0,
      `${result.error || ''}\n${result.stdout}\n${result.stderr}`,
    );
  });
}
