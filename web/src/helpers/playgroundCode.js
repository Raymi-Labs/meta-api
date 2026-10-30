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

import { pendingToolCalls } from './playgroundCapabilities';
import { MODEL_PROTOCOLS } from './modelProtocols';

export const PLAYGROUND_CODE_PROTOCOLS = [
  ['chat', 'openai'],
  ['messages', 'anthropic'],
  ['responses', 'openai-response'],
].map(([id, endpoint]) => ({
  id,
  ...MODEL_PROTOCOLS.find((item) => item.endpoint === endpoint),
}));

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

const defined = (value) => value !== undefined && value !== null;

export function convertPlaygroundPayload(payload, protocol = 'chat') {
  const errors = [];
  const notes = [];
  if (
    !payload?.model ||
    !Array.isArray(payload.messages) ||
    payload.messages.some(
      (message) =>
        !message ||
        typeof message !== 'object' ||
        (message.tool_calls && !Array.isArray(message.tool_calls)),
    )
  )
    return { body: null, errors: ['model / messages'], notes };
  const { group, ...source } = JSON.parse(JSON.stringify(payload));
  if (
    !source.messages.length ||
    source.messages.every((message) =>
      ['system', 'developer'].includes(message.role),
    )
  ) {
    source.messages.push({ role: 'user', content: 'Hello!' });
    notes.push('example');
  }
  if (pendingToolCalls(source.messages).length) errors.push('tool_results');
  if (protocol === 'chat') return { body: source, errors, notes };
  if (!['messages', 'responses'].includes(protocol))
    return { body: null, errors: ['protocol'], notes };

  const allowed = new Set([
    'model',
    'messages',
    'stream',
    'temperature',
    'top_p',
    'max_tokens',
    'max_completion_tokens',
    'tools',
    'tool_choice',
    'parallel_tool_calls',
  ]);
  if (protocol === 'responses') {
    allowed.add('reasoning_effort');
    allowed.add('response_format');
  } else allowed.add('stop');
  for (const [key, value] of Object.entries(source)) {
    // A zero penalty / single candidate has the same behavior in all three APIs.
    if (
      (['frequency_penalty', 'presence_penalty'].includes(key) &&
        value === 0) ||
      (key === 'n' && value === 1)
    )
      continue;
    if (!allowed.has(key) && defined(value)) errors.push(key);
  }
  const body = { model: source.model };
  for (const key of ['stream', 'temperature', 'top_p'])
    if (defined(source[key])) body[key] = source[key];
  const limit = source.max_completion_tokens ?? source.max_tokens;
  if (
    defined(source.max_completion_tokens) &&
    defined(source.max_tokens) &&
    source.max_tokens !== source.max_completion_tokens
  )
    errors.push('max_tokens / max_completion_tokens');
  if (protocol === 'messages') {
    body.max_tokens = limit ?? 4096;
    if (body.temperature > 1 || body.temperature < 0)
      errors.push('temperature (0–1)');
    if (!defined(limit)) notes.push('max_tokens');
    // top_p=1 is neutral; native Messages models may reject simultaneous controls.
    if (defined(body.temperature) && body.top_p === 1) delete body.top_p;
    if (defined(source.stop))
      body.stop_sequences = Array.isArray(source.stop)
        ? source.stop
        : [source.stop];
  } else {
    if (defined(limit)) body.max_output_tokens = limit;
    if (source.reasoning_effort)
      body.reasoning = { effort: source.reasoning_effort };
    if (source.response_format) {
      const format = source.response_format;
      if (format.type === 'json_schema' && format.json_schema)
        body.text = { format: { type: 'json_schema', ...format.json_schema } };
      else if (['text', 'json_object'].includes(format.type))
        body.text = { format: { type: format.type } };
      else errors.push('response_format');
    }
    if (defined(source.parallel_tool_calls))
      body.parallel_tool_calls = source.parallel_tool_calls;
  }

  if (source.tools) {
    if (!Array.isArray(source.tools)) errors.push('tools');
    else
      body.tools = source.tools.map((tool, index) => {
        const fn = tool?.function;
        for (const key of Object.keys(tool || {}))
          if (!['type', 'function'].includes(key))
            errors.push(`tools[${index}].${key}`);
        if (tool?.type !== 'function' || !fn?.name || !fn.parameters) {
          errors.push(`tools[${index}]`);
          return null;
        }
        for (const key of Object.keys(fn))
          if (!['name', 'description', 'parameters', 'strict'].includes(key))
            errors.push(`tools[${index}].${key}`);
        if (protocol === 'responses')
          return {
            type: 'function',
            name: fn.name,
            ...(defined(fn.description) ? { description: fn.description } : {}),
            parameters: fn.parameters,
            strict: fn.strict ?? false,
          };
        if (fn.strict) errors.push(`tools[${index}].strict`);
        return {
          name: fn.name,
          ...(defined(fn.description) ? { description: fn.description } : {}),
          input_schema: fn.parameters,
        };
      });
  }
  if (defined(source.tool_choice)) {
    const choice = source.tool_choice;
    if (
      typeof choice === 'string' &&
      ['auto', 'none', 'required'].includes(choice)
    )
      body.tool_choice =
        protocol === 'responses'
          ? choice
          : { type: choice === 'required' ? 'any' : choice };
    else if (choice?.type === 'function' && choice.function?.name)
      body.tool_choice =
        protocol === 'responses'
          ? { type: 'function', name: choice.function.name }
          : { type: 'tool', name: choice.function.name };
    else errors.push('tool_choice');
  }
  if (protocol === 'messages' && defined(source.parallel_tool_calls)) {
    body.tool_choice = {
      ...(body.tool_choice || { type: 'auto' }),
      disable_parallel_tool_use: !source.parallel_tool_calls,
    };
  }

  const contentParts = (content, role, path) => {
    if (typeof content === 'string') return [{ type: 'text', text: content }];
    if (content == null) return [];
    if (!Array.isArray(content)) {
      errors.push(path);
      return [];
    }
    return content
      .map((part, index) => {
        for (const key of Object.keys(part || {}))
          if (!['type', 'text', 'image_url'].includes(key))
            errors.push(`${path}[${index}].${key}`);
        if (part?.type === 'text' && typeof part.text === 'string')
          return { type: 'text', text: part.text };
        if (
          part?.type === 'image_url' &&
          part.image_url?.url &&
          role === 'user'
        ) {
          for (const key of Object.keys(part.image_url))
            if (!['url', 'detail'].includes(key))
              errors.push(`${path}[${index}].image_url.${key}`);
          if (protocol === 'responses')
            return {
              type: 'input_image',
              image_url: part.image_url.url,
              detail: part.image_url.detail || 'auto',
            };
          if (part.image_url.detail && part.image_url.detail !== 'auto')
            errors.push(`${path}[${index}].detail`);
          const data = part.image_url.url.match(
            /^data:(image\/(?:png|jpeg|gif|webp));base64,([\s\S]+)$/,
          );
          return {
            type: 'image',
            source: data
              ? { type: 'base64', media_type: data[1], data: data[2] }
              : { type: 'url', url: part.image_url.url },
          };
        }
        errors.push(`${path}[${index}].${part?.type || 'type'}`);
        return null;
      })
      .filter(Boolean);
  };
  const result = [];
  const system = [];
  const appendMessage = (role, content) => {
    if (!content.length) return;
    if (protocol === 'messages' && result.at(-1)?.role === role)
      result.at(-1).content.push(...content);
    else result.push({ role, content });
  };
  for (const [index, message] of source.messages.entries()) {
    const path = `messages[${index}]`;
    for (const key of Object.keys(message)) {
      if (
        !['role', 'content', 'tool_calls', 'tool_call_id'].includes(key) &&
        defined(message[key]) &&
        message[key] !== ''
      )
        errors.push(`${path}.${key}`);
    }
    if (message.role === 'tool') {
      if (!message.tool_call_id || typeof message.content !== 'string') {
        errors.push(path);
        continue;
      }
      if (protocol === 'responses')
        result.push({
          type: 'function_call_output',
          call_id: message.tool_call_id,
          output: message.content,
        });
      else
        appendMessage('user', [
          {
            type: 'tool_result',
            tool_use_id: message.tool_call_id,
            content: message.content,
          },
        ]);
      continue;
    }
    if (!['user', 'assistant', 'system', 'developer'].includes(message.role)) {
      errors.push(`${path}.role`);
      continue;
    }
    const parts = contentParts(
      message.content,
      message.role,
      `${path}.content`,
    );
    if (
      protocol === 'messages' &&
      ['system', 'developer'].includes(message.role)
    ) {
      if (result.length || parts.some((part) => part.type !== 'text'))
        errors.push(`${path}.role`);
      system.push(...parts);
      continue;
    }
    const textParts = parts.filter((part) => part.type !== 'text' || part.text);
    if (protocol === 'responses') {
      if (textParts.length) {
        if (textParts.every((part) => part.type === 'text'))
          result.push({
            role: message.role,
            content: textParts.map((part) => part.text).join('\n'),
          });
        else
          result.push({
            role: message.role,
            content: textParts.map((part) =>
              part.type === 'text' ? { ...part, type: 'input_text' } : part,
            ),
          });
      }
    }
    const nativeParts = [...textParts];
    for (const call of message.tool_calls || []) {
      if (
        message.role !== 'assistant' ||
        !call?.id ||
        call.type !== 'function' ||
        !call.function?.name
      ) {
        errors.push(`${path}.tool_calls`);
        continue;
      }
      if (protocol === 'responses')
        result.push({
          type: 'function_call',
          call_id: call.id,
          name: call.function.name,
          arguments: call.function.arguments,
        });
      else {
        try {
          const input = JSON.parse(call.function.arguments);
          if (!input || typeof input !== 'object' || Array.isArray(input))
            throw new Error('tool input');
          nativeParts.push({
            type: 'tool_use',
            id: call.id,
            name: call.function.name,
            input,
          });
        } catch {
          errors.push(`${path}.tool_calls.arguments`);
        }
      }
    }
    if (protocol === 'messages') appendMessage(message.role, nativeParts);
  }
  if (protocol === 'messages') {
    body.messages = result;
    if (system.length) body.system = system;
    if (result.at(-1)?.role === 'assistant') errors.push('assistant_prefill');
  } else body.input = result;
  return { body, errors: [...new Set(errors)], notes };
}

