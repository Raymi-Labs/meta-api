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

import React, {
  useContext,
  useEffect,
  useCallback,
  useRef,
  useMemo,
  useState,
} from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Toast, Banner, Button } from '@douyinfe/semi-ui';

import { getPlaygroundOwner } from '../../components/playground/configStorage';

// Context
import { UserContext } from '../../context/User';
import { useIsMobile } from '../../hooks/common/useIsMobile';

// hooks
import { usePlaygroundState } from '../../hooks/playground/usePlaygroundState';
import { useMessageActions } from '../../hooks/playground/useMessageActions';
import { useApiRequest } from '../../hooks/playground/useApiRequest';
import { useMessageEdit } from '../../hooks/playground/useMessageEdit';
import { usePlaygroundCatalog } from '../../hooks/playground/usePlaygroundCatalog';
import {
  pendingToolCalls,
  withModelCapabilities,
} from '../../helpers/playgroundCapabilities';
import ToolResultsForm from '../../components/playground/ToolResultsForm';
import { useDataLoader } from '../../hooks/playground/useDataLoader';

// Constants and utils
import { MESSAGE_ROLES } from '../../constants/playground.constants';
import {
  getLogo,
  stringToColor,
  buildMessageContent,
  createMessage,
  createLoadingAssistantMessage,
  buildApiPayload,
  encodeToBase64,
} from '../../helpers';

// Components
import {
  OptimizedSettingsPanel,
  OptimizedMessageContent,
  OptimizedMessageActions,
} from '../../components/playground/OptimizedComponents';
import ChatArea from '../../components/playground/ChatArea';
import { SlidersHorizontal, Code2, RefreshCw, Copy } from 'lucide-react';
import PlaygroundWorkspace from '../../components/playground/PlaygroundWorkspace';
import ModelTabs from '../../components/playground/ModelTabs';
import SystemPromptEditor from '../../components/playground/SystemPromptEditor';
import RequestCodePanel from '../../components/playground/RequestCodePanel';
import {
  buildPlaygroundPreview,
  reconcileModelTabs,
} from '../../helpers/playgroundRequest';
import { PlaygroundProvider } from '../../contexts/PlaygroundContext';

