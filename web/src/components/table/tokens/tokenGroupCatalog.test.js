import { describe, expect, mock, test } from 'bun:test';

mock.module('../../../helpers/apiCore', () => ({ API: {} }));

const {
  catalogGroupNames,
  loadTokenGroupCatalog,
  parseTokenGroupCatalog,
  peekTokenGroupCatalog,
  resetTokenGroupCatalogForTest,
  TOKEN_GROUP_CATALOG_FRESH_MS,
} = await import('./tokenGroupCatalog');

describe('token group catalog cache', () => {
  test('group names skip auto and keep order', () => {
    expect(
      catalogGroupNames([{ value: 'b' }, 'auto', { value: 'a' }, null]),
    ).toEqual(['b', 'a']);
    expect(
      catalogGroupNames(Array.from({ length: 120 }, (_, i) => `g${i}`)),
    ).toHaveLength(100);
  });

  test('vendors come with the catalog', () => {
    expect(
      parseTokenGroupCatalog([
        { name: 'gpt-x', groups: ['a'], vendor: 'OpenAI' },
        { name: 'odd', groups: ['b'] },
        { name: 'broken' },
      ]),
    ).toEqual({
      catalog: { 'gpt-x': ['a'], odd: ['b'] },
      vendors: { 'gpt-x': 'OpenAI' },
    });
  });

  test('one request is shared, cached per user and group list', async () => {
    resetTokenGroupCatalogForTest();
    let calls = 0;
    const fetcher = async (names) => {
      calls += 1;
      return { catalog: { m: names }, vendors: { m: `v${calls}` } };
    };
    const [first, second] = await Promise.all([
      loadTokenGroupCatalog(1, ['a', 'b'], { fetcher }),
      loadTokenGroupCatalog(1, ['a', 'b'], { fetcher }),
    ]);
    expect(calls).toBe(1);
    expect(second).toBe(first);
    expect(peekTokenGroupCatalog(1, ['a', 'b'])).toMatchObject({
      vendors: { m: 'v1' },
      fresh: true,
    });
    expect(
      peekTokenGroupCatalog(
        1,
        ['a', 'b'],
        Date.now() + TOKEN_GROUP_CATALOG_FRESH_MS + 1,
      ).fresh,
    ).toBe(false);
    expect(peekTokenGroupCatalog(2, ['a', 'b'])).toBeNull();
    expect(peekTokenGroupCatalog(1, ['b', 'a'])).toBeNull();

    await loadTokenGroupCatalog(1, ['a', 'b'], { fetcher });
    expect(calls).toBe(1);
    await loadTokenGroupCatalog(1, ['a', 'b'], { fetcher, force: true });
    expect(calls).toBe(2);
    expect(peekTokenGroupCatalog(1, ['a', 'b']).vendors).toEqual({ m: 'v2' });
  });

  test('a failed refresh keeps the last good copy', async () => {
    resetTokenGroupCatalogForTest();
    await loadTokenGroupCatalog(1, ['a'], {
      fetcher: async () => ({ catalog: { m: ['a'] }, vendors: {} }),
    });
    await expect(
      loadTokenGroupCatalog(1, ['a'], {
        force: true,
        fetcher: async () => {
          throw new Error('offline');
        },
      }),
    ).rejects.toThrow('offline');
    expect(peekTokenGroupCatalog(1, ['a']).catalog).toEqual({ m: ['a'] });
    expect(await loadTokenGroupCatalog(1, [])).toEqual({
      catalog: {},
      vendors: {},
    });
  });
});
