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

import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  ArrowDown,
  ArrowRight,
  Check,
  CheckCheck,
  ChevronRight,
  Code2,
  Copy,
  Download,
  Globe2,
  Image,
  KeyRound,
  Layers3,
  MessageSquare,
  Monitor,
  Network,
  ShieldCheck,
  Terminal,
  Wallet,
} from 'lucide-react';
import {
  Claude,
  DeepSeek,
  Gemini,
  OpenAI,
  Qwen,
  Grok,
} from '../../helpers/providerIcons';
import { copy } from '../../helpers/clipboard';
import WorldMap from './WorldMap';
import './EnterpriseHome.css';

const providers = [
  { name: 'OpenAI', Icon: OpenAI, description: '通用对话与代码开发' },
  { name: 'Claude', Icon: Claude.Color, description: '长文理解与编程协作' },
  { name: 'Gemini', Icon: Gemini.Color, description: '多模态理解与创作' },
  { name: 'DeepSeek', Icon: DeepSeek.Color, description: '推理与问题求解' },
  { name: 'Qwen', Icon: Qwen.Color, description: '语言理解与应用开发' },
  { name: 'Grok', Icon: Grok, description: '对话与内容探索' },
];

const tools = [
  { name: 'Claude Code', Icon: Claude.Color, type: '终端编程' },
  { name: 'Codex', Icon: OpenAI, type: '代码协作' },
  { name: 'Gemini CLI', Icon: Gemini.Color, type: '命令行助手' },
];

const questions = [
  [
    '已有的 SDK 和工具可以继续用吗？',
    '兼容 OpenAI、Claude、Gemini 等常见接口。按接入文档配置地址、密钥和模型即可；不同模型支持的参数可能有所不同。',
  ],
  [
    '在哪里查看模型和价格？',
    '模型广场提供可用模型、接口类型和价格信息。登录后可查看账号可用分组，具体费用以模型、分组和实际用量为准。',
  ],
  [
    '如何管理 API Key 和用量？',
    '在控制台创建和管理令牌，按项目设置额度、模型限制与有效期，并通过使用日志查看调用和费用。',
  ],
  [
    '可以在手机和电脑上使用吗？',
    '下载页提供桌面客户端和安卓公开测试版，具体系统要求、可用版本与安装说明以下载页为准。',
  ],
];

function CopyButton({ value, onCopy, label, t, className = '' }) {
  const [result, setResult] = useState('');
  useEffect(() => {
    setResult('');
  }, [value]);
  useEffect(() => {
    if (!result) return;
    const timer = window.setTimeout(() => setResult(''), 2500);
    return () => window.clearTimeout(timer);
  }, [result]);
  const handleCopy = async () => {
    try {
      const success = onCopy ? await onCopy() : await copy(value);
      setResult(success ? '已复制' : '复制失败，请手动选择');
    } catch {
      setResult('复制失败，请手动选择');
    }
  };
  return (
    <button
      type='button'
      className={`yh-copy ${className}`}
      onClick={handleCopy}
      aria-label={t(label)}
    >
      {result === '已复制' ? <Check size={16} /> : <Copy size={16} />}
      <span aria-live='polite'>{t(result || label)}</span>
    </button>
  );
}

