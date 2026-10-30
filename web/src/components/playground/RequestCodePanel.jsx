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
import {
  Code2,
  Copy,
  Check,
  ChevronDown,
  ExternalLink,
  Bug,
  Terminal,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Toast } from '@douyinfe/semi-ui';
import { Link } from 'react-router-dom';
import hljs from 'highlight.js/lib/core';
import python from 'highlight.js/lib/languages/python';
import javascript from 'highlight.js/lib/languages/javascript';
import bash from 'highlight.js/lib/languages/bash';
import json from 'highlight.js/lib/languages/json';
import { copy, getServerAddress } from '../../helpers';
import {
  buildPlaygroundCode,
  convertPlaygroundPayload,
  getPlaygroundSetup,
  PLAYGROUND_CODE_PROTOCOLS,
} from '../../helpers/playgroundCode';
import { getDeclaredModelProtocols } from '../../helpers/modelProtocols';
import DebugPanel from './DebugPanel';

for (const [name, grammar] of Object.entries({
  python,
  javascript,
  bash,
  json,
}))
  hljs.registerLanguage(name, grammar);

export default function RequestCodePanel({
  payload,
  model,
  debugData,
  activeDebugTab,
  onActiveDebugTabChange,
  customRequestMode,
}) {
  const { t } = useTranslation();
  const [view, setView] = useState('code');
  const [protocol, setProtocol] = useState('chat');
  const [language, setLanguage] = useState('python');
  const [copied, setCopied] = useState(false);
  const address = getServerAddress();
  const converted = useMemo(
    () => convertPlaygroundPayload(payload, protocol),
    [payload, protocol],
  );
  const code = useMemo(
    () => buildPlaygroundCode(payload, language, address, protocol),
    [payload, language, address, protocol],
  );
  const highlighted = useMemo(
    () =>
      code && code.length < 100000
        ? hljs.highlight(code, {
            language: language === 'curl' ? 'bash' : language,
            ignoreIllegals: true,
          }).value
        : null,
    [code, language],
  );
  useEffect(() => setCopied(false), [code]);
  const setup = getPlaygroundSetup(language, protocol);
  const standard = PLAYGROUND_CODE_PROTOCOLS.find(
    (item) => item.id === protocol,
  );
  const declared = getDeclaredModelProtocols(model).includes(standard.endpoint);
  const copyText = async (text, isCode = false) => {
    if (await copy(text)) {
      if (isCode) setCopied(true);
      Toast.success(t('代码已复制到剪贴板'));
    } else Toast.error(t('复制失败，请手动选择文本复制'));
  };
  return (
    <section className='pg-code-panel' aria-label={t('请求代码')}>
      <header className='pg-panel-heading pg-code-heading'>
        <Code2 size={22} />
        <h2>{t('获取代码')}</h2>
        <Link to='/docs' target='_blank' rel='noopener noreferrer'>
          {t('文档')} <ExternalLink size={13} />
        </Link>
        <button
          className={
            'pg-code-debug-toggle' + (view === 'debug' ? ' is-active' : '')
          }
          aria-label={t('调试信息')}
          title={t('调试信息')}
          aria-pressed={view === 'debug'}
          onClick={() => setView(view === 'debug' ? 'code' : 'debug')}
        >
          <Bug size={16} />
        </button>
      </header>
      {view === 'code' ? (
        <>
          <div className='pg-code-standard-label'>{t('API 标准')}</div>
          <div
            className='pg-code-standards'
            role='group'
            aria-label={t('API 标准')}
          >
            {PLAYGROUND_CODE_PROTOCOLS.map((item) => (
              <button
                key={item.id}
                aria-pressed={protocol === item.id}
                className={protocol === item.id ? 'is-active' : ''}
                onClick={() => setProtocol(item.id)}
              >
                {item.label}
              </button>
            ))}
          </div>
          <div className='pg-code-tools'>
            <div className='pg-code-language'>
              <select
                aria-label={t('代码语言')}
                value={language}
                onChange={(event) => setLanguage(event.target.value)}
              >
                <option value='python'>Python</option>
                <option value='javascript'>JavaScript</option>
                <option value='curl'>cURL</option>
                <option value='json'>JSON</option>
              </select>
              <ChevronDown size={18} aria-hidden='true' />
            </div>
            <button
              disabled={!code}
              onClick={() => copyText(code, true)}
              aria-label={t('复制代码')}
            >
              {copied ? <Check size={18} /> : <Copy size={18} />}{' '}
              {t(copied ? '已复制' : '复制')}
            </button>
          </div>
          <div className='pg-code-body'>
            {converted.errors.length > 0 ? (
              <div className='pg-code-notice is-error' role='status'>
                <strong>{t('当前设置无法完整转换，请调整后复制。')}</strong>
                <code>{converted.errors.join(', ')}</code>
                {converted.errors.includes('tool_results') && (
                  <span>{t('请先填写工具结果，再生成下一次请求。')}</span>
                )}
              </div>
            ) : (
              <>
                {!declared && (
                  <p className='pg-code-notice'>
                    {t(
                      '该模型尚未声明支持此协议，请先确认所用分组的接口配置。',
                    )}
                  </p>
                )}
                {converted.notes.includes('max_tokens') && (
                  <p className='pg-code-notice'>
                    {t('Messages 必须设置输出上限，示例使用 4096。')}
                  </p>
                )}
                <pre
                  className='pg-request-code'
                  aria-label={t('请求代码')}
                  tabIndex={0}
                >
                  {highlighted ? (
                    <code dangerouslySetInnerHTML={{ __html: highlighted }} />
                  ) : (
                    <code>{code || t('请输入您的问题...')}</code>
                  )}
                </pre>
              </>
            )}
            {setup && (
              <details className='pg-code-setup'>
                <summary>
                  <Terminal size={15} />
                  {t('安装与运行')}
                </summary>
                <div>
                  <p>
                    {language === 'python' || language === 'javascript'
                      ? t('将代码保存为 {{filename}}，替换密钥后在终端运行：', {
                          filename:
                            language === 'python'
                              ? 'playground.py'
                              : 'playground.mjs',
                        })
                      : t('先设置密钥，再粘贴上方 cURL 命令运行。')}
                  </p>
                  <div className='pg-setup-command'>
                    <pre>{setup}</pre>
                    <button
                      aria-label={t('复制运行命令')}
                      title={t('复制运行命令')}
                      onClick={() => copyText(setup)}
                    >
                      <Copy size={15} />
                    </button>
                  </div>
                  <p>{t('以上命令适用于 macOS / Linux / WSL。')}</p>
                </div>
              </details>
            )}
          </div>
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
