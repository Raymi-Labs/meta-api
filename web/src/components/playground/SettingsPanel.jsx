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
import { Tooltip } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import ParameterControl from './ParameterControl';
import ImageUrlInput from './ImageUrlInput';
import ConfigManager from './ConfigManager';
import CustomRequestEditor from './CustomRequestEditor';
import ModelInfoCard from './ModelInfoCard';
import PlaygroundPriceSummary from './PlaygroundPriceSummary';
import CapabilitySettings from './CapabilitySettings';
import SystemPromptEditor from './SystemPromptEditor';
import { formatModelTokenLimit } from '../../helpers/modelAttributes';

export default function SettingsPanel({
  inputs,
  parameterEnabled,
  styleState,
  showDebugPanel,
  customRequestMode,
  customRequestBody,
  onInputChange,
  onParameterToggle,
  onConfigImport,
  onConfigReset,
  onCustomRequestModeChange,
  onCustomRequestBodyChange,
  previewPayload,
  messages,
  model,
  catalogState,
  comparison,
  localSession = false,
}) {
  const { t } = useTranslation();
  const currentConfig = {
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
  };
  const count = [
    'temperature',
    'top_p',
    'max_tokens',
    'frequency_penalty',
    'presence_penalty',
    'seed',
  ].filter(
    (key) =>
      parameterEnabled[key] &&
      inputs[key] !== null &&
      inputs[key] !== undefined,
  ).length;
  const inspected = comparison ? comparison.model : model;
  return (
    <div className='pg-settings-scroll model-settings-scroll'>
      <section className='pg-settings-section pg-prompt-settings'>
        <SystemPromptEditor
          value={inputs.systemPrompt}
          onChange={(value) => onInputChange('systemPrompt', value)}
          disabled={customRequestMode}
        />
      </section>
      {customRequestMode && (
        <p role='status' className='pg-field-hint'>
          {t('已在自定义模式中忽略')}
        </p>
      )}
      {comparison && (inputs.imageEnabled || inputs.toolsEnabled) && (
        <button
          className='pg-parallel-toggle'
          onClick={() => {
            onInputChange('imageEnabled', false);
            onInputChange('toolsEnabled', false);
          }}
        >
          {t('关闭图片和工具')}
        </button>
      )}
      <CapabilitySettings
        inputs={inputs}
        model={comparison ? null : model}
        textOnly={Boolean(comparison)}
        onInputChange={onInputChange}
        disabled={customRequestMode}
      />
      <details className='pg-settings-section pg-parameters'>
        <summary>
          <span>{t('生成参数')}</span>
          <small>
            {customRequestMode
              ? t('已在自定义模式中忽略')
              : count
                ? t('已启用 {{count}} 项', { count })
                : t('使用模型默认值')}
          </small>
        </summary>
        <Tooltip
          content={t(
            '开启后才发送该参数；关闭时保留当前数值，实际行为使用模型默认值。',
          )}
        >
          <button
            className='pg-info-button'
            aria-label={t('参数说明')}
            type='button'
          >
            <Info size={14} />
          </button>
        </Tooltip>
        <ParameterControl
          inputs={inputs}
          parameterEnabled={parameterEnabled}
          onInputChange={onInputChange}
          onParameterToggle={onParameterToggle}
          disabled={customRequestMode}
        />
      </details>
      {!comparison && (
        <details className='pg-settings-section pg-advanced'>
          <summary>{t('高级设置')}</summary>
          <ImageUrlInput
            imageUrls={inputs.imageUrls}
            imageEnabled={inputs.imageEnabled}
            onImageUrlsChange={(urls) => onInputChange('imageUrls', urls)}
            onImageEnabledChange={(value) =>
              onInputChange('imageEnabled', value)
            }
            disabled={customRequestMode}
          />
          <CustomRequestEditor
            customRequestMode={customRequestMode}
            customRequestBody={customRequestBody}
            onCustomRequestModeChange={onCustomRequestModeChange}
            onCustomRequestBodyChange={onCustomRequestBodyChange}
            defaultPayload={previewPayload}
          />
        </details>
      )}
      <ConfigManager
        compact
        localSession={localSession}
        currentConfig={currentConfig}
        onConfigImport={onConfigImport}
        onConfigReset={onConfigReset}
        styleState={styleState}
        messages={messages}
      />
      <details className='pg-settings-section pg-model-details-fold'>
        <summary>
          <span>{t('模型信息')}</span>
          <small className='pg-model-summary'>
            <span
              title={t('上下文长度')}
              aria-label={`${t('上下文长度')} ${formatModelTokenLimit(inspected?.context_length)}`}
            >
              {formatModelTokenLimit(inspected?.context_length)}
            </span>
            <PlaygroundPriceSummary
              compact
              model={inspected}
              group={inputs.group}
              catalogState={catalogState}
            />
          </small>
        </summary>
        {comparison && (
          <div
            className='pg-inspector-models'
            role='group'
            aria-label={t('查看模型资料')}
          >
            {comparison.names.map((name) => (
              <button
                key={name}
                aria-pressed={comparison.inspected === name}
                onClick={() => comparison.onInspect(name)}
              >
                {name}
              </button>
            ))}
          </div>
        )}
        <ModelInfoCard
          model={inspected}
          modelName={comparison ? comparison.inspected : inputs.model}
          group={inputs.group}
          catalogState={catalogState}
        />
      </details>
    </div>
  );
}