const EnterpriseHome = ({
  brandName,
  brandLogo,
  currentEndpoint,
  endpointItems,
  handleCopyEndpoint,
  handleOpenDocs,
  serverAddress,
  setEndpointIndex,
  t,
}) => {
  const [codeLanguage, setCodeLanguage] = useState('Python');
  const baseUrl = `${serverAddress.replace(/\/+$/, '')}/v1`;
  const code =
    codeLanguage === 'Python'
      ? `import os
from openai import OpenAI

client = OpenAI(
    base_url=${JSON.stringify(baseUrl)},
    api_key=os.environ["API_KEY"],
)

response = client.chat.completions.create(
    model="MODEL_ID",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`
      : `curl ${JSON.stringify(`${baseUrl}/chat/completions`)} \\
  -H "Authorization: Bearer $API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "MODEL_ID",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`;
  const docsAction = handleOpenDocs ? (
    <button type='button' className='yh-text-link' onClick={handleOpenDocs}>
      {t('阅读 API 文档')} <ArrowRight size={16} />
    </button>
  ) : (
    <Link className='yh-text-link' to='/docs'>
      {t('阅读 API 文档')} <ArrowRight size={16} />
    </Link>
  );

  return (
    <main className='home-enterprise'>
      <section className='yh-hero' aria-labelledby='hero-title'>
        <WorldMap />
        <div className='yh-container yh-hero-inner'>
          <div className='yh-eyebrow'>
            <span />
            {brandName} · {t('让好模型，为你所用')}
          </div>
          <h1 id='hero-title'>
            {t('一个 API，')}
            <span>{t('连接万千可能。')}</span>
          </h1>
          <p className='yh-hero-description'>
            {t('用熟悉的工具，调用你需要的模型。')}
            <br className='yh-mobile-break' />
            {t('从第一行代码，到下一个好产品。')}
          </p>
          <div className='yh-actions'>
            <Link className='yh-btn yh-btn-primary' to='/console'>
              {t('开始构建')} <ArrowRight size={17} />
            </Link>
            <Link className='yh-btn yh-btn-secondary' to='/pricing'>
              {t('浏览模型')} <ChevronRight size={17} />
            </Link>
          </div>
          <ul className='yh-hero-benefits'>
            <li>
              <Layers3 size={16} />
              {t('多模型接入')}
            </li>
            <li>
              <Code2 size={16} />
              {t('兼容主流 SDK')}
            </li>
            <li>
              <Wallet size={16} />
              {t('按量计费')}
            </li>
          </ul>
        </div>
        <div className='yh-container yh-capabilities'>
          <article className='yh-capability'>
            <h2>{t('一个密钥，多种可能')}</h2>
            <p>{t('从文本到图像，让创作自由切换。')}</p>
            <div className='yh-key-visual' aria-hidden='true'>
              <div className='yh-provider-pill'>
                {providers.slice(0, 5).map(({ name, Icon }) => (
                  <Icon key={name} size={25} />
                ))}
              </div>
              <div className='yh-connector' />
              <div className='yh-key-pill'>
                <KeyRound size={15} />
                <span>sk-••••••••••••</span>
              </div>
              <div className='yh-connector' />
              <div className='yh-modality-pill'>
                <MessageSquare size={18} />
                <Code2 size={18} />
                <Image size={18} />
              </div>
            </div>
          </article>
          <article className='yh-capability'>
            <h2>{t('熟悉的 SDK，更简单的接入')}</h2>
            <p>{t('配置地址与密钥，即可开始。')}</p>
            <div className='yh-sdk-visual' aria-hidden='true'>
              <div>
                <span>01</span>
                <code>client = OpenAI(</code>
              </div>
              <div className='yh-code-highlight'>
                <span>02</span>
                <code>
                  {' '}
                  base_url=<b>"{baseUrl}"</b>,
                </code>
              </div>
              <div>
                <span>03</span>
                <code>
                  {' '}
                  api_key=<b>"YOUR_API_KEY"</b>,
                </code>
              </div>
              <div>
                <span>04</span>
                <code>)</code>
              </div>
              <div className='yh-sdk-caption'>OpenAI · Anthropic · Gemini</div>
            </div>
          </article>
          <article className='yh-capability'>
            <h2>{t('多条线路，一个入口')}</h2>
            <p>{t('统一调度，让应用专注业务。')}</p>
            <div className='yh-route-visual' aria-hidden='true'>
              <div className='yh-route-labels'>
                <span>OpenAI</span>
                <span>Claude</span>
                <span>Gemini</span>
              </div>
              <svg viewBox='0 0 58 105'>
                <path d='M0 14 Q30 14 58 52M0 52H58M0 90Q30 90 58 52' />
              </svg>
              <div className='yh-route-hub'>
                <img src={brandLogo} alt='' />
              </div>
              <svg viewBox='0 0 35 105'>
                <path d='M0 52H35' />
              </svg>
              <div className='yh-route-app'>
                <Layers3 size={19} />
                <span>{t('你的应用')}</span>
              </div>
            </div>
          </article>
        </div>
      </section>

      <section
        className='yh-container yh-desktop-section'
        aria-labelledby='desktop-title'
      >
        <div className='yh-section-copy'>
          <span className='yh-section-kicker'>
            <Monitor size={16} />
            {t('不止于 API')}
          </span>
          <h2 id='desktop-title'>
            {t('你的 AI 工具，')}
            <br />
            {t('在这里汇合。')}
          </h2>
          <p>
            {t(
              '在电脑上管理 AI 工具，在手机上随时对话。把接入的时间，留给真正想做的事。',
            )}
          </p>
          <ul className='yh-check-list'>
            <li>
              <CheckCheck size={17} />
              {t('统一管理客户端、模型与分组')}
            </li>
            <li>
              <CheckCheck size={17} />
              {t('沿用熟悉的开发方式')}
            </li>
          </ul>
          <Link className='yh-btn yh-btn-primary' to='/download'>
            <Download size={17} />
            {t('下载客户端')}
          </Link>
          <span className='yh-platform-note'>macOS · Windows · Android</span>
        </div>
        <div className='yh-desktop-art'>
          <div className='yh-app-window'>
            <div className='yh-window-chrome'>
              <div className='yh-window-dots'>
                <i />
                <i />
                <i />
              </div>
              <span>{brandName} Desktop</span>
              <span className='yh-demo-label'>{t('接入示意')}</span>
            </div>
            <div className='yh-app-body'>
              <aside className='yh-app-sidebar' aria-hidden='true'>
                <img src={brandLogo} alt='' />
                <span className='active'>
                  <Layers3 size={19} />
                </span>
                <Terminal size={19} />
                <KeyRound size={19} />
                <Wallet size={19} />
              </aside>
              <div className='yh-app-content'>
                <div className='yh-app-heading'>
                  <div>
                    <small>WORKSPACE</small>
                    <h3>{t('我的 AI 工具')}</h3>
                  </div>
                  <span className='yh-app-count'>03</span>
                </div>
                {tools.map(({ name, Icon, type }) => (
                  <div className='yh-tool-row' key={name}>
                    <span className='yh-tool-icon' aria-hidden='true'>
                      <Icon size={26} />
                    </span>
                    <div>
                      <strong>{name}</strong>
                      <span>{t(type)}</span>
                    </div>
                    <ChevronRight size={16} />
                  </div>
                ))}
                <div className='yh-workspace-footer'>
                  <ShieldCheck size={15} />
                  {t('一个工作区，连接你的工作流')}
                </div>
              </div>
            </div>
          </div>
          <div className='yh-floating-label'>
            <Globe2 size={17} />
            <span>{t('从灵感，到实现')}</span>
            <ArrowRight size={16} />
          </div>
        </div>
      </section>

      <section className='yh-model-section' aria-labelledby='models-title'>
        <div className='yh-container'>
          <div className='yh-section-heading'>
            <div>
              <span className='yh-section-kicker'>MODEL ECOSYSTEM</span>
              <h2 id='models-title'>{t('好模型，不必只选一个。')}</h2>
              <p>
                {t(
                  '为不同任务，找到合适的模型。具体可用模型与价格，以模型广场为准。',
                )}
              </p>
            </div>
            <Link className='yh-text-link' to='/pricing'>
              {t('查看模型与价格')} <ArrowRight size={16} />
            </Link>
          </div>
          <div className='yh-model-grid'>
            {providers.map(({ name, Icon, description }) => (
              <Link className='yh-model-card' key={name} to='/pricing'>
                <span className='yh-model-icon' aria-hidden='true'>
                  <Icon size={30} />
                </span>
                <div>
                  <h3>{name}</h3>
                  <p>{t(description)}</p>
                </div>
                <ArrowRight className='yh-model-arrow' size={17} />
              </Link>
            ))}
          </div>
        </div>
      </section>

      <section
        className='yh-container yh-developer-section'
        id='quick-start'
        aria-labelledby='developer-title'
      >
        <div className='yh-section-copy'>
          <span className='yh-section-kicker'>
            <Terminal size={16} />
            BUILT FOR DEVELOPERS
          </span>
          <h2 id='developer-title'>
            {t('少一点配置，')}
            <br />
            {t('多一点创造。')}
          </h2>
          <p>{t('保留你的开发习惯，从一次简单的 API 调用开始。')}</p>
          <ol className='yh-steps'>
            <li>
              <span>01</span>
              <div>
                <Link to='/console/token'>
                  {t('创建 API Key')} <ArrowRight size={14} />
                </Link>
                <p>{t('在控制台创建令牌，按需设置额度。')}</p>
              </div>
            </li>
            <li>
              <span>02</span>
              <div>
                <strong>{t('配置接入地址')}</strong>
                <p>{t('将示例中的 API_KEY 和 MODEL_ID 替换为你的配置。')}</p>
              </div>
            </li>
            <li>
              <span>03</span>
              <div>
                <strong>{t('发起第一次调用')}</strong>
                <p>{t('在使用日志中查看请求和费用。')}</p>
              </div>
            </li>
          </ol>
          {docsAction}
        </div>
        <div className='yh-code-panel'>
          <div className='yh-code-toolbar'>
            <div
              className='yh-code-tabs'
              role='group'
              aria-label={t('代码语言')}
            >
              {['Python', 'cURL'].map((lang) => (
                <button
                  key={lang}
                  type='button'
                  aria-pressed={lang === codeLanguage}
                  onClick={() => setCodeLanguage(lang)}
                >
                  {lang}
                </button>
              ))}
            </div>
            <CopyButton value={code} label='复制代码' t={t} />
          </div>
          <pre className='yh-code-block'>
            <code>
              {code.split('\n').map((line, index) => (
                <span
                  className={
                    line.includes('base_url') || line.startsWith('curl ')
                      ? 'yh-code-focus'
                      : ''
                  }
                  key={`${codeLanguage}-${index}`}
                >
                  <i aria-hidden='true'>{String(index + 1).padStart(2, '0')}</i>
                  {line || ' '}
                </span>
              ))}
            </code>
          </pre>
          <p className='yh-code-note'>
            {t('示例使用 Chat Completions，其他接口请参阅文档。')}
          </p>
          <div className='yh-endpoint-picker'>
            <label htmlFor='yh-endpoint'>{t('接口地址')}</label>
            <select
              id='yh-endpoint'
              value={currentEndpoint}
              onChange={(event) => {
                const index = endpointItems.findIndex(
                  (item) => item.value === event.target.value,
                );
                if (index >= 0) setEndpointIndex(index);
              }}
            >
              {endpointItems.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.value}
                </option>
              ))}
            </select>
            <CopyButton
              value={`${serverAddress}${currentEndpoint}`}
              onCopy={handleCopyEndpoint}
              label='复制完整地址'
              t={t}
            />
          </div>
        </div>
      </section>

      <section
        className='yh-container yh-faq-section'
        aria-labelledby='faq-title'
      >
        <div>
          <span className='yh-section-kicker'>GOOD TO KNOW</span>
          <h2 id='faq-title'>
            {t('开始之前，')}
            <br />
            {t('你可能想了解。')}
          </h2>
          {docsAction}
        </div>
        <div className='yh-faq-list'>
          {questions.map(([question, answer]) => (
            <details key={question}>
              <summary>
                {t(question)}
                <span>
                  <ArrowDown size={17} />
                </span>
              </summary>
              <p>{t(answer)}</p>
            </details>
          ))}
        </div>
      </section>

      <section className='yh-container yh-final-cta'>
        <div className='yh-cta-symbol' aria-hidden='true'>
          <Network size={29} />
        </div>
        <h2>{t('下一个好想法，从这里开始。')}</h2>
        <p>{t('模型已经就位，只等你的创意。')}</p>
        <Link className='yh-btn yh-btn-primary' to='/console'>
          {t('开始构建')} <ArrowRight size={17} />
        </Link>
      </section>
    </main>
  );
};

export default EnterpriseHome;
