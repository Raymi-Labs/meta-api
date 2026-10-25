/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createImageLanguageStore,
  IMAGE_LANGUAGES,
  imageLanguageStore,
  normalizeImageLanguage,
  t,
  translateMessage,
} from './language'
import en from './locales/en.json'
import { DEFAULT_PARAMS } from './types'

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value)
    },
    removeItem: (key: string) => {
      values.delete(key)
    },
  }
}
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('standalone image language state', () => {
  it('shares all 17 codes and host aliases', () => {
    expect(IMAGE_LANGUAGES).toHaveLength(17)
    expect(normalizeImageLanguage('zh_HK')).toBe('zh-TW')
    expect(normalizeImageLanguage('de-DE')).toBe('de')
    expect(normalizeImageLanguage('vi-VN')).toBe('vi')
  })

  it('restores the canonical preference on refresh and the next full page', async () => {
    const storage = memoryStorage({
      'site-language': 'fr',
      i18nextLng: 'zh-CN',
    })
    const load = vi.fn(async () => ({ 画廊: 'Galerie' }))
    const first = createImageLanguageStore({ storage: () => storage, load })
    expect(first.getSnapshot().language).toBe('fr')
    await first.change('de')
    const nextPage = createImageLanguageStore({ storage: () => storage, load })
    expect(nextPage.getSnapshot().language).toBe('de')
    expect(storage.getItem('site-language')).toBe('de')
  })

  it('ignores an older language request that finishes after the latest choice', async () => {
    const storage = memoryStorage({ 'site-language': 'zh-CN' })
    let finishEnglish!: (dictionary: Record<string, string>) => void
    const store = createImageLanguageStore({
      storage: () => storage,
      load: (language) =>
        language === 'en'
          ? new Promise((resolve) => {
              finishEnglish = resolve
            })
          : Promise.resolve({ 画廊: 'ギャラリー' }),
    })
    const older = store.change('en')
    expect(await store.change('ja')).toBe(true)
    finishEnglish({ 画廊: 'Gallery' })
    expect(await older).toBe(false)
    expect(store.getSnapshot().language).toBe('ja')
    expect(store.getDictionary()['画廊']).toBe('ギャラリー')
    expect(storage.getItem('site-language')).toBe('ja')
  })

  it('keeps the active language on a loading failure and allows retry', async () => {
    const storage = memoryStorage({ 'site-language': 'zh-CN' })
    const load = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({ 画廊: 'Gallery' })
    const store = createImageLanguageStore({ storage: () => storage, load })
    expect(await store.change('en')).toBe(false)
    expect(store.getSnapshot()).toEqual({
      language: 'zh-CN',
      pending: null,
      error: true,
      storageError: false,
    })
    expect(storage.getItem('site-language')).toBe('zh-CN')
    expect(await store.change('en')).toBe(true)
    expect(store.getSnapshot()).toEqual({
      language: 'en',
      pending: null,
      error: false,
      storageError: false,
    })
  })

  it('observes other tabs and pageshow, and removes listeners on disposal', async () => {
    const storage = memoryStorage({ 'site-language': 'zh-CN' })
    const target = Object.assign(new EventTarget(), {
      document: Object.assign(new EventTarget(), {
        visibilityState: 'visible',
      }),
    })
    const store = createImageLanguageStore({
      storage: () => storage,
      load: async () => ({}),
    })
    const dispose = store.initialize(target as unknown as Window)
    await tick()
    storage.setItem('site-language', 'uk')
    target.dispatchEvent(
      Object.assign(new Event('storage'), { key: 'site-language' }),
    )
    await tick()
    expect(store.getSnapshot().language).toBe('uk')
    storage.setItem('site-language', 'bg')
    target.dispatchEvent(new Event('pageshow'))
    await tick()
    expect(store.getSnapshot().language).toBe('bg')
    dispose()
    storage.setItem('site-language', 'ja')
    target.dispatchEvent(new Event('focus'))
    await tick()
    expect(store.getSnapshot().language).toBe('bg')
  })

  it('does not let focus restoration cancel a pending explicit choice', async () => {
    const storage = memoryStorage({ 'site-language': 'zh-CN' })
    let finish!: (dictionary: Record<string, string>) => void
    const store = createImageLanguageStore({
      storage: () => storage,
      load: () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    })
    const loading = store.change('en')
    store.refresh()
    expect(store.getSnapshot().pending).toBe('en')
    finish({ 画廊: 'Gallery' })
    expect(await loading).toBe(true)
    expect(store.getSnapshot().language).toBe('en')
  })

  it('keeps the in-memory choice after blocked browser storage and ignores another storage area', async () => {
    const storage = memoryStorage({ 'site-language': 'zh-CN' })
    storage.setItem = () => {
      throw new Error('blocked')
    }
    const store = createImageLanguageStore({
      storage: () => storage,
      load: async () => ({}),
    })
    const target = Object.assign(new EventTarget(), {
      document: Object.assign(new EventTarget(), {
        visibilityState: 'visible',
      }),
    })
    const dispose = store.initialize(target as unknown as Window)
    await tick()
    expect(await store.change('en')).toBe(true)
    expect(store.getSnapshot().storageError).toBe(true)
    target.dispatchEvent(new Event('focus'))
    target.dispatchEvent(
      Object.assign(new Event('storage'), {
        key: 'site-language',
        storageArea: memoryStorage(),
      }),
    )
    target.dispatchEvent(
      Object.assign(new Event('storage'), {
        key: 'i18nextLng',
        storageArea: storage,
      }),
    )
    await tick()
    expect(store.getSnapshot().language).toBe('en')
    dispose()
  })

  it('records an explicit choice for the host account bridge without copying account data', async () => {
    for (const user of [
      null,
      { id: 17, name: 'private name', setting: '{}' },
    ]) {
      const storage = memoryStorage({
        'site-language': 'zh-CN',
        user: JSON.stringify(user),
      })
      const store = createImageLanguageStore({
        storage: () => storage,
        load: async () => ({}),
      })
      await store.change('en')
      expect(
        JSON.parse(storage.getItem('site-language-pending-account')!),
      ).toEqual({ accountId: user ? '17' : null, language: 'en' })
    }
  })

  it('reports account marker storage failure after the canonical preference succeeds and can retry', async () => {
    const storage = memoryStorage({
      'site-language': 'zh-CN',
      user: JSON.stringify({ id: 17 }),
    })
    const write = storage.setItem
    let blockAccountMarker = true
    storage.setItem = (key, value) => {
      if (key === 'site-language-pending-account' && blockAccountMarker)
        throw new Error('Storage quota exceeded')
      write(key, value)
    }
    const store = createImageLanguageStore({
      storage: () => storage,
      load: async () => ({ 画廊: 'Gallery' }),
    })
    expect(await store.change('en')).toBe(true)
    expect(store.getSnapshot()).toEqual({
      language: 'en',
      pending: null,
      error: false,
      storageError: true,
    })
    expect(storage.getItem('site-language')).toBe('en')
    expect(storage.getItem('site-language-pending-account')).toBeNull()
    store.refresh()
    expect(store.getSnapshot().language).toBe('en')

    blockAccountMarker = false
    expect(await store.change('en')).toBe(true)
    expect(store.getSnapshot().storageError).toBe(false)
    expect(
      JSON.parse(storage.getItem('site-language-pending-account')!),
    ).toEqual({ accountId: '17', language: 'en' })
  })

  it('does not alter tasks, inputs, model output, prompts or API credentials', async () => {
    const storage = memoryStorage({ 'site-language': 'zh-CN' })
    vi.stubGlobal('localStorage', storage)
    const { useStore } = await import('./store')
    const original = useStore.getState()
    const tasks: typeof original.tasks = [
      {
        id: 'language-running-task',
        prompt: '用户输入：画廊',
        status: 'running',
        apiModel: 'custom-image-model',
        outputImages: ['model-output-1'],
        revisedPromptByImage: { 'model-output-1': '模型返回的原始描述：画廊' },
        params: { ...DEFAULT_PARAMS },
        inputImageIds: [],
        error: null,
        createdAt: 1,
        finishedAt: null,
        elapsed: null,
      },
    ]
    const inputImages = [
      { id: 'reference-1', dataUrl: 'data:image/png;base64,test' },
    ]
    const token = 'sk-language-regression-fixture-not-a-real-token'
    const settings = {
      ...original.settings,
      apiKey: token,
      model: 'custom-image-model',
      profiles: original.settings.profiles.map((profile) => ({
        ...profile,
        apiKey: token,
        name: '我的中文配置',
        model: 'custom-image-model',
      })),
    }
    const persistedSettings = JSON.stringify(settings)
    storage.setItem('image-settings-test-fixture', persistedSettings)
    useStore.setState({
      tasks,
      prompt: '不要翻译我的提示词 @图1',
      inputImages,
      settings,
    })
    const store = createImageLanguageStore({
      storage: () => storage,
      load: async () => ({}),
    })
    await store.change('en')
    await store.change('ja')
    expect(useStore.getState().tasks).toBe(tasks)
    expect(useStore.getState().tasks[0].status).toBe('running')
    expect(useStore.getState().inputImages).toBe(inputImages)
    expect(useStore.getState().settings).toBe(settings)
    expect(useStore.getState().settings.apiKey).toBe(token)
    expect(useStore.getState().settings.profiles[0].apiKey).toBe(token)
    expect(useStore.getState().settings.profiles[0].name).toBe('我的中文配置')
    expect(useStore.getState().tasks[0].apiModel).toBe('custom-image-model')
    expect(
      useStore.getState().tasks[0].revisedPromptByImage?.['model-output-1'],
    ).toBe('模型返回的原始描述：画廊')
    expect(storage.getItem('image-settings-test-fixture')).toBe(
      persistedSettings,
    )
    expect(useStore.getState().prompt).toBe('不要翻译我的提示词 @图1')
    useStore.setState(original, true)
  })

  it('translates local messages while keeping substituted user data and unknown errors intact', async () => {
    await imageLanguageStore.change('en', false)
    expect(t('画廊')).toBe(en['画廊'])
    const title = '用户起的中文名称：画廊'
    expect(translateMessage(`已提交：${title}`)).toBe(
      en['已提交：{{value0}}'].replace('{{value0}}', title),
    )
    const unknown = 'ProviderXYZ 403: 用户原始响应 / request-123'
    expect(translateMessage(unknown)).toBe(unknown)
    await imageLanguageStore.change('zh-CN', false)
    expect(t('画廊')).toBe('画廊')
  })
})
