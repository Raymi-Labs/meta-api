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

import {
  formatMessageForAPI,
  isValidMessage,
  buildMessageContent,
  createMessage,
} from './playgroundUtils';
import { MESSAGE_ROLES } from '../constants/playground.constants';

// 构建API请求负载
export const buildApiPayload = (
  messages,
  systemPrompt,
  inputs,
  parameterEnabled,
) => {
  const processedMessages = messages
    .filter((message) => !['loading', 'error'].includes(message.status))
    .filter(isValidMessage)
    .map(formatMessageForAPI)
    .filter(Boolean);

  systemPrompt = systemPrompt ?? inputs.systemPrompt;

  // 如果有系统提示，插入到消息开头
  if (systemPrompt && systemPrompt.trim()) {
    processedMessages.unshift({
      role: MESSAGE_ROLES.SYSTEM,
      content: systemPrompt.trim(),
    });
  }

  const payload = {
    model: inputs.model,
    group: inputs.group,
    messages: processedMessages,
    stream: inputs.stream,
  };

  // 添加启用的参数
  const parameterMappings = {
    temperature: 'temperature',
    top_p: 'top_p',
    max_tokens: 'max_tokens',
    frequency_penalty: 'frequency_penalty',
    presence_penalty: 'presence_penalty',
    seed: 'seed',
  };

  Object.entries(parameterMappings).forEach(([key, param]) => {
    const enabled = parameterEnabled[key];
    const value = inputs[param];
    const hasValue = value !== undefined && value !== null;

    if (enabled && hasValue) {
      payload[param] = value;
    }
  });

  return payload;
};

export function buildPlaygroundPreview(
  messages,
  draft,
  inputs,
  parameterEnabled,
) {
  const previewMessages = [...messages];
  if (draft.trim()) {
    previewMessages.push(
      createMessage(
        'user',
        buildMessageContent(
          draft,
          (inputs.imageUrls || []).filter((url) => url.trim()),
          inputs.imageEnabled,
        ),
      ),
    );
  }
  return buildApiPayload(previewMessages, null, inputs, parameterEnabled);
}

export function reconcileModelTabs(tabs, activeModel, models) {
  const available = new Set(models.map((model) => model.value));
  const saved = Array.isArray(tabs)
    ? tabs.filter((model) => available.has(model))
    : [];
  if (available.has(activeModel)) saved.push(activeModel);
  if (saved.length === 0) return models.slice(0, 3).map((model) => model.value);
  return [...new Set(saved)];
}

export function getPlaygroundEndpoint(address) {
  try {
    const url = new URL(address);
    if (!['https:', 'http:'].includes(url.protocol)) return '';
    url.username = '';
    url.password = '';
    url.search = '';
    url.hash = '';
    url.pathname =
      url.pathname.replace(/\/+$/, '').replace(/\/v1$/, '') +
      '/v1/chat/completions';
    return url.toString();
  } catch {
    return '';
  }
}

function pythonLiteral(value, depth = 0) {
  if (value === null) return 'None';
  if (typeof value === 'boolean') return value ? 'True' : 'False';
  if (typeof value !== 'object') return JSON.stringify(value);
  const array = Array.isArray(value);
  const entries = array ? value : Object.entries(value);
  const [open, close] = array ? ['[', ']'] : ['{', '}'];
  if (!entries.length) return open + close;
  const lines = entries.map(
    (entry) =>
      '    '.repeat(depth + 1) +
      (array
        ? pythonLiteral(entry, depth + 1)
        : `${JSON.stringify(entry[0])}: ${pythonLiteral(entry[1], depth + 1)}`),
  );
  return `${open}\n${lines.join(',\n')},\n${'    '.repeat(depth)}${close}`;
}

export function buildPlaygroundCode(payload, language, address) {
  const endpoint = getPlaygroundEndpoint(address);
  if (!payload || !endpoint) return '';
  // Public API keys choose their own group. The cookie-authenticated /pg field
  // is deliberately not exported to /v1, where it cannot select a token group.
  const { group, ...body } = payload;
  const json = JSON.stringify(body, null, 2);
  if (language === 'json') return json;
  if (language === 'curl') {
    const shellEndpoint = "'" + endpoint.replaceAll("'", "'\\''") + "'";
    const command = [
      `curl ${shellEndpoint}`,
      '  -H "Content-Type: application/json"',
      '  -H "Authorization: Bearer $META_API_KEY"',
      "  --data-binary @- <<'META_API_JSON'",
    ].join(' \\\n');
    return `${command}
${json}
META_API_JSON`;
  }
  if (language === 'javascript') {
    return `const response = await fetch(${JSON.stringify(endpoint)}, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: \`Bearer \${process.env.META_API_KEY}\`,
  },
  body: JSON.stringify(${json}),
});
if (!response.ok) throw new Error(await response.text());
${body.stream !== false ? 'for await (const chunk of response.body) {\n  process.stdout.write(chunk);\n}' : 'console.log(await response.json());'}`;
  }
  return `import json
import os
from urllib.request import Request, urlopen

payload = ${pythonLiteral(JSON.parse(json))}
request = Request(
    ${JSON.stringify(endpoint)},
    data=json.dumps(payload).encode("utf-8"),
    headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer " + os.environ["META_API_KEY"],
    },
    method="POST",
)
with urlopen(request) as response:
${body.stream !== false ? '    for line in response:\n        print(line.decode("utf-8"), end="")' : '    print(json.load(response))'}`;
}