export function getPlaygroundSetup(language, protocol = 'chat') {
  const anthropic = protocol === 'messages';
  if (language === 'json') return '';
  const lines = [];
  if (language === 'python')
    lines.push(`python -m pip install ${anthropic ? 'anthropic' : 'openai'}`);
  if (language === 'javascript')
    lines.push(`npm install ${anthropic ? '@anthropic-ai/sdk' : 'openai'}`);
  lines.push('export META_API_KEY="YOUR_API_KEY"');
  if (language === 'python') lines.push('python playground.py');
  if (language === 'javascript') lines.push('node playground.mjs');
  return lines.join('\n');
}

export function buildPlaygroundCode(
  payload,
  language,
  address,
  protocol = 'chat',
) {
  const base = getPlaygroundEndpoint(address).replace(
    /\/chat\/completions$/,
    '',
  );
  const standard = PLAYGROUND_CODE_PROTOCOLS.find(
    (item) => item.id === protocol,
  );
  const converted = convertPlaygroundPayload(payload, protocol);
  if (!base || !standard || converted.errors.length || !converted.body)
    return '';
  const body = converted.body;
  const json = JSON.stringify(body, null, 2);
  const endpoint = base + standard.path.replace(/^\/v1/, '');
  if (language === 'json') return json;
  if (language === 'curl') {
    const url = "'" + endpoint.replaceAll("'", "'\\''") + "'";
    const auth =
      protocol === 'messages'
        ? [
            '  -H "x-api-key: $META_API_KEY"',
            '  -H "anthropic-version: 2023-06-01"',
          ]
        : ['  -H "Authorization: Bearer $META_API_KEY"'];
    const lines = [
      `curl --fail-with-body${body.stream ? ' --no-buffer' : ''} ${url}`,
      '  -H "Content-Type: application/json"',
      ...auth,
      "  --data-binary @- <<'META_API_JSON'",
    ];
    return lines.join(' \\\n') + `\n${json}\nMETA_API_JSON`;
  }
  const anthropic = protocol === 'messages';
  const client = anthropic ? 'Anthropic' : 'OpenAI';
  const sdkBase = anthropic ? base.replace(/\/v1$/, '') : base;
  const method = protocol === 'chat' ? 'chat.completions' : protocol;
  if (language === 'javascript')
    return `import ${client} from "${anthropic ? '@anthropic-ai/sdk' : 'openai'}";

const client = new ${client}({
  apiKey: process.env.META_API_KEY,
  baseURL: ${JSON.stringify(sdkBase)},${anthropic ? '\n  defaultHeaders: { "anthropic-version": "2023-06-01" },' : ''}
});

const response = await client.${method}.create(${json});
${body.stream ? 'for await (const event of response) {\n  console.log(JSON.stringify(event));\n}' : 'console.log(JSON.stringify(response, null, 2));'}`;

  // Required arguments and common SDK fields are explicit; custom Chat fields
  // travel through extra_body so older SDKs do not reject unknown keywords.
  const keys = new Set([
    'model',
    'stream',
    'temperature',
    'top_p',
    'tools',
    'tool_choice',
    ...(protocol === 'chat'
      ? [
          'messages',
          'max_tokens',
          'max_completion_tokens',
          'parallel_tool_calls',
          'reasoning_effort',
          'response_format',
          'frequency_penalty',
          'presence_penalty',
          'seed',
          'stop',
        ]
      : protocol === 'messages'
        ? ['messages', 'system', 'max_tokens', 'stop_sequences']
        : [
            'input',
            'max_output_tokens',
            'parallel_tool_calls',
            'reasoning',
            'text',
          ]),
  ]);
  const native = Object.fromEntries(
    Object.entries(body).filter(([key]) => keys.has(key)),
  );
  const extra = Object.fromEntries(
    Object.entries(body).filter(([key]) => !keys.has(key)),
  );
  if (Object.keys(extra).length) native.extra_body = extra;
  const argumentsCode = Object.entries(native)
    .map(([key, value]) => `    ${key}=${pythonLiteral(value, 1)},`)
    .join('\n');
  return `import os
from ${anthropic ? 'anthropic' : 'openai'} import ${client}

client = ${client}(
    api_key=os.environ["META_API_KEY"],
    base_url=${JSON.stringify(sdkBase)},${anthropic ? '\n    default_headers={"anthropic-version": "2023-06-01"},' : ''}
)

response = client.${method}.create(
${argumentsCode}
)
${body.stream ? 'for event in response:\n    print(event.model_dump_json())' : 'print(response.model_dump_json(indent=2))'}`;
}
