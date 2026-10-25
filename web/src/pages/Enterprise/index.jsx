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

import React, { useContext, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AppWindow,
  ArrowDown,
  ArrowRight,
  ArrowUpRight,
  BarChart3,
  Bot,
  Check,
  ChevronDown,
  Cloud,
  Code2,
  Copy,
  Gauge,
  Handshake,
  KeyRound,
  Layers3,
  Mail,
  Minus,
  Network,
  RotateCcw,
  Shapes,
  SlidersHorizontal,
  Terminal,
  Users,
  WifiOff,
} from 'lucide-react';
import { StatusContext } from '../../context/Status';
import { usePublicTranslation } from '../../helpers/publicLocale';
import { getLogo, getSystemName } from '../../helpers/storage';
import {
  Claude,
  DeepSeek,
  Gemini,
  Grok,
  OpenAI,
  Qwen,
} from '../../helpers/providerIcons';
import { getPublicHomeServerAddress } from '../Home/homeAddress';
import {
  createEnterpriseExample,
  ENTERPRISE_CONTACT,
  ENTERPRISE_PROTOCOLS,
} from './enterpriseData';
import './Enterprise.css';

const providers = [
  { name: 'OpenAI', Icon: OpenAI },
  { name: 'Claude', Icon: Claude.Color },
  { name: 'Gemini', Icon: Gemini.Color },
  { name: 'DeepSeek', Icon: DeepSeek.Color },
  { name: 'Qwen', Icon: Qwen.Color },
  { name: 'Grok', Icon: Grok },
];

function CopyAction({ value, label, t, compact = false }) {
  const [result, setResult] = useState('');
  const sequence = useRef(0);
  useEffect(() => {
    sequence.current += 1;
    setResult('');
    return () => {
      sequence.current += 1;
    };
  }, [value]);
  useEffect(() => {
    if (!result) return;
    const timer = window.setTimeout(() => setResult(''), 3000);
    return () => window.clearTimeout(timer);
  }, [result]);
  const handleCopy = async () => {
    const current = ++sequence.current;
    try {
      await navigator.clipboard.writeText(value);
      if (current === sequence.current) setResult('success');
    } catch {
      if (current === sequence.current) setResult('failure');
    }
  };
  return (
    <div className={`enterprise-copy${compact ? ' is-compact' : ''}`}>
      <button type='button' onClick={handleCopy} aria-label={label}>
        {result === 'success' ? (
          <Check size={15} aria-hidden='true' />
        ) : (
          <Copy size={15} aria-hidden='true' />
        )}
        <span>{result === 'success' ? t('已复制') : label}</span>
      </button>
      <span className='enterprise-copy-feedback' role='status'>
        {result === 'failure' ? t('复制失败，请手动选择') : ''}
      </span>
    </div>
  );
}

