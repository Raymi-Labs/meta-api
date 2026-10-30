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
      <ModelInfoCard
        model={model}
        modelName={inputs.model}
        group={inputs.group}
        catalogState={catalogState}
      />
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
      <CapabilitySettings
        inputs={inputs}
        model={model}
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
      <details className='pg-settings-section pg-advanced'>
        <summary>
          {t('高级设置')}
          <ChevronDown size={14} />
        </summary>
        <ImageUrlInput
          imageUrls={inputs.imageUrls}
          imageEnabled={inputs.imageEnabled}
          onImageUrlsChange={(urls) => onInputChange('imageUrls', urls)}
          onImageEnabledChange={(value) => onInputChange('imageEnabled', value)}
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
