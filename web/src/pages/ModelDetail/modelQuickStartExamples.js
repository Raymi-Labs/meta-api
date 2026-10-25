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

import { buildDocsRequest, getDocsBaseUrl } from '../Docs/runtimeDocs.js';

const PROTOCOLS = [
  ['openai', 'Chat Completions'],
  ['openai-response', 'Responses'],
  ['anthropic', 'Messages'],
  ['gemini', 'Gemini'],
  ['image-generation', 'Images'],
];

// Match the selected model's declared protocols. A model name alone never
// establishes which request formats its available channels support.
export function getModelQuickStartExamples(modelData, serverAddress, origin) {
  if (
    typeof modelData?.model_name !== 'string' ||
    !modelData.model_name.trim() ||
    !Array.isArray(modelData.supported_endpoint_types)
  ) {
    return [];
  }
  const baseUrl = getDocsBaseUrl(serverAddress, origin);
  if (!baseUrl) return [];

  return PROTOCOLS.filter(([endpoint]) =>
    modelData.supported_endpoint_types.includes(endpoint),
  ).map(([endpoint, label]) => {
    const command = buildDocsRequest(modelData, endpoint, baseUrl)
      .replace(/^```bash\n/, '')
      .replace(/\n```$/, '');
    return {
      endpoint,
      label,
      code: `export NAN_API_KEY="YOUR_API_KEY"\n\n${command}`,
    };
  });
}
