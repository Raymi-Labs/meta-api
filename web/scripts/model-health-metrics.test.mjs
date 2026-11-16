import assert from 'node:assert/strict';
import { test } from 'bun:test';
import { supportsTextGenerationMetrics } from '../src/helpers/modelHealthMetrics.js';

test('text generation metrics follow declared output, endpoint, and streaming support', () => {
  for (const endpoint of [
    'embeddings',
    'jina-rerank',
    'decisions',
    'audio-speech',
    'audio-transcription',
    'audio-translation',
    'realtime',
  ]) {
    assert.equal(
      supportsTextGenerationMetrics({ supported_endpoint_types: [endpoint] }),
      false,
      endpoint,
    );
  }
  assert.equal(
    supportsTextGenerationMetrics({ output_modalities: ['audio'] }),
    false,
  );
  assert.equal(
    supportsTextGenerationMetrics({ output_modalities: ['image'] }),
    false,
  );
  assert.equal(supportsTextGenerationMetrics({ output_modalities: [] }), false);
  assert.equal(
    supportsTextGenerationMetrics({ capabilities: { streaming: false } }),
    false,
  );
  assert.equal(
    supportsTextGenerationMetrics({
      output_modalities: ['text'],
      supported_endpoint_types: ['openai'],
      capabilities: { streaming: true },
    }),
    true,
  );
  assert.equal(supportsTextGenerationMetrics(undefined), false);
});
