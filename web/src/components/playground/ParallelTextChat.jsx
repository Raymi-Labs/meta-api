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
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Select, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useApiRequest } from '../../hooks/playground/useApiRequest';
import { buildApiPayload } from '../../helpers/playgroundRequest';
import {
  createMessage,
  createLoadingAssistantMessage,
} from '../../helpers/playgroundUtils';
import { withModelCapabilities } from '../../helpers/playgroundCapabilities';
import {
  canUseParallelReply,
  isPlainConversation,
  isTextComparisonModel,
  MAX_PARALLEL_MODELS,
  sharedConversation,
  updateParallelReply,
} from '../../helpers/playgroundParallel';
import { getPlaygroundOwner } from './configStorage';
import MessageContent from './MessageContent';
import './parallelTextChat.css';

const noop = () => {};

function ReplyRequest({ task, owner, setMessages, controls }) {
  const connection = useRef(null);
  const alive = useRef(false);
  const update = useCallback(
    (change) => {
      if (!alive.current || getPlaygroundOwner() !== owner) return;
      setMessages((previous) => updateParallelReply(previous, task.id, change));
    },
    [owner, setMessages, task.id],
  );
  const { sendRequest, onStopGenerator } = useApiRequest(
    update,
    noop,
    noop,
    connection,
    noop,
  );
  const handlers = useRef({ sendRequest, onStopGenerator });
  handlers.current = { sendRequest, onStopGenerator };
  useEffect(() => {
    alive.current = true;
    let cancelled = false;
    const stop = () => {
      setMessages((items) =>
        items.map((m) =>
          m.id === task.id && ['loading', 'incomplete'].includes(m.status)
            ? { ...m, parallelStopped: true, status: 'complete' }
            : m,
        ),
      );
      handlers.current.onStopGenerator();
    };
    controls.current.set(task.id, stop);
    // StrictMode's discarded effect must never send a billable request.
    queueMicrotask(() => {
      if (!cancelled && alive.current && getPlaygroundOwner() === owner)
        handlers.current.sendRequest(
          task.payload,
          task.payload.stream !== false,
        );
    });
    return () => {
      cancelled = true;
      alive.current = false;
      const source = connection.current;
      connection.current = null;
      source?.close?.();
      controls.current.delete(task.id);
    };
  }, [task, owner, controls, setMessages]);
  return null;
}

