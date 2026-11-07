import { describe, expect, test } from 'bun:test';
import {
  buildVendorSections,
  formPatchForSelection,
  moveTokenGroup,
  nextTokenGroupSelection,
  selectionFromForm,
  sortSelectedGroupsByRatio,
} from './tokenGroupSelection';
import { readTokenRouting } from './tokenRouting';

describe('token group selection', () => {
  test('checking keeps the first group preferred and appends backups', () => {
    expect(nextTokenGroupSelection([], 'a')).toEqual(['a']);
    expect(nextTokenGroupSelection(['a'], 'b')).toEqual(['a', 'b']);
    expect(nextTokenGroupSelection(['a', 'b'], 'a')).toEqual(['b']);
  });

  test('auto is exclusive and single mode keeps one group', () => {
    expect(nextTokenGroupSelection(['a', 'b'], 'auto')).toEqual(['auto']);
    expect(nextTokenGroupSelection(['auto'], 'a')).toEqual(['a']);
    expect(nextTokenGroupSelection(['a'], 'b', false)).toEqual(['b']);
  });

  test('reordering and cheapest-first sorting', () => {
    expect(moveTokenGroup(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b']);
    expect(moveTokenGroup(['a', 'b'], 0, -1)).toEqual(['a', 'b']);
    const byName = new Map([
      ['a', { ratio: 1 }],
      ['b', { ratio: 0.1 }],
      ['c', { ratio: undefined }],
      ['d', { ratio: 0.1 }],
    ]);
    expect(sortSelectedGroupsByRatio(['a', 'c', 'b', 'd'], byName)).toEqual([
      'b',
      'd',
      'a',
      'c',
    ]);
  });

  test('vendor entries list serving groups and match vendor or model', () => {
    const catalog = {
      'gpt-x': ['a', 'b'],
      'gpt-y': ['c'],
      'claude-y': ['b'],
      'odd-z': ['a'],
    };
    const vendors = {
      'gpt-x': 'OpenAI',
      'gpt-y': 'OpenAI',
      'claude-y': 'Anthropic',
    };
    const all = buildVendorSections(catalog, vendors, '', 'Other');
    expect(all.map((entry) => entry.vendor)).toEqual([
      'Anthropic',
      'OpenAI',
      'Other',
    ]);
    expect([...all[1].groups].sort()).toEqual(['a', 'b', 'c']);
    expect(all[1].models).toEqual(['gpt-x', 'gpt-y']);

    const byVendor = buildVendorSections(catalog, vendors, 'open', 'Other');
    expect(byVendor.map((entry) => entry.vendor)).toEqual(['OpenAI']);
    expect(byVendor[0].matchedModels).toEqual([]);

    const byModel = buildVendorSections(catalog, vendors, 'GPT-Y', 'Other');
    expect(byModel.map((entry) => entry.vendor)).toEqual(['OpenAI']);
    expect(byModel[0].matchedModels).toEqual(['gpt-y']);
    expect([...byModel[0].groups].sort()).toEqual(['a', 'b', 'c']);

    expect(buildVendorSections(catalog, vendors, 'nothing', 'Other')).toEqual(
      [],
    );
  });

  test('one group is a plain token, more become a routing config', () => {
    expect(formPatchForSelection(['a'], {}, readTokenRouting)).toEqual({
      group: 'a',
      routing_enabled: false,
      routing_config: '',
    });
    const values = {
      routing_config: JSON.stringify({
        groups: ['a', 'b'],
        model_groups: { m: ['b'], n: ['c'] },
      }),
      auto_group_fallback: true,
    };
    const patch = formPatchForSelection(['b', 'a'], values, readTokenRouting);
    expect(patch.group).toBe('b');
    expect(patch.routing_enabled).toBe(true);
    expect(JSON.parse(patch.routing_config)).toEqual({
      groups: ['b', 'a'],
      model_groups: { m: ['b'], n: [] },
      auto_fallback: true,
    });
    expect(
      selectionFromForm(
        { routing_enabled: true, routing_config: patch.routing_config },
        readTokenRouting,
      ),
    ).toEqual(['b', 'a']);
    expect(selectionFromForm({ group: 'a' }, readTokenRouting)).toEqual(['a']);
    expect(selectionFromForm({}, readTokenRouting)).toEqual([]);
  });
});
