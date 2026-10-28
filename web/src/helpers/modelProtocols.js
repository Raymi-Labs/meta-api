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
export const MODEL_PROTOCOLS = Object.freeze([
  {
    endpoint: 'openai',
    label: 'Chat Completions',
    path: '/v1/chat/completions',
    example: true,
  },
  {
    endpoint: 'openai-response',
    label: 'Responses',
    path: '/v1/responses',
    example: true,
  },
  {
    endpoint: 'anthropic',
    label: 'Messages',
    path: '/v1/messages',
    example: true,
  },
  {
    endpoint: 'gemini',
    label: 'Gemini',
    path: '/v1beta/models/{model}:generateContent',
    example: true,
  },
  {
    endpoint: 'image-generation',
    label: 'Images',
    path: '/v1/images/generations',
    example: true,
  },
  {
    endpoint: 'openai-response-compact',
    label: 'Responses Compact',
    path: '/v1/responses/compact',
  },
  { endpoint: 'openai-video', label: 'Videos', path: '/v1/videos' },
]);

const TEXT_PROTOCOLS = ['openai', 'openai-response', 'anthropic'];

export function getDeclaredModelProtocols(model) {
  if (!Array.isArray(model?.supported_endpoint_types)) return [];
  return [
    ...new Set(
      model.supported_endpoint_types.filter(
        (value) => typeof value === 'string' && value.trim(),
      ),
    ),
  ];
}

export function getModelProtocolLabel(endpoint) {
  return (
    MODEL_PROTOCOLS.find((item) => item.endpoint === endpoint)?.label ||
    endpoint
  );
}

export function canOfferTextProtocolExamples(model) {
  const declared = getDeclaredModelProtocols(model);
  // Never infer capability from a vendor/model name. Mixed media endpoints are
  // excluded too: a video model can also carry the channel's default openai tag.
  if (!declared.some((endpoint) => TEXT_PROTOCOLS.includes(endpoint)))
    return false;
  if (
    declared.some((endpoint) =>
      /image|video|audio|embed|rerank/i.test(endpoint),
    )
  )
    return false;
  if (
    Array.isArray(model?.output_modalities) &&
    model.output_modalities.length &&
    !model.output_modalities.includes('text')
  )
    return false;
  return true;
}

export function getModelProtocolOptions(
  model,
  includeTextProtocolTemplates = false,
) {
  const declared = getDeclaredModelProtocols(model);
  const templates =
    includeTextProtocolTemplates && canOfferTextProtocolExamples(model);
  return MODEL_PROTOCOLS.filter(
    (item) =>
      item.example &&
      (declared.includes(item.endpoint) ||
        (templates && TEXT_PROTOCOLS.includes(item.endpoint))),
  ).map((item) => ({ ...item, declared: declared.includes(item.endpoint) }));
}

export function getModelEndpointRows(model, endpointMap = {}) {
  const declared = getDeclaredModelProtocols(model);
  const endpoints = [
    ...declared,
    ...getModelProtocolOptions(model, true)
      .filter((item) => !item.declared)
      .map((item) => item.endpoint),
  ];
  return endpoints.map((endpoint) => {
    const known = MODEL_PROTOCOLS.find((item) => item.endpoint === endpoint);
    const configured =
      endpointMap && Object.hasOwn(endpointMap, endpoint)
        ? endpointMap[endpoint]
        : null;
    const isDeclared = declared.includes(endpoint);
    // Templates use their own standard path; an unrelated model's custom mapping
    // cannot establish this model's support or change its example destination.
    const path =
      (isDeclared && typeof configured?.path === 'string' && configured.path) ||
      known?.path ||
      '';
    const method =
      (isDeclared &&
        typeof configured?.method === 'string' &&
        configured.method) ||
      'POST';
    return {
      endpoint,
      label: getModelProtocolLabel(endpoint),
      declared: isDeclared,
      method,
      path: path.replaceAll(
        '{model}',
        encodeURIComponent(model?.model_name || model?.modelName || ''),
      ),
    };
  });
}
