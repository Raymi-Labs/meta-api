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

import React from 'react';
import { Chat } from '@douyinfe/semi-ui';
import { MessageSquare, ArrowUpRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import CustomInputRender from './CustomInputRender';

const ChatArea = ({
  chatRef,
  message,
  inputs,
  roleInfo,
  onMessageSend,
  onStopGenerator,
  onClearMessages,
  renderCustomChatContent,
  renderChatBoxAction,
  draft,
  onDraftChange,
  generating,
  disabled,
  onInputChange,
  onOpenSettings,
  customRequestMode,
  toolResults,
}) => {
  const { t } = useTranslation();
  return (
    <section className='pg-chat'>
      <div className='pg-messages'>
        {!message.length ? (
          <div className='pg-empty'>
            <div className='pg-empty-icon'>
              <MessageSquare size={28} />
            </div>
            <span className='pg-eyebrow'>PLAYGROUND</span>
            <h1>{t('选择模型开始对话')}</h1>
            <p>{t('调整参数、测试提示词，将请求复制为代码。')}</p>
            <div className='pg-starter-prompts'>
              {[t('解释一个概念'), t('比较两个方案'), t('编写一段代码')].map(
                (text) => (
                  <button
                    key={text}
                    disabled={customRequestMode}
                    onClick={() => onDraftChange(text)}
                  >
                    {text}
                    <ArrowUpRight size={14} />
                  </button>
                ),
              )}
            </div>
            <span className='pg-empty-note'>
              Chat Completions · {t('模型体验')}
            </span>
          </div>
        ) : (
          <Chat
            ref={chatRef}
            chats={message}
            roleConfig={roleInfo}
            chatBoxRenderConfig={{
              renderChatBoxContent: renderCustomChatContent,
              renderChatBoxAction,
              renderChatBoxTitle: () => null,
            }}
            renderInputArea={() => null}
            showClearContext={false}
            showStopGenerate={false}
            style={{ height: '100%', maxWidth: '100%' }}
          />
        )}
      </div>
      {toolResults}
      <CustomInputRender
        draft={draft}
        onDraftChange={onDraftChange}
        onSend={onMessageSend}
        onStop={onStopGenerator}
        onClear={onClearMessages}
        generating={generating}
        disabled={disabled}
        inputs={inputs}
        onInputChange={onInputChange}
        onOpenSettings={onOpenSettings}
        customRequestMode={customRequestMode}
      />
    </section>
  );
};
export default ChatArea;
