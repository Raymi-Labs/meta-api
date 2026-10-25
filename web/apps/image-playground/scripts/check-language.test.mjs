/* Copyright (C) 2025 QuantumNous. Licensed under AGPL-3.0-or-later. */
import { describe, expect, it } from 'vitest'
import { collectSourceKeys, validateDictionaries } from './check-language.mjs'

describe('image language coverage gate', () => {
  it('extracts literal keys, static constants and local message templates', () => {
    const keys = collectSourceKeys(`
      const NOTICE = '静态帮助说明';
      t('新的菜单'); t(NOTICE); tsource('源词条');
      showToast(\`已提交：\${userName}\`);
      const userPrompt = '业务提示词原样保留';
    `)
    expect([...keys.keys()]).toEqual([
      '新的菜单',
      '静态帮助说明',
      '源词条',
      '已提交：{{value0}}',
    ])
  })

  it('collects mapped UI labels without collecting preset prompt data', () => {
    const keys = collectSourceKeys(
      `
      const ECOMMERCE_GROUPS = [{ label: '主图', description: '展示说明' }];
      const PRODUCT_ASSET_BUCKETS = [];
      const ECOMMERCE_PLATFORMS = ['新平台'];
      const STYLE_PRESETS = [], SUITE_TEMPLATES = [], SIZE_PRESETS = [];
      const INDUSTRY_PRESETS = [{ name: '新的行业', brief: { productName: '提示词商品名称', extraPrompt: '不可翻译' } }];
    `,
      'lib/ecommerce.ts',
    )
    expect([...keys.keys()]).toEqual(['主图', '展示说明', '新平台', '新的行业'])
  })

  it('rejects missing languages, empty values and changed interpolation tokens', () => {
    const source = { '第 {{count}} 轮': '第 {{count}} 轮', 保存: '保存' }
    expect(
      validateDictionaries(source, {
        en: { '第 {{count}} 轮': 'Round {{count}}', 保存: 'Save' },
      }),
    ).toEqual([])
    expect(
      validateDictionaries(source, {
        en: { '第 {{count}} 轮': 'Round {{index}}', 保存: ' ' },
        ja: null,
      }),
    ).toEqual([
      'en: interpolation mismatch: 第 {{count}} 轮',
      'en: missing or empty: 保存',
      'ja: missing locale resource',
    ])
  })

  it('rejects leaked translation markers even when text and placeholders are otherwise valid', () => {
    const source = { '第 {{count}} 轮': '第 {{count}} 轮' }
    for (const marker of ['ZXQ00001', 'ZXV00002', 'ZXE00003', 'zxq 00004']) {
      expect(
        validateDictionaries(source, {
          en: { '第 {{count}} 轮': `Round {{count}} ${marker}` },
        }),
      ).toEqual(['en: temporary translation marker: 第 {{count}} 轮'])
    }
    expect(
      validateDictionaries(source, {
        en: { '第 {{count}} 轮': 'Round {{count}}' },
      }),
    ).toEqual([])
  })
})
