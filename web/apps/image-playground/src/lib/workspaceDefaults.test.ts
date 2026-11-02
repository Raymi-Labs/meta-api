import { afterEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_PARAMS } from '../types'
import { createDefaultOpenAIProfile, DEFAULT_SETTINGS, getActiveApiProfile, getAgentApiProfile, getAgentImageProfile, normalizeSettings } from './apiProfiles'
import { buildSettingsFromUrlParams } from './urlSettings'
import * as canvasImage from './canvasImage'
import { callAgentResponsesApi, callBatchImageSingle } from './agentApi'

const image = 'data:image/png;base64,aW1hZ2U='
function launch(model: string | undefined = 'gpt-6-sol') {
  return new URLSearchParams({ settings: JSON.stringify({ profiles: [{ id: 'newapi-image-playground', name: '默认', provider: 'openai', baseUrl: 'https://fixture.example/v1', apiKey: 'fixture-key', model: 'gpt-image-2', apiMode: 'images', streamImages: false, agentTextModel: model }] }) })
}
function settings() { return normalizeSettings(buildSettingsFromUrlParams(DEFAULT_SETTINGS, launch())) }

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('managed workspace default contracts', () => {
  it('imports/reimports and revokes metadata without mutating gallery or custom settings', () => {
    let current = settings()
    expect(getAgentApiProfile(current)).toMatchObject({ model: 'gpt-6-sol', agentImageModel: 'gpt-image-2', apiMode: 'responses' })
    expect(getActiveApiProfile(current)).toMatchObject({ model: 'gpt-image-2', apiMode: 'images' })
    expect(getAgentImageProfile(getAgentApiProfile(current)).model).toBe('gpt-image-2')
    current = normalizeSettings(buildSettingsFromUrlParams(current, launch('gpt-6-sol-next')))
    expect(current.profiles).toHaveLength(1)
    expect(getAgentApiProfile(current).model).toBe('gpt-6-sol-next')
    current = normalizeSettings(buildSettingsFromUrlParams(current, launch('')))
    expect(getAgentApiProfile(current).apiMode).toBe('images')
    const custom = createDefaultOpenAIProfile({ apiMode: 'responses', model: 'my-model' })
    expect(getAgentApiProfile(normalizeSettings({ profiles: [custom] }))).toMatchObject({ model: 'my-model', agentImageModel: undefined })
  })

  it.each([false, true])('runs text tool planning → Images API → tool result with edit=%s', async (edit) => {
    if (edit) vi.spyOn(canvasImage, 'imageDataUrlToPngBlob').mockResolvedValue(new Blob(['image'], { type: 'image/png' }))
    const calls: Array<{ url: string; body: any }> = []
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input)
      if (url === image) return new Response(new Blob(['image'], { type: 'image/png' }))
      const body = init?.body instanceof FormData ? init.body : JSON.parse(String(init?.body))
      calls.push({ url, body })
      if (url.endsWith('/responses')) {
        expect(body.model).toBe('gpt-6-sol')
        expect(body.tools.map((tool: any) => tool.type)).toEqual(['function', 'function'])
        const output = calls.length === 1 ? [{ type: 'function_call', name: 'generate_image_batch', call_id: 'image-call', arguments: '{"images":[{"id":"one","prompt":"cat"}]}' }] : [{ type: 'message', content: [{ type: 'output_text', text: '完成' }] }]
        return new Response(JSON.stringify({ output }), { headers: { 'Content-Type': 'application/json' } })
      }
      expect(url).toBe(`https://fixture.example/v1/images/${edit ? 'edits' : 'generations'}`)
      expect(body instanceof FormData ? body.get('model') : body.model).toBe('gpt-image-2')
      if (edit) { expect(body.getAll('image[]')).toHaveLength(1); expect(body.get('mask')).toBeTruthy() }
      return new Response(JSON.stringify({ data: [{ b64_json: 'aW1hZ2U=' }] }), { headers: { 'Content-Type': 'application/json' } })
    })
    const config = settings(), profile = getAgentApiProfile(config)
    const plan = await callAgentResponsesApi({ settings: { ...config, agentWebSearch: false }, profile, params: DEFAULT_PARAMS, input: [{ role: 'user', content: 'cat' }] })
    expect(plan.outputItems?.[0].type).toBe('function_call')
    const result = await callBatchImageSingle({ profile, params: DEFAULT_PARAMS, batchItemId: 'one', prompt: 'cat', referenceImageDataUrls: edit ? [image] : [], maskDataUrl: edit ? image : undefined })
    expect(result.error).toBeNull()
    expect(result.image?.dataUrl).toBe(image)
    const final = await callAgentResponsesApi({ settings: config, profile, params: DEFAULT_PARAMS, input: [{ type: 'function_call_output', call_id: 'image-call', output: JSON.stringify({ status: 'done' }) }] })
    expect(final.text).toBe('完成')
    expect(calls).toHaveLength(3)
  })

  it('propagates cancellation to in-flight Images requests and never starts already cancelled requests', async () => {
    const cancel = new AbortController()
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
    }))
    const opts = { profile: getAgentApiProfile(settings()), params: DEFAULT_PARAMS, batchItemId: 'one', prompt: 'cat', referenceImageDataUrls: [], signal: cancel.signal }
    const pending = callBatchImageSingle(opts)
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    cancel.abort()
    expect((await pending).error).toBe('请求已取消')
    expect((await callBatchImageSingle(opts)).error).toBe('请求已取消')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('returns image errors without retries or using the text model for images', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { message: 'fixture unavailable' } }), { status: 503 }))
    const result = await callBatchImageSingle({ profile: getAgentApiProfile(settings()), params: DEFAULT_PARAMS, batchItemId: 'one', prompt: 'cat', referenceImageDataUrls: [] })
    expect(result.image).toBeNull()
    expect(result.error).toContain('fixture unavailable')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
