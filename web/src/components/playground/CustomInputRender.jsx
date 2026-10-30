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

import React, { useRef, useEffect, useCallback } from 'react';
import { Toast } from '@douyinfe/semi-ui';
import { ArrowUp, Square, Trash2, ImagePlus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { usePlayground } from '../../contexts/PlaygroundContext';

const CustomInputRender = ({
  draft,
  onDraftChange,
  onSend,
  onStop,
  onClear,
  generating,
  disabled,
  inputs,
  onInputChange,
  onOpenSettings,
  customRequestMode,
}) => {
  const { t } = useTranslation();
  const { onPasteImage, imageEnabled } = usePlayground();
  const containerRef = useRef(null);
  const handlePaste = useCallback(
    async (e) => {
      const items = e.clipboardData?.items;
      if (!items) return;

      for (let i = 0; i < items.length; i++) {
        const item = items[i];

        if (item.type.indexOf('image') !== -1) {
          e.preventDefault();
          const file = item.getAsFile();

          if (file) {
            try {
              if (!imageEnabled) {
                Toast.warning({
                  content: t('请先在设置中启用图片功能'),
                  duration: 3,
                });
                return;
              }

              const reader = new FileReader();
              reader.onload = (event) => {
                const base64 = event.target.result;

                if (onPasteImage) {
                  onPasteImage(base64);
                  Toast.success({
                    content: t('图片已添加'),
                    duration: 2,
                  });
                } else {
                  Toast.error({
                    content: t('无法添加图片'),
                    duration: 2,
                  });
                }
              };
              reader.onerror = () => {
                console.error('Failed to read image file:', reader.error);
                Toast.error({
                  content: t('粘贴图片失败'),
                  duration: 2,
                });
              };
              reader.readAsDataURL(file);
            } catch (error) {
              console.error('Failed to paste image:', error);
              Toast.error({
                content: t('粘贴图片失败'),
                duration: 2,
              });
            }
          }
          break;
        }
      }
    },
    [onPasteImage, imageEnabled, t],
  );

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    container.addEventListener('paste', handlePaste);
    return () => {
      container.removeEventListener('paste', handlePaste);
    };
  }, [handlePaste]);

  const submit = () => {
    if (!generating && !disabled && (draft.trim() || customRequestMode))
      onSend(draft);
  };
  return (
    <div className='pg-composer-wrap' ref={containerRef}>
      <div className='pg-composer'>
        <textarea
          aria-label={t('请输入您的问题...')}
          placeholder={t('请输入您的问题...')}
          value={draft}
          disabled={customRequestMode}
          onChange={(event) => onDraftChange(event.target.value)}
          onKeyDown={(event) => {
            if (
              (event.ctrlKey || event.metaKey) &&
              event.key === 'Enter' &&
              !event.nativeEvent.isComposing
            ) {
              event.preventDefault();
              submit();
            }
          }}
        />
        <div className='pg-composer-tools'>
          <button
            aria-label={t('清空对话')}
            title={t('清空对话')}
            onClick={onClear}
            disabled={generating}
          >
            <Trash2 size={17} />
          </button>
          <button
            aria-label={t('图片地址')}
            title={t('图片地址')}
            aria-pressed={inputs.imageEnabled}
            disabled={generating || customRequestMode}
            onClick={() => {
              onInputChange('imageEnabled', !inputs.imageEnabled);
              onOpenSettings();
            }}
            className={inputs.imageEnabled ? 'is-active' : ''}
          >
            <ImagePlus size={18} />
          </button>
          <label className='pg-stream-toggle'>
            <input
              type='checkbox'
              checked={inputs.stream}
              disabled={generating || customRequestMode}
              onChange={(event) =>
                onInputChange('stream', event.target.checked)
              }
            />
            <span>{t('流式输出')}</span>
          </label>
          {generating ? (
            <button
              className='pg-send'
              onClick={onStop}
              aria-label={t('停止生成')}
            >
              <Square size={15} />
              <span>{t('停止生成')}</span>
            </button>
          ) : (
            <button
              className='pg-send'
              disabled={disabled || (!draft.trim() && !customRequestMode)}
              onClick={submit}
              aria-label={t('发送')}
            >
              <ArrowUp size={17} />
              <span>{t('发送')}</span>
            </button>
          )}
        </div>
      </div>
      <div className='pg-composer-caption'>
        {customRequestMode
          ? t('自定义请求体模式')
          : t('切换模型保留当前对话；每次只请求选中的模型。')}
        <span>⌘ / Ctrl + Enter</span>
      </div>
    </div>
  );
};
export default CustomInputRender;
