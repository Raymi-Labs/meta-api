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

export const DEFAULT_TOOL_DEFINITIONS = JSON.stringify(
  [
    {
      type: 'function',
      function: {
        name: 'get_current_time',
        description: 'Get the current time in a timezone.',
        parameters: {
          type: 'object',
          properties: {
            timezone: {
              type: 'string',
              description: 'IANA timezone, e.g. Asia/Shanghai',
            },
          },
          required: ['timezone'],
        },
      },
    },
  ],
  null,
  2,
);

export function parsePlaygroundTools(text) {
  try {
    const tools = JSON.parse(text || '[]');
    if (!Array.isArray(tools) || !tools.length || tools.length > 128)
      return null;
    const names = new Set();
    for (const tool of tools) {
      const fn = tool?.function;
      if (
        tool?.type !== 'function' ||
        !/^[a-zA-Z0-9_-]{1,64}$/.test(fn?.name || '') ||
        names.has(fn.name) ||
        !fn.parameters ||
        typeof fn.parameters !== 'object' ||
        Array.isArray(fn.parameters) ||
        fn.parameters.type !== 'object'
      )
        return null;
      names.add(fn.name);
    }
    return tools;
  } catch {
    return null;
  }
}

export function withModelCapabilities(inputs, model) {
  return {
    ...inputs,
    reasoningEffort:
      model?.capabilities?.reasoning === false ? '' : inputs.reasoningEffort,
    toolsEnabled:
      model?.capabilities?.function_calling === false
        ? false
        : inputs.toolsEnabled,
  };
}

// Tool streams can interleave multiple calls and split both names and arguments.
export function mergeToolCallDeltas(previous = [], deltas = []) {
  const next = previous.map((call) => ({
    ...call,
    function: { ...call.function },
  }));
  for (const delta of deltas) {
    if (!Number.isInteger(delta.index) || delta.index < 0 || delta.index >= 128)
      continue;
    const call = next[delta.index] || {
      id: '',
      type: 'function',
      function: { name: '', arguments: '' },
    };
    next[delta.index] = {
      ...call,
      id: delta.id || call.id,
      type: delta.type || call.type,
      function: {
        name: call.function.name + (delta.function?.name || ''),
        arguments: call.function.arguments + (delta.function?.arguments || ''),
      },
    };
  }
  return next;
}

export function pendingToolCalls(messages) {
  let pending = [];
  for (const message of messages) {
    if (
      ['loading', 'incomplete', 'error'].includes(message.status) ||
      message.toolCallsInterrupted
    )
      continue;
    if (message.role === 'assistant' && message.tool_calls?.length)
      pending = message.tool_calls.filter(Boolean);
    if (message.role === 'tool')
      pending = pending.filter((call) => call.id !== message.tool_call_id);
  }
  return pending;
}

export function toolCallsAreComplete(calls) {
  const ids = new Set();
  return (
    calls?.length > 0 &&
    Array.from(calls).every((call) => {
      if (!call?.id || !call.function?.name || ids.has(call.id)) return false;
      ids.add(call.id);
      try {
        const args = JSON.parse(call.function.arguments);
        return (
          args !== null && typeof args === 'object' && !Array.isArray(args)
        );
      } catch {
        return false;
      }
    })
  );
}
