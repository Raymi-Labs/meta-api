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

export const ENTERPRISE_CONTACT = 'nanashiwang@gmail.com';

export const ENTERPRISE_PROTOCOLS = [
  { id: 'chat', name: 'Chat Completions', endpoint: '/chat/completions' },
  { id: 'messages', name: 'Messages', endpoint: '/messages' },
  { id: 'responses', name: 'Responses', endpoint: '/responses' },
];

// Examples are text only. This page never sends requests to a model endpoint.
export function createEnterpriseExample(protocol, language, baseUrl) {
  const selected =
    ENTERPRISE_PROTOCOLS.find((item) => item.id === protocol) ||
    ENTERPRISE_PROTOCOLS[0];
  const payload =
    selected.id === 'responses'
      ? { model: 'MODEL_ID', input: 'Hello!' }
      : {
          model: 'MODEL_ID',
          ...(selected.id === 'messages' ? { max_tokens: 1024 } : {}),
          messages: [{ role: 'user', content: 'Hello!' }],
        };

  if (language === 'cURL') {
    const authentication =
      selected.id === 'messages'
        ? '  -H "x-api-key: $API_KEY" \\\n  -H "anthropic-version: 2023-06-01"'
        : '  -H "Authorization: Bearer $API_KEY"';
    return `curl "${baseUrl}${selected.endpoint}" \\
${authentication} \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(payload, null, 2)}'`;
  }

  if (selected.id === 'messages') {
    // The Anthropic SDK appends /v1/messages to its base URL itself.
    const server = baseUrl.replace(/\/v1$/, '');
    return `import os
from anthropic import Anthropic

client = Anthropic(
    base_url="${server}",
    api_key=os.environ["API_KEY"],
)

response = client.messages.create(
    model="MODEL_ID",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.content[0].text)`;
  }

  const request =
    selected.id === 'responses'
      ? `response = client.responses.create(
    model="MODEL_ID",
    input="Hello!",
)
print(response.output_text)`
      : `response = client.chat.completions.create(
    model="MODEL_ID",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`;

  return `import os
from openai import OpenAI

client = OpenAI(
    base_url="${baseUrl}",
    api_key=os.environ["API_KEY"],
)

${request}`;
}
