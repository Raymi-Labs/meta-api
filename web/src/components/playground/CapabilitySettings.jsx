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
import { Switch, TextArea } from '@douyinfe/semi-ui';
import { Brain, Wrench, Radio, ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  DEFAULT_TOOL_DEFINITIONS,
  parsePlaygroundTools,
} from '../../helpers/playgroundCapabilities';

export default function CapabilitySettings({
  inputs,
  model,
  onInputChange,
  disabled,
}) {
  const { t } = useTranslation();
  const noReasoning = model?.capabilities?.reasoning === false;
  const noTools = model?.capabilities?.function_calling === false;
  const tools = parsePlaygroundTools(inputs.toolsJson);
  const choiceValid =
    !inputs.toolChoice ||
    ['auto', 'required', 'none'].includes(inputs.toolChoice) ||
    tools?.some((tool) => tool.function.name === inputs.toolChoice);
  const toolsEnabled = inputs.toolsEnabled && !noTools;
  return (
    <section className='pg-settings-section pg-capabilities'>
      <h3>{t('模型能力')}</h3>
      <div className='pg-capability-row'>
        <span>
          <Brain size={17} />
          {t('深度思考')}
        </span>
        <select
          aria-label={t('思考强度')}
          value={noReasoning ? '' : inputs.reasoningEffort || ''}
          disabled={disabled || noReasoning}
          onChange={(e) => onInputChange('reasoningEffort', e.target.value)}
        >
          <option value=''>{noReasoning ? t('不支持') : t('模型默认')}</option>
          <option value='none'>{t('关闭（需模型支持）')}</option>
          <option value='low'>{t('低')}</option>
          <option value='medium'>{t('中')}</option>
          <option value='high'>{t('高')}</option>
        </select>
      </div>
      {inputs.reasoningEffort && !noReasoning && (
        <p className='pg-field-hint'>
          {t('通过 reasoning_effort 请求，支持的档位由模型与渠道决定。')}
        </p>
      )}
      <div className='pg-capability-row'>
        <span>
          <Wrench size={17} />
          {t('工具调用')}
        </span>
        {noTools ? (
          <small>{t('不支持')}</small>
        ) : (
          <Switch
            aria-label={t('工具调用')}
            size='small'
            checked={Boolean(toolsEnabled)}
            disabled={disabled}
            onChange={(checked) => {
              onInputChange('toolsEnabled', checked);
              if (checked && !inputs.toolsJson)
                onInputChange('toolsJson', DEFAULT_TOOL_DEFINITIONS);
            }}
          />
        )}
      </div>
      {toolsEnabled && (
        <div className='pg-tool-settings'>
          <p className='pg-field-hint'>
            {t('定义函数供模型调用，返回后手动填入执行结果。')}
          </p>
          <details open>
            <summary>
              {t('函数定义')} <span>{tools?.length || 0}</span>
              <ChevronDown size={13} />
            </summary>
            <TextArea
              aria-label={t('函数定义')}
              value={inputs.toolsJson || ''}
              disabled={disabled}
              onChange={(value) => onInputChange('toolsJson', value)}
              autosize={{ minRows: 5, maxRows: 12 }}
            />
          </details>
          {!tools && (
            <p role='alert' className='pg-field-error'>
              {t('请输入有效的函数工具数组，parameters 必须为 object schema。')}
            </p>
          )}
          <label className='pg-tool-choice'>
            {t('调用策略')}
            <select
              aria-label={t('调用策略')}
              disabled={disabled}
              value={inputs.toolChoice || 'auto'}
              onChange={(e) => onInputChange('toolChoice', e.target.value)}
            >
              <option value='auto'>{t('自动')}</option>
              <option value='required'>{t('必须调用')}</option>
              <option value='none'>{t('不调用')}</option>
              {tools?.map((tool) => (
                <option key={tool.function.name} value={tool.function.name}>
                  {tool.function.name}
                </option>
              ))}
            </select>
          </label>
          {!choiceValid && (
            <p role='alert' className='pg-field-error'>
              {t('请检查函数定义与调用策略')}
            </p>
          )}
        </div>
      )}
      <div className='pg-capability-row'>
        <span>
          <Radio size={17} />
          {t('流式输出')}
        </span>
        <Switch
          aria-label={t('流式输出')}
          checked={inputs.stream}
          onChange={(checked) => onInputChange('stream', checked)}
          size='small'
          disabled={disabled}
        />
      </div>
    </section>
  );
}
