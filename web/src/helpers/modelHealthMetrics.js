const NON_GENERATIVE_ENDPOINTS = new Set([
  'embeddings',
  'jina-rerank',
  'decisions',
  'audio-speech',
  'audio-transcription',
  'audio-translation',
  'realtime',
]);

export function supportsTextGenerationMetrics(model) {
  if (!model) return false;
  if (model.capabilities?.streaming === false) return false;
  if (
    Array.isArray(model.output_modalities) &&
    !model.output_modalities.includes('text')
  )
    return false;
  return !model.supported_endpoint_types?.some((endpoint) =>
    NON_GENERATIVE_ENDPOINTS.has(endpoint),
  );
}
