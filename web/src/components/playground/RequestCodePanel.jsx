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

import React, { useMemo, useState, useEffect } from 'react';
import { Code2, Copy, Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Toast } from '@douyinfe/semi-ui';
import { Link } from 'react-router-dom';
import { copy, getServerAddress } from '../../helpers';
import { buildPlaygroundCode } from '../../helpers/playgroundRequest';
import DebugPanel from './DebugPanel';

export default function RequestCodePanel({
  payload,
  debugData,
  activeDebugTab,
  onActiveDebugTabChange,
  customRequestMode,
}) {
  const { t } = useTranslation();
  const [view, setView] = useState('code');
  const [language, setLanguage] = useState('python');
  const [copied, setCopied] = useState(false);
  const address = getServerAddress();
  const code = useMemo(
    () => buildPlaygroundCode(payload, language, address),
    [payload, language, address],
  );
  useEffect(() => setCopied(false), [code]);
  const hasContent = Boolean(
    payload?.model && payload?.messages?.length && code,
  );
  const copyCode = async () => {
    if (await copy(code)) {
      setCopied(true);
      Toast.success(t('代码已复制到剪贴板'));
    } else Toast.error(t('复制失败，请手动选择文本复制'));
  };
  return (
    <section className='pg-code-panel' aria-label={t('请求代码')}>
      <header className='pg-panel-heading'>
        <Code2 size={18} />
        <h2>{t('请求代码')}</h2>
        <Link to='/docs'>{t('文档')}</Link>
      </header>
      <div className='pg-segments'>
        <button
          className={view === 'code' ? 'is-active' : ''}
          onClick={() => setView('code')}
        >
          {t('请求代码')}
        </button>
        <button
          className={view === 'debug' ? 'is-active' : ''}
          onClick={() => setView('debug')}
        >
          {t('调试信息')}
        </button>
      </div>
      {view === 'code' ? (
        <>
          <div className='pg-code-protocol'>
            Chat Completions <span>/v1/chat/completions</span>
          </div>
          <div className='pg-code-tools'>
            <select
              aria-label={t('代码语言')}
              value={language}
              onChange={(event) => {
                setLanguage(event.target.value);
                setCopied(false);
              }}
            >
              <option value='python'>Python</option>
              <option value='curl'>cURL</option>
              <option value='javascript'>JavaScript</option>
              <option value='json'>JSON</option>
            </select>
            <button
              disabled={!hasContent}
              onClick={copyCode}
              title={t('复制代码')}
              aria-label={t('复制代码')}
            >
              {copied ? <Check size={15} /> : <Copy size={15} />} {t('复制')}
            </button>
          </div>
          <pre
            className='pg-request-code'
            aria-label={t('请求代码')}
            tabIndex={0}
          >
            <code>{hasContent ? code : t('请输入您的问题...')}</code>
          </pre>
          <p className='pg-code-hint'>
            {t('代码使用 API 令牌，请选择与当前分组一致的令牌。')}{' '}
            <Link to='/console/token'>{t('令牌管理')}</Link>
          </p>
        </>
      ) : (
        <div className='pg-debug-view'>
          <DebugPanel
            debugData={debugData}
            activeDebugTab={activeDebugTab}
            onActiveDebugTabChange={onActiveDebugTabChange}
            styleState={{ isMobile: false }}
            customRequestMode={customRequestMode}
          />
        </div>
      )}
    </section>
  );
}