function CodeLine({ line }) {
  const parts = line.split(
    /("[^"\n]*"|'[^'\n]*'|\b(?:import|from|print|curl)\b|\b\d+\b)/g,
  );
  return parts.map((part, index) => {
    const kind = /^['"]/.test(part)
      ? 'string'
      : /^(import|from|print|curl)$/.test(part)
        ? 'keyword'
        : /^\d+$/.test(part)
          ? 'number'
          : '';
    return (
      <span
        key={index}
        className={kind ? `enterprise-code-${kind}` : undefined}
      >
        {part}
      </span>
    );
  });
}

function ExamplePanel({ baseUrl, t }) {
  const [protocol, setProtocol] = useState('chat');
  const [language, setLanguage] = useState('Python');
  const tabRefs = useRef([]);
  const activeProtocol = ENTERPRISE_PROTOCOLS.find(
    (item) => item.id === protocol,
  );
  const code = createEnterpriseExample(protocol, language, baseUrl);
  const handleTabKey = (event, index) => {
    let next;
    if (event.key === 'ArrowRight')
      next = (index + 1) % ENTERPRISE_PROTOCOLS.length;
    if (event.key === 'ArrowLeft')
      next =
        (index - 1 + ENTERPRISE_PROTOCOLS.length) % ENTERPRISE_PROTOCOLS.length;
    if (event.key === 'Home') next = 0;
    if (event.key === 'End') next = ENTERPRISE_PROTOCOLS.length - 1;
    if (next === undefined) return;
    event.preventDefault();
    setProtocol(ENTERPRISE_PROTOCOLS[next].id);
    tabRefs.current[next]?.focus();
  };
  return (
    <div className='enterprise-code-scene'>
      <div className='enterprise-code-orbit' aria-hidden='true' />
      <div className='enterprise-code-panel'>
        <div className='enterprise-code-topline'>
          <span className='enterprise-window-dots' aria-hidden='true'>
            <i />
            <i />
            <i />
          </span>
          <span>
            <Terminal size={13} aria-hidden='true' /> API
          </span>
        </div>
        <div
          className='enterprise-protocols'
          role='tablist'
          aria-label={t('接口协议')}
        >
          {ENTERPRISE_PROTOCOLS.map((item, index) => (
            <button
              key={item.id}
              ref={(node) => {
                tabRefs.current[index] = node;
              }}
              type='button'
              role='tab'
              id={`enterprise-tab-${item.id}`}
              aria-controls='enterprise-code-example'
              aria-selected={protocol === item.id}
              tabIndex={protocol === item.id ? 0 : -1}
              onClick={() => setProtocol(item.id)}
              onKeyDown={(event) => handleTabKey(event, index)}
            >
              {item.name}
            </button>
          ))}
        </div>
        <div className='enterprise-code-toolbar'>
          <label className='enterprise-language-select'>
            <span className='enterprise-sr-only'>{t('代码语言')}</span>
            <select
              value={language}
              onChange={(event) => setLanguage(event.target.value)}
            >
              <option value='Python'>Python</option>
              <option value='cURL'>cURL</option>
            </select>
            <ChevronDown size={13} aria-hidden='true' />
          </label>
          <CopyAction value={code} label={t('复制代码')} t={t} compact />
        </div>
        <div
          id='enterprise-code-example'
          role='tabpanel'
          aria-labelledby={`enterprise-tab-${protocol}`}
        >
          <pre tabIndex={0} aria-label={`${activeProtocol.name} · ${language}`}>
            <code>
              {code.split('\n').map((line, index) => (
                <span className='enterprise-code-line' key={index}>
                  <span
                    className='enterprise-code-line-number'
                    aria-hidden='true'
                  >
                    {String(index + 1).padStart(2, '0')}
                  </span>
                  <span>
                    <CodeLine line={line} />
                    {'\n'}
                  </span>
                </span>
              ))}
            </code>
          </pre>
        </div>
        <div className='enterprise-code-endpoint'>
          <span className='enterprise-status-dot' aria-hidden='true' />
          <code>{baseUrl}</code>
          <Code2 size={15} aria-hidden='true' />
        </div>
      </div>
      <p className='enterprise-code-note'>
        {t(
          '示例仅展示接口结构，请替换密钥和模型；协议及参数以实际模型支持为准。',
        )}
      </p>
    </div>
  );
}

function RoutingDemo({ t, brandName, brandLogo }) {
  const [failed, setFailed] = useState(false);
  return (
    <div className={`enterprise-routing${failed ? ' is-failed' : ''}`}>
      <div className='enterprise-routing-toolbar'>
        <span className='enterprise-eyebrow'>
          <Network size={15} aria-hidden='true' />
          {t('接入示意')}
        </span>
        <button
          type='button'
          className='enterprise-demo-button'
          aria-pressed={failed}
          onClick={() => setFailed((current) => !current)}
        >
          {failed ? (
            <RotateCcw size={15} aria-hidden='true' />
          ) : (
            <WifiOff size={15} aria-hidden='true' />
          )}
          {failed ? t('恢复主线路') : t('模拟主线路故障')}
        </button>
      </div>
      <div className='enterprise-routing-graph'>
        <svg
          className='enterprise-routing-lines'
          viewBox='0 0 900 300'
          preserveAspectRatio='none'
          aria-hidden='true'
        >
          <path d='M170 88 C310 88 305 150 450 150' data-active={!failed} />
          <path d='M170 212 C310 212 305 150 450 150' data-active={failed} />
          <path d='M450 150 C600 150 590 62 735 62' data-active='true' />
          <path d='M450 150 L735 150' data-active='true' />
          <path d='M450 150 C600 150 590 238 735 238' data-active='true' />
        </svg>
        <div className='enterprise-route-sources'>
          <div
            className={`enterprise-route-node enterprise-route-primary${!failed ? ' is-active' : ''}`}
          >
            <div>
              <OpenAI size={24} />
              <strong>OpenAI</strong>
              {failed && <WifiOff size={16} aria-hidden='true' />}
            </div>
            <span>{t('优先调用线路')}</span>
          </div>
          <div className={`enterprise-route-node${failed ? ' is-active' : ''}`}>
            <div>
              <Cloud size={24} aria-hidden='true' />
              <strong>Azure</strong>
            </div>
            <span>{t('备用调用线路')}</span>
          </div>
        </div>
        <ArrowDown
          className='enterprise-route-mobile-arrow'
          size={22}
          aria-hidden='true'
        />
        <div className='enterprise-route-hub'>
          <div className='enterprise-route-hub-ring'>
            <div>
              <img src={brandLogo} alt={brandName} width='42' height='42' />
            </div>
          </div>
          <strong>{t('配置路由')}</strong>
          <code>MODEL_ID</code>
        </div>
        <ArrowDown
          className='enterprise-route-mobile-arrow'
          size={22}
          aria-hidden='true'
        />
        <div className='enterprise-route-consumers'>
          <span>
            <AppWindow size={20} aria-hidden='true' />
            {t('你的应用')}
          </span>
          <span>
            <Users size={20} aria-hidden='true' />
            {t('团队')}
          </span>
          <span>
            <Bot size={20} aria-hidden='true' />
            {t('智能体')}
          </span>
        </div>
      </div>
      <p className='enterprise-routing-status' role='status'>
        <span className='enterprise-status-dot' aria-hidden='true' />
        {failed ? t('主线路中断，使用备用线路') : t('主线路可用')}
      </p>
      <p className='enterprise-routing-note'>
        {t('示意动画，非实时监控。实际切换取决于渠道配置和请求结果。')}
      </p>
    </div>
  );
}

export default function Enterprise() {
  const { t, language } = usePublicTranslation();
  const [statusState] = useContext(StatusContext);
  const brandName = getSystemName();
  const brandLogo = getLogo();
  const serverAddress = getPublicHomeServerAddress(
    statusState?.status?.server_address || 'https://meta-api.vip',
    'https://meta-api.vip',
  );
  const baseUrl = `${serverAddress}/v1`;
  const contactHref = `mailto:${ENTERPRISE_CONTACT}?subject=${encodeURIComponent(t('企业接入咨询'))}`;

  useEffect(() => {
    const previousTitle = document.title;
    document.title = `${t('企业定制')} · ${brandName}`;
    return () => {
      document.title = previousTitle;
    };
  }, [brandName, language, t]);

  const comparison = [
    {
      label: t('接口接入'),
      separate: t('各自维护地址、鉴权与请求格式。'),
      unified: t('复用兼容 SDK，以统一地址接入不同协议。'),
    },
    {
      label: t('调用管理'),
      separate: t('在不同后台分散管理密钥与用量。'),
      unified: t('集中管理令牌、额度和本人调用记录。'),
    },
    {
      label: t('线路配置'),
      separate: t('自行维护渠道切换与异常处理。'),
      unified: t('按已配置的优先级、权重和备用组分配请求。'),
    },
  ];
  const capabilities = [
    {
      Icon: Code2,
      title: t('多协议兼容'),
      description: t(
        'Chat Completions、Messages 与 Responses，按模型选择合适的接口。',
      ),
    },
    {
      Icon: KeyRound,
      title: t('令牌权限与额度'),
      description: t('按令牌设置模型范围、额度和有效期，区分项目调用。'),
    },
    {
      Icon: Gauge,
      title: t('请求与并发限制'),
      description: t('按配置控制请求窗口和并发，让接入节奏更可控。'),
    },
    {
      Icon: BarChart3,
      title: t('用量与费用记录'),
      description: t('在控制台查看本人调用、消耗与计费明细。'),
    },
    {
      Icon: Shapes,
      title: t('多模态调用'),
      description: t('为文本、图像、音频等任务选择对应模型和接口。'),
    },
    {
      Icon: SlidersHorizontal,
      title: t('配置与故障处理'),
      description: t('结合渠道优先级、权重、备用组、重试及熔断配置管理线路。'),
    },
  ];
  const services = [
    {
      Icon: AppWindow,
      title: t('产品与应用团队'),
      description: t(
        '梳理模型能力、接口形式和调用流程，让产品尽快完成接入验证。',
      ),
    },
    {
      Icon: Terminal,
      title: t('开发与智能体工具'),
      description: t(
        '围绕 SDK、编程助手与智能体工具，确认地址、密钥和协议配置。',
      ),
    },
    {
      Icon: Handshake,
      title: t('用量与业务协作'),
      description: t(
        '结合预计用量、计费方式和已有业务流程，沟通适合的合作方案。',
      ),
    },
  ];

  return (
    <main className='enterprise-page'>
      <section className='enterprise-hero' aria-labelledby='enterprise-title'>
        <div className='enterprise-container enterprise-hero-grid'>
          <div className='enterprise-hero-copy'>
            <div className='enterprise-eyebrow enterprise-hero-eyebrow'>
              <span className='enterprise-status-dot' aria-hidden='true' />
              {t('面向团队的模型接入')}
            </div>
            <h1 id='enterprise-title'>
              {t('让 AI 成为')}
              <span>{t('业务的下一步。')}</span>
            </h1>
            <p className='enterprise-hero-description'>
              {t(
                '从模型接入到调用管理，用统一 API 连接业务、开发工具与智能体。',
              )}
            </p>
            <div className='enterprise-actions'>
              <a
                href={contactHref}
                className='enterprise-button enterprise-button-primary'
              >
                {t('发送邮件咨询')}
                <ArrowUpRight size={19} aria-hidden='true' />
              </a>
              <Link
                to='/docs/api-examples'
                className='enterprise-button enterprise-button-secondary'
              >
                {t('文档')}
                <ArrowRight size={17} aria-hidden='true' />
              </Link>
            </div>
            <div className='enterprise-hero-benefits'>
              <span>
                <Layers3 size={15} aria-hidden='true' />
                {t('多模型接入')}
              </span>
              <span>
                <Code2 size={15} aria-hidden='true' />
                {t('兼容主流 SDK')}
              </span>
              <span>
                <BarChart3 size={15} aria-hidden='true' />
                {t('按量计费')}
              </span>
            </div>
          </div>
          <ExamplePanel baseUrl={baseUrl} t={t} />
        </div>
      </section>

      <section
        className='enterprise-container enterprise-providers'
        aria-labelledby='enterprise-providers-title'
      >
        <p id='enterprise-providers-title'>
          {t('熟悉的模型，统一的接入方式。')}
        </p>
        <div className='enterprise-provider-strip'>
          {providers.map(({ name, Icon }) => (
            <div key={name}>
              <Icon size={29} />
              <span>{name}</span>
            </div>
          ))}
        </div>
        <Link to='/pricing' className='enterprise-text-link'>
          {t('查看模型与价格')}
          <ArrowUpRight size={14} aria-hidden='true' />
        </Link>
      </section>

      <section
        className='enterprise-section enterprise-comparison-section'
        aria-labelledby='enterprise-comparison-title'
      >
        <div className='enterprise-container'>
          <div className='enterprise-section-heading'>
            <span className='enterprise-section-index' aria-hidden='true'>
              01 /
            </span>
            <h2 id='enterprise-comparison-title'>
              {t('少一些重复接入，多一些产品进展。')}
            </h2>
            <p>{t('把分散的配置，收拢到清晰的调用流程。')}</p>
          </div>
          <div className='enterprise-comparison-grid'>
            <article className='enterprise-comparison-card'>
              <h3>
                <Layers3 size={20} aria-hidden='true' />
                {t('分别对接供应商')}
              </h3>
              <div className='enterprise-separate-visual' aria-hidden='true'>
                {[OpenAI, Claude.Color, Gemini.Color].map((Icon, index) => (
                  <span key={index}>
                    <Icon size={24} />
                    <i />
                    <Code2 size={18} />
                  </span>
                ))}
              </div>
              {comparison.map((row) => (
                <div className='enterprise-comparison-row' key={row.label}>
                  <Minus size={16} aria-hidden='true' />
                  <div>
                    <strong>{row.label}</strong>
                    <p>{row.separate}</p>
                  </div>
                </div>
              ))}
            </article>
            <article className='enterprise-comparison-card is-unified'>
              <h3>
                <img src={brandLogo} alt='' width='24' height='24' />
                {t('通过统一平台接入')}
              </h3>
              <div className='enterprise-unified-visual' aria-hidden='true'>
                <span className='enterprise-mini-providers'>
                  <OpenAI size={21} />
                  <Claude.Color size={21} />
                  <Gemini.Color size={21} />
                </span>
                <i />
                <span className='enterprise-mini-hub'>
                  <img src={brandLogo} alt='' width='28' height='28' />
                </span>
                <i />
                <Code2 size={25} />
              </div>
              {comparison.map((row) => (
                <div className='enterprise-comparison-row' key={row.label}>
                  <Check size={16} aria-hidden='true' />
                  <div>
                    <strong>{row.label}</strong>
                    <p>{row.unified}</p>
                  </div>
                </div>
              ))}
            </article>
          </div>
        </div>
      </section>

      <section
        className='enterprise-section'
        aria-labelledby='enterprise-capabilities-title'
      >
        <div className='enterprise-container'>
          <div className='enterprise-section-heading'>
            <span className='enterprise-section-index' aria-hidden='true'>
              02 /
            </span>
            <h2 id='enterprise-capabilities-title'>
              {t('为实际业务准备的接入能力')}
            </h2>
          </div>
          <div className='enterprise-capability-grid'>
            {capabilities.map(({ Icon, title, description }) => (
              <article className='enterprise-capability' key={title}>
                <span className='enterprise-capability-icon'>
                  <Icon size={24} strokeWidth={1.6} aria-hidden='true' />
                </span>
                <h3>{title}</h3>
                <p>{description}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section
        className='enterprise-section enterprise-routing-section'
        aria-labelledby='enterprise-routing-title'
      >
        <div className='enterprise-container'>
          <div className='enterprise-section-heading'>
            <span className='enterprise-section-index' aria-hidden='true'>
              03 /
            </span>
            <h2 id='enterprise-routing-title'>{t('多条线路，一个入口')}</h2>
            <p>{t('查看备用线路如何承接请求。')}</p>
          </div>
          <RoutingDemo t={t} brandName={brandName} brandLogo={brandLogo} />
        </div>
      </section>

      <section
        className='enterprise-section enterprise-services'
        aria-labelledby='enterprise-services-title'
      >
        <div className='enterprise-container'>
          <div className='enterprise-section-heading'>
            <span className='enterprise-section-index' aria-hidden='true'>
              04 /
            </span>
            <h2 id='enterprise-services-title'>{t('从你的场景出发')}</h2>
          </div>
          <div className='enterprise-service-grid'>
            {services.map(({ Icon, title, description }, index) => (
              <article className='enterprise-service' key={title}>
                <div className='enterprise-service-visual' aria-hidden='true'>
                  <span>0{index + 1}</span>
                  <Icon size={42} strokeWidth={1.25} />
                </div>
                <h3>{title}</h3>
                <p>{description}</p>
                <a href={contactHref} className='enterprise-text-link'>
                  {t('发送邮件咨询')}
                  <ArrowUpRight size={16} aria-hidden='true' />
                </a>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section
        id='enterprise-contact'
        className='enterprise-contact-section'
        aria-labelledby='enterprise-contact-title'
      >
        <div className='enterprise-container enterprise-contact-grid'>
          <div>
            <span className='enterprise-eyebrow'>
              <Mail size={16} aria-hidden='true' />
              {t('企业接入咨询')}
            </span>
            <h2 id='enterprise-contact-title'>{t('聊聊你的下一步。')}</h2>
            <p>
              {t(
                '告诉我们使用场景、目标模型与预计用量，我们一起梳理接入方案。',
              )}
            </p>
          </div>
          <div className='enterprise-contact-card'>
            <a href={contactHref} className='enterprise-contact-email'>
              {ENTERPRISE_CONTACT}
              <ArrowUpRight size={21} aria-hidden='true' />
            </a>
            <div className='enterprise-contact-tools'>
              <CopyAction
                value={ENTERPRISE_CONTACT}
                label={t('复制邮箱')}
                t={t}
              />
              <Link to='/docs/troubleshooting' className='enterprise-text-link'>
                {t('服务支持')}
                <ArrowUpRight size={15} aria-hidden='true' />
              </Link>
            </div>
          </div>
        </div>
      </section>
    </main>
  );
}
