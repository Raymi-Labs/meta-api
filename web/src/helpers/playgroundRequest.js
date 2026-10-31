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

import { parsePlaygroundTools } from './playgroundCapabilities';
import { sharedConversation } from './playgroundParallel';

// 构建API请求负载
export const buildApiPayload = (
  messages,
  systemPrompt,
  inputs,
  parameterEnabled,
) => {
  const processedMessages = sharedConversation(messages)
    .filter(
      (message) => !['loading', 'incomplete', 'error'].includes(message.status),
    )
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

  if (inputs.reasoningEffort) payload.reasoning_effort = inputs.reasoningEffort;
  if (inputs.toolsEnabled) {
    const tools = parsePlaygroundTools(inputs.toolsJson);
    if (!tools) throw new Error('invalid tools');
    payload.tools = tools;
    const choice = inputs.toolChoice || 'auto';
    if (['auto', 'required', 'none'].includes(choice))
      payload.tool_choice = choice;
    else if (tools.some((tool) => tool.function.name === choice))
      payload.tool_choice = { type: 'function', function: { name: choice } };
    else throw new Error('invalid tool choice');
  }
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

export { getPlaygroundEndpoint, buildPlaygroundCode } from './playgroundCode';
