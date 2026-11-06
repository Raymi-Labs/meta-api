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
import { DEFAULT_CONFIG } from '../constants/playground.constants';

export const SESSION_LIMIT = 100;
export const BACKUP_LIMIT = 20 * 1024 * 1024;
const clone = (value) => JSON.parse(JSON.stringify(value));
const id = () => crypto.randomUUID();

export function newPlaygroundSession(config = DEFAULT_CONFIG) {
  return {
    id: id(),
    title: '',
    updatedAt: Date.now(),
    messages: [],
    draft: '',
    config: {
      ...clone(DEFAULT_CONFIG),
      inputs: {
        ...clone(DEFAULT_CONFIG.inputs),
        ...clone(config.inputs || {}),
        systemPrompt: '',
        imageEnabled: false,
        imageUrls: [''],
        toolsEnabled: false,
        toolsJson: '',
      },
      parameterEnabled: clone(
        config.parameterEnabled || DEFAULT_CONFIG.parameterEnabled,
      ),
    },
    parallelMode: false,
    parallelModels: [],
  };
}

export function recoverSession(session) {
  return {
    ...session,
    messages: session.messages.map((message) =>
      ['loading', 'incomplete'].includes(message.status)
        ? {
            ...message,
            status: 'error',
            isThinkingComplete: true,
            localInterrupted: true,
            parallelStopped: !!message.parallelRound,
            toolCallsInterrupted: !!message.tool_calls?.length,
          }
        : message,
    ),
  };
}

export function sessionTitle(session, fallback) {
  const first = session.messages.find(
    (m) => m.role === 'user' && typeof m.content === 'string',
  );
  return session.title || first?.content.slice(0, 60) || fallback;
}

// Backups are data, never instructions or requests. Only known fields survive.
export function parseSessionBackup(text) {
  if (new Blob([text]).size > BACKUP_LIMIT) throw new Error('对话备份过大');
  const raw = JSON.parse(text, (key, value) => {
    if (['__proto__', 'constructor', 'prototype'].includes(key))
      throw new Error('对话备份格式无效');
    return value;
  });
  if (
    raw?.format !== 'meta-api-local-conversations' ||
    raw.version !== 1 ||
    !Array.isArray(raw.sessions) ||
    !raw.sessions.length ||
    raw.sessions.length > SESSION_LIMIT
  )
    throw new Error('对话备份格式无效');
  return raw.sessions.map((s) => {
    if (
      !s ||
      typeof s !== 'object' ||
      !Array.isArray(s.messages) ||
      s.messages.length > 10000 ||
      !s.config ||
      typeof s.config.inputs !== 'object' ||
      !s.config.inputs ||
      Array.isArray(s.config.inputs)
    )
      throw new Error('对话备份格式无效');
    for (const m of s.messages) {
      if (
        !m ||
        !['user', 'assistant', 'system', 'tool'].includes(m.role) ||
        !(
          typeof m.content === 'string' ||
          Array.isArray(m.content) ||
          m.content == null
        ) ||
        (m.tool_calls != null && !Array.isArray(m.tool_calls))
      )
        throw new Error('对话备份格式无效');
      if (m.id != null && typeof m.id !== 'string')
        throw new Error('对话备份格式无效');
      if (Array.isArray(m.content)) {
        for (const block of m.content) {
          if (
            !block ||
            typeof block !== 'object' ||
            !(
              (block.type === 'text' && typeof block.text === 'string') ||
              (block.type === 'image_url' &&
                typeof block.image_url?.url === 'string')
            )
          )
            throw new Error('对话备份格式无效');
        }
      }
      for (const call of m.tool_calls || []) {
        if (
          !call ||
          typeof call.id !== 'string' ||
          call.type !== 'function' ||
          typeof call.function?.name !== 'string' ||
          typeof call.function?.arguments !== 'string'
        )
          throw new Error('对话备份格式无效');
      }
    }
    const config = clone(DEFAULT_CONFIG);
    for (const k of Object.keys(config.inputs)) {
      const v = s.config.inputs[k];
      if (v === undefined) continue;
      const sample = config.inputs[k];
      if (
        sample === null
          ? !(v === null || typeof v === 'number')
          : Array.isArray(sample)
            ? !Array.isArray(v) || v.some((x) => typeof x !== 'string')
            : typeof v !== typeof sample
      )
        throw new Error('对话备份格式无效');
      config.inputs[k] = v;
    }
    for (const k of Object.keys(config.parameterEnabled)) {
      if (typeof s.config.parameterEnabled?.[k] === 'boolean')
        config.parameterEnabled[k] = s.config.parameterEnabled[k];
    }
    // Custom JSON may embed a prior conversation; never activate it on import.
    return recoverSession({
      id: id(),
      title: typeof s.title === 'string' ? s.title.slice(0, 80) : '',
      updatedAt: Date.now(),
      config,
      messages: clone(s.messages),
      draft: typeof s.draft === 'string' ? s.draft : '',
      parallelMode: s.parallelMode === true,
      parallelModels: Array.isArray(s.parallelModels)
        ? s.parallelModels.filter((x) => typeof x === 'string').slice(0, 5)
        : [],
    });
  });
}

export function sessionBackup(sessions) {
  return JSON.stringify(
    { format: 'meta-api-local-conversations', version: 1, sessions },
    null,
    2,
  );
}

export function createConversationGate() {
  let current = null;
  return {
    acquire(session) {
      if (current && current !== session) return false;
      current = session;
      return true;
    },
    release(session) {
      if (current === session) current = null;
    },
    get current() {
      return current;
    },
  };
}