export default function ParallelTextChat({
  owner,
  messages,
  setMessages,
  saveMessages,
  models,
  catalog,
  inputs,
  parameterEnabled,
  disabled,
  draft,
  setDraft,
  isMobile,
}) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState([]);
  const [tasks, setTasks] = useState([]);
  const controls = useRef(new Map());
  const snapshots = useRef(new Map());
  const sending = useRef(false);
  const messageRef = useRef(messages);
  messageRef.current = messages;
  const eligible = models.filter((m) =>
    isTextComparisonModel(
      catalog?.models.find((item) => item.model_name === m.value),
    ),
  );
  const chosenModels = selected.filter((name) =>
    eligible.some((m) => m.value === name),
  );
  const generating = messages.some((m) =>
    ['loading', 'incomplete'].includes(m.status),
  );
  const plain =
    isPlainConversation(messages) &&
    !inputs.imageEnabled &&
    !inputs.toolsEnabled;
  const latestRound = messages[messages.length - 1]?.parallelRound;
  let contextReady = true;
  try {
    sharedConversation(messages);
  } catch {
    contextReady = false;
  }
  const canSend =
    !disabled &&
    plain &&
    contextReady &&
    chosenModels.length >= 2 &&
    chosenModels.length <= MAX_PARALLEL_MODELS;

  useEffect(() => {
    if (!generating) {
      sending.current = false;
      saveMessages(messages);
    }
  }, [generating, messages, saveMessages]);

  const send = () => {
    if (
      sending.current ||
      generating ||
      !canSend ||
      !draft.trim() ||
      getPlaygroundOwner() !== owner
    )
      return;
    const userMessage = createMessage('user', draft);
    const history = [...sharedConversation(messageRef.current), userMessage];
    const round = userMessage.id;
    const answers = chosenModels.map((model) => ({
      ...createLoadingAssistantMessage(),
      parallelRound: round,
      parallelModel: model,
      parallelGroup: inputs.group,
    }));
    const nextTasks = answers.map((answer) => {
      const model = catalog.models.find(
        (m) => m.model_name === answer.parallelModel,
      );
      const payload = buildApiPayload(
        history,
        null,
        withModelCapabilities(
          {
            ...inputs,
            model: answer.parallelModel,
            toolsEnabled: false,
            imageEnabled: false,
          },
          model,
        ),
        parameterEnabled,
      );
      return { id: answer.id, payload: JSON.parse(JSON.stringify(payload)) };
    });
    sending.current = true;
    snapshots.current = new Map(
      nextTasks.map((task) => [task.id, task.payload]),
    );
    const next = [...messageRef.current, userMessage, ...answers];
    setMessages(next);
    saveMessages(next);
    setTasks(nextTasks);
    setDraft('');
  };
  const retry = (reply) => {
    if (
      sending.current ||
      generating ||
      disabled ||
      getPlaygroundOwner() !== owner ||
      reply.parallelRound !== latestRound
    )
      return;
    const payload = snapshots.current.get(reply.id);
    if (
      !payload ||
      payload.group !== inputs.group ||
      !eligible.some((m) => m.value === payload.model)
    )
      return;
    sending.current = true;
    const replacement = {
      ...createLoadingAssistantMessage(),
      parallelRound: reply.parallelRound,
      parallelModel: reply.parallelModel,
      parallelGroup: reply.parallelGroup,
    };
    snapshots.current.set(replacement.id, payload);
    setMessages((items) =>
      items.map((m) => (m.id === reply.id ? replacement : m)),
    );
    setTasks((items) => [
      ...items.filter((task) => task.id !== reply.id),
      { id: replacement.id, payload: JSON.parse(JSON.stringify(payload)) },
    ]);
  };
  const choose = (reply) => {
    if (
      generating ||
      !canUseParallelReply(reply) ||
      reply.parallelRound !== latestRound
    )
      return;
    setMessages((items) =>
      items.map((m) =>
        m.parallelRound === reply.parallelRound
          ? { ...m, parallelChosen: m.id === reply.id }
          : m,
      ),
    );
  };
  const copy = async (text) => {
    try {
      await navigator.clipboard.writeText(
        typeof text === 'string' ? text : JSON.stringify(text),
      );
    } catch {
      Toast.error(t('复制失败'));
    }
  };
  return (
    <section className='pg-parallel' aria-label={t('多模型同时回答')}>
      {tasks.map((task) => (
        <ReplyRequest
          key={task.id}
          task={task}
          owner={owner}
          setMessages={setMessages}
          controls={controls}
        />
      ))}
      <div className='pg-parallel-selection'>
        <Select
          multiple
          maxTagCount={MAX_PARALLEL_MODELS}
          aria-label={t('选择文本模型（最多5个）')}
          placeholder={t('选择文本模型（最多5个）')}
          value={chosenModels}
          disabled={generating || disabled}
          filter
          optionList={eligible.map((m) => ({
            label: m.value,
            value: m.value,
            disabled:
              chosenModels.length >= MAX_PARALLEL_MODELS &&
              !chosenModels.includes(m.value),
          }))}
          onChange={(names) => {
            if (names.length <= MAX_PARALLEL_MODELS) setSelected(names);
          }}
          style={{ width: '100%' }}
        />
        <p>{t('每个模型独立调用和计费；同轮收到相同上下文。')}</p>
        {!plain && (
          <p role='alert'>
            {t(
              '多模型模式仅支持纯文本，请关闭图片和工具；已有多媒体或工具历史需新开对话。',
            )}
          </p>
        )}
        {!contextReady && (
          <p role='status'>
            {t('请选择一条完整回答作为后续上下文，或撤销本轮。')}
          </p>
        )}
        {!eligible.length && <p>{t('暂无可用模型')}</p>}
      </div>
      <div className='pg-parallel-messages'>
        {messages.map((reply) => (
          <article
            key={reply.id}
            className={`pg-parallel-reply ${reply.parallelChosen ? 'is-chosen' : ''}`}
            data-message-id={reply.id}
          >
            <header>
              <strong>
                {reply.parallelModel ||
                  (reply.role === 'user' ? t('用户') : t('回复'))}
              </strong>
              {reply.parallelModel && (
                <span>{reply.parallelGroup || t('默认分组')}</span>
              )}
            </header>
            {reply.role === 'user' ? (
              <p className='pg-parallel-user-text'>
                {typeof reply.content === 'string'
                  ? reply.content
                  : reply.content
                      .filter((part) => part.type === 'text')
                      .map((part) => part.text)
                      .join('\n')}
              </p>
            ) : reply.status === 'error' ? (
              <p role='alert'>{reply.content || t('请求发生错误')}</p>
            ) : (
              <MessageContent
                message={reply}
                styleState={{ isMobile }}
                onToggleReasoningExpansion={(id) =>
                  setMessages((items) =>
                    items.map((m) =>
                      m.id === id
                        ? { ...m, isReasoningExpanded: !m.isReasoningExpanded }
                        : m,
                    ),
                  )
                }
              />
            )}
            <footer>
              <Button size='small' onClick={() => copy(reply.content)}>
                {t('复制')}
              </Button>
              {reply.parallelRound &&
                ['loading', 'incomplete'].includes(reply.status) && (
                  <Button
                    size='small'
                    onClick={() => controls.current.get(reply.id)?.()}
                  >
                    {t('停止生成')}
                  </Button>
                )}
              {reply.parallelStopped && <span>{t('已停止')}</span>}
              {reply.parallelRound === latestRound &&
                canUseParallelReply(reply) && (
                  <Button
                    size='small'
                    disabled={generating}
                    theme={reply.parallelChosen ? 'solid' : 'light'}
                    onClick={() => choose(reply)}
                  >
                    {t(reply.parallelChosen ? '已选为上下文' : '用这条继续')}
                  </Button>
                )}
              {reply.parallelRound === latestRound &&
                (reply.status === 'error' || reply.parallelStopped) &&
                snapshots.current.has(reply.id) && (
                  <Button
                    size='small'
                    disabled={
                      generating ||
                      disabled ||
                      inputs.group !== snapshots.current.get(reply.id).group ||
                      !eligible.some((m) => m.value === reply.parallelModel)
                    }
                    onClick={() => retry(reply)}
                  >
                    {t('重试')}
                  </Button>
                )}
            </footer>
          </article>
        ))}
      </div>
      <div className='pg-parallel-composer'>
        <textarea
          aria-label={t('请输入您的问题...')}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (
              (e.metaKey || e.ctrlKey) &&
              e.key === 'Enter' &&
              !e.nativeEvent.isComposing
            ) {
              e.preventDefault();
              send();
            }
          }}
        />
        <div>
          <Button
            disabled={generating}
            onClick={() => {
              setTasks([]);
              snapshots.current.clear();
              setMessages([]);
              saveMessages([]);
            }}
          >
            {t('清空对话')}
          </Button>
          {latestRound && (
            <Button
              disabled={generating}
              onClick={() => {
                const index = messages.findIndex((m) => m.id === latestRound);
                if (index >= 0) {
                  setTasks([]);
                  snapshots.current.clear();
                  setMessages(messages.slice(0, index));
                }
              }}
            >
              {t('撤销本轮')}
            </Button>
          )}
          {generating ? (
            <Button onClick={() => controls.current.forEach((stop) => stop())}>
              {t('停止生成')}
            </Button>
          ) : (
            <Button
              theme='solid'
              disabled={!canSend || !draft.trim()}
              onClick={send}
            >
              {t('同时发送')}
            </Button>
          )}
        </div>
      </div>
    </section>
  );
}
