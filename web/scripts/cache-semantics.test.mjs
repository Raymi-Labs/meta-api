import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  getNonCachePromptTokens,
  promptTokensExcludeCache,
} from '../src/helpers/cacheCreation.js';
import { resolveTieredLogParams } from '../src/helpers/tieredBillingDisplay.js';

const legacyConverted = {
  claude: true,
  usage_semantic: 'anthropic',
  request_conversion: ['Claude Messages', 'OpenAI Compatible'],
  billing_mode: 'tiered_expr',
  prompt_tokens: 58459,
  cache_tokens: 57856,
  tiered_params: { p: 603, len: 58459, cr: 57856 },
};

test('stored prompt semantic overrides client protocol and stale Claude flags', () => {
  assert.equal(promptTokensExcludeCache(null), false);
  assert.equal(
    promptTokensExcludeCache({
      ...legacyConverted,
      prompt_tokens_semantic: 'inclusive',
      tiered_params: undefined,
    }),
    false,
  );
  assert.equal(
    promptTokensExcludeCache({
      ...legacyConverted,
      prompt_tokens_semantic: 'non_cache',
    }),
    true,
  );
  assert.equal(promptTokensExcludeCache({ usage_semantic: 'anthropic' }), true);
  assert.equal(promptTokensExcludeCache({ usage_semantic: 'Anthropic' }), true);
  assert.equal(promptTokensExcludeCache({ usage_semantic: 'openai' }), false);
});

test('exact settlement snapshot repairs historical cross-protocol counters', () => {
  for (const finalFormat of ['OpenAI Compatible', 'OpenAI Responses']) {
    assert.equal(
      promptTokensExcludeCache({
        ...legacyConverted,
        request_conversion: ['Claude Messages', finalFormat],
      }),
      false,
    );
  }
  const normalizedOpenRouter = {
    ...legacyConverted,
    prompt_tokens: 603,
  };
  assert.equal(promptTokensExcludeCache(normalizedOpenRouter), true);
  // Stored settlement parameters remain the source of truth for old bills.
  assert.equal(resolveTieredLogParams(legacyConverted, {}).p, 603);
  assert.equal(resolveTieredLogParams(normalizedOpenRouter, {}).p, 603);
});

test('ambiguous historical records retain their prior interpretation', () => {
  for (const changed of [
    { billing_mode: undefined },
    { request_conversion: undefined },
    { request_conversion: ['Claude Messages'] },
    { request_conversion: ['OpenAI Compatible', 'Claude Messages'] },
    { tiered_params: undefined },
    { tiered_params: { len: '58459' } },
    { tiered_params: { len: null } },
    { tiered_params: { len: Infinity } },
    { tiered_params: { len: -1 } },
    { tiered_params: { len: 60000 } },
    { cache_tokens: 0 },
  ]) {
    assert.equal(
      promptTokensExcludeCache({ ...legacyConverted, ...changed }),
      true,
    );
  }
});

test('tiered fallback partitions inclusive and non-cache counters consistently', () => {
  const tier = { inputPrice: 1, cacheReadPrice: 0.1, cacheCreatePrice: 1.25 };
  const inclusive = resolveTieredLogParams(
    {
      claude: true,
      prompt_tokens_semantic: 'inclusive',
      prompt_tokens: 1000,
      cache_tokens: 600,
      cache_creation_tokens: 100,
    },
    tier,
  );
  assert.equal(inclusive.p, 300);
  assert.equal(inclusive.len, 1000);
  const nonCache = resolveTieredLogParams(
    {
      prompt_tokens_semantic: 'non_cache',
      prompt_tokens: 300,
      cache_tokens: 600,
      cache_creation_tokens: 100,
    },
    tier,
  );
  assert.equal(nonCache.p, 300);
  assert.equal(nonCache.len, 1000);
});

test('inclusive cache creation is deducted once before rendering its price breakdown', () => {
  const record = {
    prompt_tokens_semantic: 'inclusive',
    prompt_tokens: 1000,
    cache_tokens: 600,
    cache_creation_tokens: 100,
    cache_creation_tokens_5m: 40,
    cache_creation_tokens_1h: 10,
  };
  assert.equal(getNonCachePromptTokens(record), 300);
  assert.equal(
    getNonCachePromptTokens({ ...record, prompt_tokens_semantic: 'non_cache' }),
    1000,
  );
  assert.equal(getNonCachePromptTokens({ ...record, prompt_tokens: 20 }), 0);
  assert.equal(getNonCachePromptTokens(null), 0);
});

test('input explanation does not infer cache semantics from the request path', () => {
  const key =
    '输入 tokens 按日志中的实际用量口径展示：Anthropic 用量不含缓存，OpenAI 用量包含缓存；不能仅凭请求路径判断。';
  for (const locale of ['zh-CN', 'zh-TW', 'en', 'ja', 'fr', 'ru', 'vi']) {
    const { translation } = JSON.parse(
      readFileSync(
        new URL(`../src/i18n/locales/${locale}.json`, import.meta.url),
      ),
    );
    assert.ok(translation[key], locale);
  }
});
