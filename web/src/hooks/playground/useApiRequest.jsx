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

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { SSE } from 'sse.js';
import {
  API_ENDPOINTS,
  MESSAGE_STATUS,
  DEBUG_TABS,
} from '../../constants/playground.constants';
import {
  getUserIdFromLocalStorage,
  handleApiError,
  extractPlaygroundErrorMessage,
  parsePlaygroundSSEItem,
  processThinkTags,
  processIncompleteThinkTags,
} from '../../helpers';

export const useApiRequest = (
  setMessage,
  setDebugData,
  setActiveDebugTab,
  sseSourceRef,
  saveMessages,
) => {
  const { t } = useTranslation();

  // 处理消息自动关闭逻辑的公共函数
  const applyAutoCollapseLogic = useCallback(
    (message, isThinkingComplete = true) => {
      const shouldAutoCollapse =
        isThinkingComplete && !message.hasAutoCollapsed;
      return {
        isThinkingComplete,
        hasAutoCollapsed: shouldAutoCollapse || message.hasAutoCollapsed,
        isReasoningExpanded: shouldAutoCollapse
          ? false
          : message.isReasoningExpanded,
      };
    },
    [],
  );

  // 流式消息更新
  const streamMessageUpdate = useCallback(
    (textChunk, type) => {
      setMessage((prevMessage) => {
        const lastMessage = prevMessage[prevMessage.length - 1];
        if (!lastMessage) return prevMessage;
        if (lastMessage.role !== 'assistant') return prevMessage;
        if (lastMessage.status === MESSAGE_STATUS.ERROR) {
          return prevMessage;
        }

        if (
          lastMessage.status === MESSAGE_STATUS.LOADING ||
          lastMessage.status === MESSAGE_STATUS.INCOMPLETE
        ) {
          let newMessage = { ...lastMessage };

          if (type === 'reasoning') {
            newMessage = {
              ...newMessage,
              reasoningContent:
                (lastMessage.reasoningContent || '') + textChunk,
              status: MESSAGE_STATUS.INCOMPLETE,
              isThinkingComplete: false,
            };
          } else if (type === 'content') {
            const shouldCollapseReasoning =
              !lastMessage.content && lastMessage.reasoningContent;
            const newContent = (lastMessage.content || '') + textChunk;

            let shouldCollapseFromThinkTag = false;
            let thinkingCompleteFromTags = lastMessage.isThinkingComplete;

            if (
              lastMessage.isReasoningExpanded &&
              newContent.includes('</think>')
            ) {
              const thinkMatches = newContent.match(/<think>/g);
              const thinkCloseMatches = newContent.match(/<\/think>/g);
              if (
                thinkMatches &&
                thinkCloseMatches &&
                thinkCloseMatches.length >= thinkMatches.length
              ) {
                shouldCollapseFromThinkTag = true;
                thinkingCompleteFromTags = true; // think标签闭合也标记思考完成
              }
            }

            // 如果开始接收content内容，且之前有reasoning内容，或者think标签已闭合，则标记思考完成
            const isThinkingComplete =
              (lastMessage.reasoningContent &&
                !lastMessage.isThinkingComplete) ||
              thinkingCompleteFromTags;

            const autoCollapseState = applyAutoCollapseLogic(
              lastMessage,
              isThinkingComplete,
            );

            newMessage = {
              ...newMessage,
              content: newContent,
              status: MESSAGE_STATUS.INCOMPLETE,
              ...autoCollapseState,
            };
          }

          return [...prevMessage.slice(0, -1), newMessage];
        }

        return prevMessage;
      });
    },
    [setMessage, applyAutoCollapseLogic],
  );

  // 完成消息
  const completeMessage = useCallback(
    (status = MESSAGE_STATUS.COMPLETE) => {
      setMessage((prevMessage) => {
        const lastMessage = prevMessage[prevMessage.length - 1];
        if (!lastMessage) return prevMessage;
        if (
          lastMessage.status === MESSAGE_STATUS.COMPLETE ||
          lastMessage.status === MESSAGE_STATUS.ERROR
        ) {
          return prevMessage;
        }

        const autoCollapseState = applyAutoCollapseLogic(lastMessage, true);

        const updatedMessages = [
          ...prevMessage.slice(0, -1),
          {
            ...lastMessage,
            status: status,
            ...autoCollapseState,
          },
        ];

        // 在消息完成时保存，传入更新后的消息列表
        if (
          status === MESSAGE_STATUS.COMPLETE ||
          status === MESSAGE_STATUS.ERROR
        ) {
          setTimeout(() => saveMessages(updatedMessages), 0);
        }

        return updatedMessages;
      });
    },
    [setMessage, applyAutoCollapseLogic, saveMessages],
  );

  // 非流式请求
  const handleNonStreamRequest = useCallback(
    async (payload) => {
      const controller = new AbortController();
      controller.close = () => controller.abort();
      sseSourceRef.current = controller;
      const isCurrent = () =>
        sseSourceRef.current === controller && !controller.signal.aborted;
      setDebugData((prev) => ({
        ...prev,
        request: payload,
        timestamp: new Date().toISOString(),
        response: null,
        sseMessages: null, // 非流式请求清除 SSE 消息
        isStreaming: false,
      }));
      setActiveDebugTab(DEBUG_TABS.REQUEST);

      try {
        const response = await fetch(API_ENDPOINTS.CHAT_COMPLETIONS, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'New-Api-User': getUserIdFromLocalStorage(),
          },
          body: JSON.stringify(payload),
          signal: controller.signal,
        });
        if (!isCurrent()) return;

        if (!response.ok) {
          let errorBody = '';
          try {
            errorBody = await response.text();
          } catch (e) {
            errorBody = '无法读取错误响应体';
          }
          if (!isCurrent()) return;

          const errorInfo = handleApiError(
            new Error(
              `HTTP error! status: ${response.status}, body: ${errorBody}`,
            ),
            response,
          );

          setDebugData((prev) => ({
            ...prev,
            response: JSON.stringify(errorInfo, null, 2),
          }));
          setActiveDebugTab(DEBUG_TABS.RESPONSE);

          throw new Error(
            `HTTP error! status: ${response.status}, body: ${errorBody}`,
          );
        }

        const data = await response.json();
        if (!isCurrent()) return;
        if (!data.choices?.[0]?.message)
          throw new Error(t('解析响应数据时发生错误'));

        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(data, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        if (data.choices?.[0]) {
          const choice = data.choices[0];
          let content = choice.message?.content || '';
          let reasoningContent =
            choice.message?.reasoning_content ||
            choice.message?.reasoning ||
            '';

          const processed = processThinkTags(content, reasoningContent);

          setMessage((prevMessage) => {
            const newMessages = [...prevMessage];
            const lastMessage = newMessages[newMessages.length - 1];
            if (lastMessage?.status === MESSAGE_STATUS.LOADING) {
              const autoCollapseState = applyAutoCollapseLogic(
                lastMessage,
                true,
              );

              newMessages[newMessages.length - 1] = {
                ...lastMessage,
                content: processed.content,
                reasoningContent: processed.reasoningContent,
                status: MESSAGE_STATUS.COMPLETE,
                ...autoCollapseState,
              };
            }
            setTimeout(() => saveMessages(newMessages), 0);
            return newMessages;
          });
        }
      } catch (error) {
        if (!isCurrent()) return;
        console.error('Non-stream request error:', error);

        const errorInfo = handleApiError(error);
        setDebugData((prev) => ({
          ...prev,
          response: JSON.stringify(errorInfo, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        setMessage((prevMessage) => {
          const newMessages = [...prevMessage];
          const lastMessage = newMessages[newMessages.length - 1];
          if (lastMessage?.status === MESSAGE_STATUS.LOADING) {
            const autoCollapseState = applyAutoCollapseLogic(lastMessage, true);

            newMessages[newMessages.length - 1] = {
              ...lastMessage,
              content: t('请求发生错误: ') + error.message,
              status: MESSAGE_STATUS.ERROR,
              ...autoCollapseState,
            };
          }
          setTimeout(() => saveMessages(newMessages), 0);
          return newMessages;
        });
      } finally {
        if (sseSourceRef.current === controller) sseSourceRef.current = null;
      }
    },
    [
      setDebugData,
      setActiveDebugTab,
      setMessage,
      t,
      applyAutoCollapseLogic,
      saveMessages,
      sseSourceRef,
    ],
  );

  // SSE请求
  const handleSSE = useCallback(
    (payload) => {
      setDebugData((prev) => ({
        ...prev,
        request: payload,
        timestamp: new Date().toISOString(),
        response: null,
        sseMessages: [], // 新增：存储 SSE 消息数组
        isStreaming: true, // 新增：标记流式状态
      }));
      setActiveDebugTab(DEBUG_TABS.REQUEST);

      const source = new SSE(API_ENDPOINTS.CHAT_COMPLETIONS, {
        headers: {
          'Content-Type': 'application/json',
          'New-Api-User': getUserIdFromLocalStorage(),
        },
        method: 'POST',
        payload: JSON.stringify(payload),
        start: false,
      });

      sseSourceRef.current = source;

      let responseData = '';
      let hasReceivedFirstResponse = false;
      let isStreamComplete = false; // 添加标志位跟踪流是否正常完成
      let hasStreamFailed = false;
      let responseStatus = 0;

      const failStream = (rawError, fallbackMessage) => {
        if (
          sseSourceRef.current !== source ||
          isStreamComplete ||
          hasStreamFailed
        )
          return;
        hasStreamFailed = true;

        const errorMessage = extractPlaygroundErrorMessage(
          rawError,
          fallbackMessage,
        );
        const rawText = typeof rawError === 'string' ? rawError.trim() : '';

        setDebugData((prev) => ({
          ...prev,
          response:
            responseData +
            `\n\nError: ${errorMessage}` +
            (rawText && rawText !== errorMessage ? `\n${rawText}` : ''),
          sseMessages: rawText
            ? [...(prev.sseMessages || []), rawText]
            : prev.sseMessages,
          isStreaming: false,
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        streamMessageUpdate(errorMessage, 'content');
        completeMessage(MESSAGE_STATUS.ERROR);
        sseSourceRef.current = null;
        source.close();
      };

      source.addEventListener('open', (e) => {
        responseStatus = e.responseCode || 0;
      });

      source.addEventListener('message', (e) => {
        if (sseSourceRef.current !== source || hasStreamFailed) return;

        const parsedEvent = parsePlaygroundSSEItem(e.data);
        if (parsedEvent.ignored) return;

        if (parsedEvent.isDone) {
          isStreamComplete = true; // 标记流正常完成
          source.close();
          sseSourceRef.current = null;
          setDebugData((prev) => ({
            ...prev,
            response: responseData,
            sseMessages: [...(prev.sseMessages || []), '[DONE]'], // 添加 DONE 标记
            isStreaming: false,
          }));
          completeMessage();
          return;
        }

        if (parsedEvent.error) {
          console.error('Failed to parse SSE message:', parsedEvent.error);
          failStream(e.data, t('解析响应数据时发生错误'));
          return;
        }

        if (parsedEvent.serverError) {
          failStream(e.data, parsedEvent.serverError);
          return;
        }

        const payload = parsedEvent.parsed;
        responseData += parsedEvent.data + '\n';

        if (!hasReceivedFirstResponse) {
          setActiveDebugTab(DEBUG_TABS.RESPONSE);
          hasReceivedFirstResponse = true;
        }

        // 新增：将 SSE 消息添加到数组
        setDebugData((prev) => ({
          ...prev,
          sseMessages: [...(prev.sseMessages || []), e.data],
        }));

        const delta = payload?.choices?.[0]?.delta;
        if (delta) {
          if (delta.reasoning_content) {
            streamMessageUpdate(delta.reasoning_content, 'reasoning');
          }
          if (delta.reasoning) {
            streamMessageUpdate(delta.reasoning, 'reasoning');
          }
          if (delta.content) {
            streamMessageUpdate(delta.content, 'content');
          }
        }
      });

      source.addEventListener('error', (e) => {
        if (
          sseSourceRef.current !== source ||
          isStreamComplete ||
          hasStreamFailed
        )
          return;
        console.error('SSE Error:', e);
        failStream(e.data, t('请求发生错误'));
      });

      source.addEventListener('readystatechange', (e) => {
        if (
          sseSourceRef.current !== source ||
          e.readyState < 2 ||
          isStreamComplete ||
          hasStreamFailed
        )
          return;

        const fallbackMessage =
          responseStatus && responseStatus !== 200
            ? `HTTP ${responseStatus}`
            : t('连接已断开');
        failStream('', fallbackMessage);
      });

      try {
        source.stream();
      } catch (error) {
        console.error('Failed to start SSE stream:', error);
        const errorInfo = handleApiError(error);

        setDebugData((prev) => ({
          ...prev,
          response: 'Stream启动失败:\n' + JSON.stringify(errorInfo, null, 2),
        }));
        setActiveDebugTab(DEBUG_TABS.RESPONSE);

        streamMessageUpdate(t('建立连接时发生错误'), 'content');
        completeMessage(MESSAGE_STATUS.ERROR);
      }
    },
    [
      setDebugData,
      setActiveDebugTab,
      streamMessageUpdate,
      completeMessage,
      t,
      applyAutoCollapseLogic,
    ],
  );

  // 停止生成
  const onStopGenerator = useCallback(() => {
    // 如果仍有活动的 SSE 连接，首先关闭
    if (sseSourceRef.current) {
      const source = sseSourceRef.current;
      sseSourceRef.current = null;
      source.close();
    }
    setDebugData((prev) => ({ ...prev, isStreaming: false }));

    // 无论是否存在 SSE 连接，都尝试处理最后一条正在生成的消息
    setMessage((prevMessage) => {
      if (prevMessage.length === 0) return prevMessage;
      const lastMessage = prevMessage[prevMessage.length - 1];

      if (
        lastMessage.status === MESSAGE_STATUS.LOADING ||
        lastMessage.status === MESSAGE_STATUS.INCOMPLETE
      ) {
        const processed = processIncompleteThinkTags(
          lastMessage.content || '',
          lastMessage.reasoningContent || '',
        );

        if (!processed.content && !processed.reasoningContent) {
          const remaining = prevMessage.slice(0, -1);
          setTimeout(() => saveMessages(remaining), 0);
          return remaining;
        }

        const autoCollapseState = applyAutoCollapseLogic(lastMessage, true);

        const updatedMessages = [
          ...prevMessage.slice(0, -1),
          {
            ...lastMessage,
            status: MESSAGE_STATUS.COMPLETE,
            reasoningContent: processed.reasoningContent || null,
            content: processed.content,
            ...autoCollapseState,
          },
        ];

        // 停止生成时也保存，传入更新后的消息列表
        setTimeout(() => saveMessages(updatedMessages), 0);

        return updatedMessages;
      }
      return prevMessage;
    });
  }, [setMessage, setDebugData, applyAutoCollapseLogic, saveMessages]);

  // 发送请求
  const sendRequest = useCallback(
    (payload, isStream) => {
      if (isStream) {
        handleSSE(payload);
      } else {
        handleNonStreamRequest(payload);
      }
    },
    [handleSSE, handleNonStreamRequest],
  );

  return {
    sendRequest,
    onStopGenerator,
    streamMessageUpdate,
    completeMessage,
  };
};
