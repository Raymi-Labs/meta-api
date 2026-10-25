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
import { Copy, KeyRound, ArrowUpRight } from 'lucide-react';
import {
  getModelQuickStartExamples,
  MODEL_EXAMPLE_LANGUAGES,
} from './modelQuickStartExamples';

const ModelQuickStart = ({ modelData, serverAddress, t, copyText }) => {
  const [endpoint, setEndpoint] = useState('');
  const [language, setLanguage] = useState('Python');
  const examples = useMemo(
    () =>
      getModelQuickStartExamples(
        modelData,
        serverAddress,
        typeof window === 'undefined' ? '' : window.location.origin,
        language,
      ),
    [modelData, serverAddress, language],
  );
  const current =
    examples.find((example) => example.endpoint === endpoint) || examples[0];

  return (
    <div className='model-quickstart'>
      <div className='model-quickstart-toolbar'>
        {examples.length > 0 && (
          <div
            className='model-quickstart-protocols'
            role='group'
            aria-label={t('支持协议')}
          >
            {examples.map((example) => (
              <button
                type='button'
                key={example.endpoint}
                aria-pressed={current?.endpoint === example.endpoint}
                onClick={() => setEndpoint(example.endpoint)}
              >
                {example.label}
              </button>
            ))}
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
      {current ? (
        <div className='model-quickstart-code'>
          <div className='model-quickstart-code-heading'>
            <select
              className='model-quickstart-language'
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
