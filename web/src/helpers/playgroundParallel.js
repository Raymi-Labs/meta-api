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
export const MAX_PARALLEL_MODELS = 5;

export function recoverParallelReplies(messages) {
  return messages?.map((item) =>
    item.parallelRound && ['loading', 'incomplete'].includes(item.status)
      ? {
          ...item,
          status: 'complete',
          parallelStopped: true,
          parallelChosen: false,
        }
      : item,
  );
}

export function isTextComparisonModel(model) {
  if (!model || !Array.isArray(model.supported_endpoint_types)) return false;
  const endpoints = model.supported_endpoint_types;
  if (
    !endpoints.some((p) =>
      ['openai', 'openai-response', 'anthropic', 'gemini'].includes(p),
    )
  )
    return false;
  if (endpoints.some((p) => /image|video|audio|embed|rerank/i.test(p)))
    return false;
  if (
    model.output_modalities?.length &&
    !model.output_modalities.includes('text')
  )
    return false;
  return true;
}

export function isPlainConversation(messages) {
  return messages.every(
    (m) =>
      ['user', 'assistant', 'system'].includes(m.role) &&
      !m.tool_calls?.length &&
      !m.toolCallsInterrupted &&
      (typeof m.content === 'string' ||
        (Array.isArray(m.content) &&
          m.content.every((c) => c.type === 'text'))),
  );
}

export function canUseParallelReply(message) {
  return (
    message.status === 'complete' &&
    !['length', 'content_filter'].includes(message.finishReason) &&
    !message.parallelStopped &&
    !message.tool_calls?.length &&
    typeof message.content === 'string' &&
    Boolean(message.content.trim())
  );
}

export function sharedConversation(messages) {
  const rounds = new Set(
    messages.filter((m) => m.parallelRound).map((m) => m.parallelRound),
  );
  for (const round of rounds) {
    const chosen = messages.filter(
      (m) =>
        m.parallelRound === round && m.parallelChosen && canUseParallelReply(m),
    );
    if (chosen.length !== 1) throw new Error('parallel reply must be selected');
  }
  return messages.filter(
    (m) => !m.parallelRound || (m.parallelChosen && canUseParallelReply(m)),
  );
}

// Existing request handlers update their last message. Give each one a private
// one-message view and merge only that ID back into the shared conversation.
export function updateParallelReply(messages, id, update) {
  const current = messages.find((m) => m.id === id);
  if (!current || current.parallelStopped) return messages;
  const result = typeof update === 'function' ? update([current]) : update;
  const replacement = result?.find((m) => m.id === id);
  if (!replacement)
    return messages.map((m) =>
      m.id === id ? { ...m, status: 'complete', parallelStopped: true } : m,
    );
  return messages.map((m) =>
    m.id === id
      ? {
          ...replacement,
          parallelRound: current.parallelRound,
          parallelModel: current.parallelModel,
          parallelGroup: current.parallelGroup,
        }
      : m,
  );
}
