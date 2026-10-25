import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'bun:test';

// Module mocks must not replace Semi's exports for unrelated tests in the same
// Bun process. Load the real renderer and its adapters only in this child run.
if (process.env.NEWAPI_PRICING_PAGINATION_TEST === import.meta.url) {
  const { beforeAll, mock } = await import('bun:test');
  const React = (await import('react')).default;
  const { renderToStaticMarkup } = await import('react-dom/server');
  const SemiTable = (await import('@douyinfe/semi-ui/lib/es/table')).default;

  // Use the real Semi Table renderer: its controlled pagination intentionally
  // does not slice dataSource. Only unrelated card/illustration chrome is stubbed.
  let tableProps;
  mock.module('@douyinfe/semi-ui', () => ({
    Card: ({ children }) => <>{children}</>,
    Empty: ({ description }) => <span>{description}</span>,
    Table: (props) => {
      tableProps = props;
      return <SemiTable {...props} />;
    },
  }));
  mock.module('@douyinfe/semi-illustrations', () => ({
    IllustrationNoResult: () => null,
    IllustrationNoResultDark: () => null,
  }));
  mock.module(
    '../src/components/table/model-pricing/view/table/PricingTableColumns',
    () => ({
      getPricingTableColumns: () => [
        { title: 'Model', dataIndex: 'model_name' },
      ],
    }),
  );
  let PricingTable;
  beforeAll(async () => {
    ({ default: PricingTable } = await import(
      '../src/components/table/model-pricing/view/table/PricingTable'
    ));
  });
  const models = Array.from({ length: 89 }, (_, index) => ({
    key: `model-${index + 1}`,
    model_name: `fixture-model-${String(index + 1).padStart(3, '0')}`,
  }));
  const renderPage = ({
    currentPage = 1,
    pageSize = 10,
    filteredModels = models,
    ...props
  } = {}) =>
    renderToStaticMarkup(
      <PricingTable
        filteredModels={filteredModels}
        currentPage={currentPage}
        pageSize={pageSize}
        setPageSize={() => {}}
        setCurrentPage={() => {}}
        t={(key) => key}
        {...props}
      />,
    );

  test('controlled table pagination renders only the requested catalog page', () => {
    for (const [currentPage, pageSize, start, end] of [
      [1, 10, 1, 10],
      [2, 10, 11, 20],
      [9, 10, 81, 89],
      [2, 20, 21, 40],
    ]) {
      const html = renderPage({ currentPage, pageSize });
      const expected = models
        .slice(start - 1, end)
        .map((item) => item.model_name);
      assert.deepEqual(
        [...html.matchAll(/>(fixture-model-\d{3})<\/td>/g)].map(
          ([, name]) => name,
        ),
        expected,
      );
      assert.equal(tableProps.pagination.total, 89);
      assert.equal(tableProps.pagination.currentPage, currentPage);
    }
  });

  test('page-size changes reset the current page and preserve full filtered total', () => {
    const changes = [];
    renderPage({
      currentPage: 9,
      setPageSize: (size) => changes.push(['size', size]),
      setCurrentPage: (page) => changes.push(['page', page]),
    });
    tableProps.pagination.onPageSizeChange(50);
    assert.deepEqual(changes, [
      ['size', 50],
      ['page', 1],
    ]);
    assert.equal(tableProps.pagination.total, models.length);
  });

  test('pagination total and rendered rows follow the filtered catalog including empty results', () => {
    const filtered = models.filter((_, index) => index % 2 === 0);
    const html = renderPage({ currentPage: 2, filteredModels: filtered });
    assert.deepEqual(
      [...html.matchAll(/>(fixture-model-\d{3})<\/td>/g)].map(
        ([, name]) => name,
      ),
      filtered.slice(10, 20).map((item) => item.model_name),
    );
    assert.equal(tableProps.pagination.total, filtered.length);
    assert.doesNotMatch(renderPage({ filteredModels: [] }), /fixture-model-/);
    assert.equal(tableProps.pagination.total, 0);
  });
} else {
  test('real Semi table pagination passes in an isolated module context', () => {
    const result = spawnSync(
      process.execPath,
      ['test', fileURLToPath(import.meta.url)],
      {
        env: {
          ...process.env,
          NEWAPI_PRICING_PAGINATION_TEST: import.meta.url,
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
