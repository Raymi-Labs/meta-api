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
import { Select } from '@douyinfe/semi-ui';
import { SlidersHorizontal, ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderGroupOption, selectFilter } from '../../helpers';
import ParameterControl from './ParameterControl';
import ImageUrlInput from './ImageUrlInput';
import ConfigManager from './ConfigManager';
import CustomRequestEditor from './CustomRequestEditor';
import ModelInfoCard from './ModelInfoCard';
import CapabilitySettings from './CapabilitySettings';

const SettingsPanel = ({
  inputs,
  parameterEnabled,
  groups,
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
}) => {
  const { t } = useTranslation();
  const currentConfig = {
    inputs,
    parameterEnabled,
    showDebugPanel,
    customRequestMode,
    customRequestBody,
  };
  return (
    <div className='pg-settings-scroll model-settings-scroll'>
      {comparison ? (
        <section className='pg-settings-section pg-comparison-inspector'>
          <h3>{t('本轮模型')}</h3>
          <p className='pg-field-hint'>
            {t('点击查看资料；不会改变调用名单。')}
          </p>
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
          {!comparison.names.length && (
            <p className='pg-field-hint'>{t('选择文本模型（最多5个）')}</p>
          )}
          {comparison.inspected && (
            <details
              key={comparison.inspected}
              className='pg-inspector-details'
            >
              <summary>
                {t('查看价格与能力')} · {comparison.inspected}
              </summary>
              <ModelInfoCard
                model={comparison.model}
                modelName={comparison.inspected}
                group={inputs.group}
                catalogState={catalogState}
              />
              <p className='pg-field-hint'>
                {t('深度思考')}：
                {comparison.model?.capabilities?.reasoning === false
                  ? t('不支持')
                  : comparison.model?.capabilities?.reasoning === true
                    ? t('支持')
                    : t('未知')}
              </p>
            </details>
          )}
        </section>
      ) : (
        <ModelInfoCard
          model={model}
          modelName={inputs.model}
          group={inputs.group}
          catalogState={catalogState}
        />
      )}
      <section className='pg-settings-section pg-group-section'>
        <label>
          {t('分组')}
          <Select
            aria-label={t('分组')}
            filter={selectFilter}
            value={inputs.group}
            onChange={(value) => onInputChange('group', value)}
            optionList={groups}
            renderOptionItem={renderGroupOption}
            disabled={customRequestMode}
            style={{ width: '100%' }}
          />
        </label>
      </section>
      {customRequestMode && (
        <p className='pg-field-hint'>{t('已在自定义模式中忽略')}</p>
      )}
      {comparison && (
        <section className='pg-settings-section pg-common-settings'>
          <h3>{t('共用设置')}</h3>
          <p className='pg-field-hint'>
            {t('参数应用于本轮所有模型；明确不支持思考的模型会忽略思考强度。')}
          </p>
          <p className='pg-field-hint'>
            {t('仅发送文本；各模型价格与能力请分别查看。')}
          </p>
          {(inputs.imageEnabled || inputs.toolsEnabled) && (
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
        </section>
      )}
      <CapabilitySettings
        inputs={inputs}
        model={comparison ? null : model}
        textOnly={Boolean(comparison)}
        onInputChange={onInputChange}
        disabled={customRequestMode}
      />
      <section className='pg-settings-section pg-parameters'>
        <h3>
          <SlidersHorizontal size={15} />
          {t('参数')}
        </h3>
        <ParameterControl
          inputs={inputs}
          parameterEnabled={parameterEnabled}
          onInputChange={onInputChange}
          onParameterToggle={onParameterToggle}
          disabled={customRequestMode}
        />
      </section>
      {!comparison && (
        <details className='pg-settings-section pg-advanced'>
          <summary>
            {t('高级设置')}
            <ChevronDown size={14} />
          </summary>
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
        currentConfig={currentConfig}
        onConfigImport={onConfigImport}
        onConfigReset={onConfigReset}
        styleState={styleState}
        messages={messages}
      />
    </div>
  );
};
export default SettingsPanel;
