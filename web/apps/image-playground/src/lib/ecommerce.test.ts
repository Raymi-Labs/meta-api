import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDefaultOpenAIProfile, DEFAULT_SETTINGS } from './apiProfiles'
import { callEcommercePlanApi, createDefaultEcommerceBrief, DEFAULT_ECOMMERCE_CAPABILITIES } from './ecommerce'

describe('ecommerce planning model', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it.each(['gpt-6-sol', ''])('uses gpt-6-sol for the default and empty launch model (%s)', async (model) => {
    vi.stubGlobal('window', { setTimeout, clearTimeout })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      output: [{ type: 'message', content: [{ type: 'output_text', text: '{"items":[]}' }] }],
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const profile = createDefaultOpenAIProfile({
      apiKey: 'test-key', baseUrl: 'https://example.test/v1', model: 'gpt-image-2', apiMode: 'images', apiProxy: false,
    })
    await callEcommercePlanApi({
      settings: DEFAULT_SETTINGS,
      profile,
      capabilities: model ? DEFAULT_ECOMMERCE_CAPABILITIES : { ...DEFAULT_ECOMMERCE_CAPABILITIES, defaultPlanModel: '' },
      brief: createDefaultEcommerceBrief(), assets: [], skeleton: [],
    })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('https://example.test/v1/responses')
    expect(JSON.parse(String(init?.body))).toMatchObject({ model: 'gpt-6-sol', text: { format: { type: 'json_schema' } } })
    expect(profile.model).toBe('gpt-image-2')
  })
})