// 生成头像
const generateAvatarDataUrl = (username) => {
  if (!username) {
    return 'https://lf3-static.bytednsdoc.com/obj/eden-cn/ptlz_zlp/ljhwZthlaukjlkulzlp/docs-icon.png';
  }
  const firstLetter = username[0].toUpperCase();
  const bgColor = stringToColor(username);
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
      <circle cx="16" cy="16" r="16" fill="${bgColor}" />
      <text x="50%" y="50%" dominant-baseline="central" text-anchor="middle" font-size="16" fill="#ffffff" font-family="sans-serif">${firstLetter}</text>
    </svg>
  `;
  return `data:image/svg+xml;base64,${encodeToBase64(svg)}`;
};

const AccountPlayground = ({ owner }) => {
  const { t } = useTranslation();
  const [userState] = useContext(UserContext);
  const isMobile = useIsMobile();
  const styleState = { isMobile };
  const [searchParams] = useSearchParams();

  const state = usePlaygroundState(owner);
  const {
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
    showSettings,
    models,
    groups,
    status,
    message,
    debugData,
    activeDebugTab,
    sseSourceRef,
    chatRef,
    handleInputChange,
    handleParameterToggle,
    debouncedSaveConfig,
    saveMessagesImmediately,
    handleConfigImport,
    handleConfigReset,
    setShowSettings,
    setModels,
    setGroups,
    setStatus,
    setMessage,
    setDebugData,
    setActiveDebugTab,
    setShowDebugPanel,
    setCustomRequestMode,
    setCustomRequestBody,
  } = state;

  const catalogState = usePlaygroundCatalog(owner);
  const selectedModel = catalogState.catalog?.models.find(
    (model) => model.model_name === inputs.model,
  );
  const effectiveInputs = withModelCapabilities(inputs, selectedModel);
  const pendingCalls = pendingToolCalls(message);
  const hasToolHistory = message.some(
    (item) => item.role === 'tool' || item.tool_calls?.length,
  );
  const [draft, setDraft] = useState('');
  const [toolResults, setToolResults] = useState({});
  const messageRef = useRef(message);
  messageRef.current = message;
  const sendingRef = useRef(false);
  const activeRef = useRef(true);
  useEffect(() => {
    activeRef.current = true;
    return () => {
      activeRef.current = false;
    };
  }, []);
  const generating = message.some((item) =>
    ['loading', 'incomplete'].includes(item.status),
  );
  useEffect(() => {
    if (!generating) sendingRef.current = false;
  }, [generating]);

  // API 请求相关
  const { sendRequest, onStopGenerator } = useApiRequest(
    setMessage,
    setDebugData,
    setActiveDebugTab,
    sseSourceRef,
    saveMessagesImmediately,
  );

  // 数据加载
  const { modelsLoading, modelsError, reloadModels } = useDataLoader(
    userState,
    inputs,
    handleInputChange,
    setModels,
    setGroups,
  );
  const tabs = reconcileModelTabs(inputs.modelTabs, inputs.model, models);
  const canSend =
    !modelsLoading &&
    !modelsError &&
    models.some((model) => model.value === inputs.model);
  useEffect(() => {
    if (modelsLoading || modelsError) return;
    if (JSON.stringify(tabs) !== JSON.stringify(inputs.modelTabs))
      handleInputChange('modelTabs', tabs);
  }, [
    modelsLoading,
    modelsError,
    JSON.stringify(tabs),
    inputs.modelTabs,
    handleInputChange,
  ]);
  const selectModel = (model) => {
    if (generating || pendingCalls.length || customRequestMode) return;
    handleInputChange('model', model);
    handleInputChange('modelTabs', [...new Set([...tabs, model])]);
  };
  const removeModel = (model) => {
    if (
      generating ||
      pendingCalls.length ||
      customRequestMode ||
      tabs.length < 2
    )
      return;
    const remaining = tabs.filter((value) => value !== model);
    handleInputChange('modelTabs', remaining);
    if (inputs.model === model) handleInputChange('model', remaining[0]);
  };

  // 消息编辑
  const {
    editingMessageId,
    editValue,
    setEditValue,
    handleMessageEdit,
    handleEditSave,
    handleEditCancel,
  } = useMessageEdit(
    message,
    setMessage,
    onMessageSend,
    saveMessagesImmediately,
  );

  // 角色信息
  const roleInfo = {
    user: {
      name: userState?.user?.username || 'User',
      avatar: generateAvatarDataUrl(userState?.user?.username),
    },
    assistant: {
      name: 'Assistant',
      avatar: getLogo(),
    },
    tool: { name: t('工具结果'), avatar: getLogo() },
    system: {
      name: 'System',
      avatar: getLogo(),
    },
  };

  // 消息操作
  const messageActions = useMessageActions(
    message,
    setMessage,
    onMessageSend,
    saveMessagesImmediately,
  );

  // A single payload builder owns the draft, system prompt and enabled parameters.
  const constructPreviewPayload = useCallback(() => {
    if (customRequestMode) {
      try {
        const payload = JSON.parse(customRequestBody);
        return payload &&
          !Array.isArray(payload) &&
          Array.isArray(payload.messages)
          ? payload
          : null;
      } catch {
        return null;
      }
    }
    try {
      const previewMessages =
        pendingCalls.length &&
        pendingCalls.every((call) => toolResults[call.id]?.trim())
          ? [
              ...message,
              ...pendingCalls.map((call) => ({
                role: 'tool',
                content: toolResults[call.id],
                tool_call_id: call.id,
              })),
            ]
          : message;
      return buildPlaygroundPreview(
        previewMessages,
        pendingCalls.length ? '' : draft,
        effectiveInputs,
        parameterEnabled,
      );
    } catch {
      return null;
    }
  }, [
    message,
    draft,
    inputs,
    selectedModel,
    toolResults,
    parameterEnabled,
    customRequestMode,
    customRequestBody,
  ]);

  const previewPayload = useMemo(constructPreviewPayload, [
    constructPreviewPayload,
  ]);

  function onMessageSend(content, retryMessages = null) {
    if (
      !activeRef.current ||
      owner !== getPlaygroundOwner() ||
      sendingRef.current ||
      generating ||
      !canSend
    )
      return false;
    let payload;
    let nextMessages;
    if (customRequestMode) {
      try {
        payload = JSON.parse(customRequestBody);
        if (
          !payload?.model ||
          !Array.isArray(payload.messages) ||
          !payload.messages.length
        )
          throw new Error('Invalid request');
        nextMessages = payload.messages.map((item) =>
          createMessage(item.role, item.content, {
            tool_calls: item.tool_calls,
            tool_call_id: item.tool_call_id,
            reasoningContent: item.reasoning_content,
          }),
        );
      } catch {
        Toast.error(t('JSON格式错误'));
        return false;
      }
    } else {
      if (!retryMessages && (!content.trim() || pendingCalls.length))
        return false;
      const messageContent = buildMessageContent(
        content,
        (inputs.imageUrls || []).filter((url) => url.trim()),
        inputs.imageEnabled,
      );
      nextMessages = retryMessages || [
        ...messageRef.current,
        createMessage(MESSAGE_ROLES.USER, messageContent),
      ];
      try {
        payload = buildApiPayload(
          nextMessages,
          null,
          effectiveInputs,
          parameterEnabled,
        );
      } catch {
        Toast.error(t('请检查函数定义与调用策略'));
        return false;
      }
    }
    sendingRef.current = true;
    const next = [...nextMessages, createLoadingAssistantMessage()];
    setMessage(next);
    saveMessagesImmediately(next);
    setDraft('');
    setToolResults({});
    sendRequest(payload, payload.stream !== false);
    if (!customRequestMode && inputs.imageEnabled)
      handleInputChange('imageEnabled', false);
    return true;
  }

  function submitToolResults(values) {
    const currentCalls = pendingToolCalls(messageRef.current);
    if (
      !currentCalls.length ||
      currentCalls.some((call) => !values[call.id]?.trim())
    )
      return;
    const next = [
      ...messageRef.current,
      ...currentCalls.map((call) =>
        createMessage('tool', values[call.id], { tool_call_id: call.id }),
      ),
    ];
    onMessageSend('', next);
  }

  // 切换推理展开状态
  const toggleReasoningExpansion = useCallback(
    (messageId) => {
      setMessage((prevMessages) =>
        prevMessages.map((msg) =>
          msg.id === messageId && msg.role === MESSAGE_ROLES.ASSISTANT
            ? { ...msg, isReasoningExpanded: !msg.isReasoningExpanded }
            : msg,
        ),
      );
    },
    [setMessage],
  );

  // 渲染函数
  const renderCustomChatContent = useCallback(
    ({ message, className }) => {
      const isCurrentlyEditing = editingMessageId === message.id;

      return (
        <OptimizedMessageContent
          message={message}
          className={className}
          styleState={styleState}
          onToggleReasoningExpansion={toggleReasoningExpansion}
          isEditing={isCurrentlyEditing}
          onEditSave={handleEditSave}
          onEditCancel={handleEditCancel}
          editValue={editValue}
          onEditValueChange={setEditValue}
        />
      );
    },
    [
      styleState,
      editingMessageId,
      editValue,
      handleEditSave,
      handleEditCancel,
      setEditValue,
      toggleReasoningExpansion,
    ],
  );

  const renderChatBoxAction = useCallback(
    (props) => {
      const { message: currentMessage } = props;
      // A tool exchange is an indivisible protocol sequence. Clear the conversation
      // to start over instead of editing/deleting individual protocol messages.
      if (hasToolHistory) {
        const index = message.findIndex(
          (item) => item.id === currentMessage.id,
        );
        return (
          <div className='pg-tool-message-actions'>
            <button
              aria-label={t('复制')}
              onClick={() =>
                messageActions.handleMessageCopy({
                  ...currentMessage,
                  content: currentMessage.tool_calls?.length
                    ? JSON.stringify(currentMessage.tool_calls, null, 2)
                    : currentMessage.content,
                })
              }
            >
              <Copy size={13} />
            </button>
            {currentMessage.role === 'assistant' &&
              (currentMessage.status === 'error' ||
                currentMessage.toolCallsInterrupted) && (
                <button
                  disabled={generating || customRequestMode}
                  onClick={() => onMessageSend('', message.slice(0, index))}
                >
                  <RefreshCw size={13} />
                  {t('重试')}
                </button>
              )}
          </div>
        );
      }
      const isAnyMessageGenerating =
        customRequestMode ||
        message.some(
          (msg) => msg.status === 'loading' || msg.status === 'incomplete',
        );
      const isCurrentlyEditing = editingMessageId === currentMessage.id;

      return (
        <OptimizedMessageActions
          message={currentMessage}
          styleState={styleState}
          onMessageReset={messageActions.handleMessageReset}
          onMessageCopy={messageActions.handleMessageCopy}
          onMessageDelete={messageActions.handleMessageDelete}
          onRoleToggle={messageActions.handleRoleToggle}
          onMessageEdit={handleMessageEdit}
          isAnyMessageGenerating={isAnyMessageGenerating}
          isEditing={isCurrentlyEditing}
        />
      );
    },
    [
      messageActions,
      hasToolHistory,
      styleState,
      message,
      editingMessageId,
      handleMessageEdit,
      customRequestMode,
    ],
  );

  // Effects

  // 处理URL参数
  useEffect(() => {
    if (searchParams.get('expired')) {
      Toast.warning(t('登录过期，请重新登录！'));
    }
  }, [searchParams, t]);

  // Playground 组件无需再监听窗口变化，isMobile 由 useIsMobile Hook 自动更新

  // 构建预览payload
  useEffect(() => {
    const timer = setTimeout(() => {
      const preview = constructPreviewPayload();
      setDebugData((prev) => ({
        ...prev,
        previewRequest: preview ? JSON.stringify(preview, null, 2) : null,
        previewTimestamp: preview ? new Date().toISOString() : null,
      }));
    }, 300);

    return () => clearTimeout(timer);
  }, [
    message,
    inputs,
    parameterEnabled,
    customRequestMode,
    customRequestBody,
    constructPreviewPayload,
    setDebugData,
  ]);

  // 自动保存配置
  useEffect(() => {
    debouncedSaveConfig();
  }, [
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
    debouncedSaveConfig,
  ]);

  // 清空对话的处理函数
  const handleClearMessages = useCallback(() => {
    if (generating) return;
    setMessage([]);
    setToolResults({});
    // 清空对话后保存，传入空数组
    setTimeout(() => saveMessagesImmediately([]), 0);
  }, [generating, setMessage, saveMessagesImmediately]);

  // 处理粘贴图片
  const handlePasteImage = useCallback(
    (base64Data) => {
      if (!inputs.imageEnabled) {
        return;
      }
      // 添加图片到 imageUrls 数组
      const newUrls = [...(inputs.imageUrls || []), base64Data];
      handleInputChange('imageUrls', newUrls);
    },
    [inputs.imageEnabled, inputs.imageUrls, handleInputChange],
  );

  // Playground Context 值
  const playgroundContextValue = {
    onPasteImage: handlePasteImage,
    imageUrls: inputs.imageUrls || [],
    imageEnabled: inputs.imageEnabled || false,
  };

  const settingsPanel = (
    <div
      className={`pg-settings-lock ${generating || pendingCalls.length ? 'is-disabled' : ''}`}
      inert={generating || pendingCalls.length ? '' : undefined}
      aria-disabled={generating || pendingCalls.length > 0}
    >
      <OptimizedSettingsPanel
        inputs={inputs}
        model={selectedModel}
        catalogState={catalogState}
        parameterEnabled={parameterEnabled}
        models={models}
        groups={groups}
        styleState={{ isMobile: false }}
        showDebugPanel={showDebugPanel}
        customRequestMode={customRequestMode}
        customRequestBody={customRequestBody}
        onInputChange={handleInputChange}
        onParameterToggle={handleParameterToggle}
        onConfigImport={handleConfigImport}
        onConfigReset={handleConfigReset}
        onCustomRequestModeChange={setCustomRequestMode}
        onCustomRequestBodyChange={setCustomRequestBody}
        previewPayload={previewPayload}
        messages={message}
      />
    </div>
  );
  const codePanel = (
    <RequestCodePanel
      payload={previewPayload}
      model={catalogState.catalog?.models.find(
        (item) => item.model_name === previewPayload?.model,
      )}
      debugData={debugData}
      activeDebugTab={activeDebugTab}
      onActiveDebugTabChange={setActiveDebugTab}
      customRequestMode={customRequestMode}
    />
  );
  return (
    <PlaygroundProvider value={playgroundContextValue}>
      <PlaygroundWorkspace
        settings={settingsPanel}
        code={codePanel}
        showSettings={showSettings}
        showCode={showDebugPanel}
        onCloseSettings={() => setShowSettings(false)}
        onCloseCode={() => setShowDebugPanel(false)}
      >
        <div className='pg-workspace-toolbar'>
          <ModelTabs
            models={models}
            tabs={tabs}
            activeModel={inputs.model}
            onSelect={selectModel}
            onRemove={removeModel}
            disabled={
              generating ||
              pendingCalls.length > 0 ||
              modelsLoading ||
              customRequestMode
            }
          />
          <div className='pg-workspace-actions'>
            <button
              className='pg-open-settings'
              onClick={() => setShowSettings(true)}
              aria-label={t('参数')}
              title={t('参数')}
            >
              <SlidersHorizontal size={17} />
            </button>
            <button
              className='pg-open-code'
              onClick={() => setShowDebugPanel(true)}
              aria-label={t('请求代码')}
              title={t('请求代码')}
            >
              <Code2 size={18} />
            </button>
          </div>
        </div>
        <div className='pg-prompt-row'>
          <SystemPromptEditor
            value={inputs.systemPrompt}
            onChange={(value) => handleInputChange('systemPrompt', value)}
            disabled={generating || customRequestMode}
          />
        </div>
        {(modelsLoading || modelsError || !models.length) && (
          <div className='pg-load-state' role='status'>
            <span>
              {modelsLoading
                ? t('加载中...')
                : modelsError
                  ? t('加载模型失败')
                  : t('暂无可用模型')}
            </span>
            {!modelsLoading && (
              <button onClick={reloadModels}>
                <RefreshCw size={13} />
                {t('重试')}
              </button>
            )}
          </div>
        )}
        <ChatArea
          chatRef={chatRef}
          message={message}
          inputs={inputs}
          roleInfo={roleInfo}
          onMessageSend={onMessageSend}
          onStopGenerator={onStopGenerator}
          onClearMessages={handleClearMessages}
          renderCustomChatContent={renderCustomChatContent}
          renderChatBoxAction={renderChatBoxAction}
          draft={draft}
          onDraftChange={setDraft}
          generating={generating}
          disabled={
            !canSend ||
            editingMessageId !== null ||
            (!customRequestMode && pendingCalls.length > 0) ||
            !previewPayload
          }
          toolResults={
            !customRequestMode &&
            pendingCalls.length > 0 && (
              <ToolResultsForm
                key={pendingCalls.map((call) => call.id).join(',')}
                calls={pendingCalls}
                values={toolResults}
                onChange={setToolResults}
                onSubmit={submitToolResults}
                disabled={generating || !canSend || !previewPayload}
              />
            )
          }
          onInputChange={handleInputChange}
          onOpenSettings={() => isMobile && setShowSettings(true)}
          customRequestMode={customRequestMode}
        />
      </PlaygroundWorkspace>
    </PlaygroundProvider>
  );
};

const Playground = () => {
  const [userState] = useContext(UserContext);
  const { t } = useTranslation();
  const [, refreshIdentity] = useState(0);
  useEffect(() => {
    const onStorage = (event) => {
      if (event.key === 'user' || event.key === null)
        refreshIdentity((value) => value + 1);
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);
  const owner = userState?.user?.id ? String(userState.user.id) : null;
  if (!owner || owner !== getPlaygroundOwner()) {
    return (
      <Banner
        type='warning'
        closeIcon={null}
        description={t('登录状态已变化，请刷新页面')}
        fullMode={false}
      >
        <Button onClick={() => window.location.reload()}>{t('刷新')}</Button>
      </Banner>
    );
  }
  return <AccountPlayground key={owner} owner={owner} />;
};

export default Playground;
