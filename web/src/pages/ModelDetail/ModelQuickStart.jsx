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

import React, { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Copy, KeyRound, ArrowUpRight, ChevronDown } from 'lucide-react';
import {
  getModelQuickStartExamples,
  MODEL_EXAMPLE_LANGUAGES,
} from './modelQuickStartExamples';

const ModelQuickStart = ({
  modelData,
  initialEndpoint = '',
  serverAddress,
  t,
  copyText,
}) => {
  const [selection, setSelection] = useState(null);
  const endpoint =
    selection?.modelName === modelData.model_name
      ? selection.endpoint
      : initialEndpoint;
  const [language, setLanguage] = useState('Python');
  const examples = useMemo(
    () =>
      getModelQuickStartExamples(
        modelData,
        serverAddress,
        typeof window === 'undefined' ? '' : window.location.origin,
        language,
        { includeTextProtocolTemplates: true },
      ),
    [modelData, serverAddress, language],
  );
  const current =
    examples.find((example) => example.endpoint === endpoint) ||
    examples.find((example) => example.declared) ||
    examples[0];

  return (
    <div className='model-quickstart'>
      <div className='model-quickstart-toolbar'>
        {examples.length > 0 && (
          <div className='model-quickstart-select'>
            <select
              aria-label={t('接口协议')}
              value={current.endpoint}
              onChange={(event) =>
                setSelection({
                  modelName: modelData.model_name,
                  endpoint: event.target.value,
                })
              }
            >
              {examples.map((example) => (
                <option key={example.endpoint} value={example.endpoint}>
                  {example.label}
                  {!example.declared && ` · ${t('未声明')}`}
                </option>
              ))}
            </select>
            <ChevronDown size={16} aria-hidden='true' />
          </div>
        )}
        <div className='model-quickstart-actions'>
          <Link to='/console/token'>
            <KeyRound size={15} aria-hidden='true' />
            {t('创建 API Key')}
          </Link>
          <Link to='/docs/api-examples'>
            {t('API 文档')}
            <ArrowUpRight size={15} aria-hidden='true' />
          </Link>
        </div>
      </div>
      {current && !current.declared && (
        <p className='model-quickstart-note' role='status'>
          {t('该模型尚未声明支持此协议，请先确认所用分组的接口配置。')}
        </p>
      )}
      {current ? (
        <div className='model-quickstart-code'>
          <div className='model-quickstart-code-heading'>
            <div className='model-quickstart-select'>
              <select
                aria-label={t('代码语言')}
                value={language}
                onChange={(event) => setLanguage(event.target.value)}
              >
                {MODEL_EXAMPLE_LANGUAGES.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
              <ChevronDown size={16} aria-hidden='true' />
            </div>
            <button type='button' onClick={() => copyText(current.code)}>
              <Copy size={15} aria-hidden='true' />
              {t('复制代码')}
            </button>
          </div>
          <pre tabIndex={0} aria-label={`${language} · ${current.label}`}>
            <code>{current.code}</code>
          </pre>
        </div>
      ) : (
        <p className='model-quickstart-note'>
          {t('此模型的调用示例请参阅 API 文档。')}
        </p>
      )}
    </div>
  );
};

export default ModelQuickStart;
