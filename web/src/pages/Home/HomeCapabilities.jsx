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

import React, { useEffect, useRef, useState } from 'react';
import {
  Bot,
  Code2,
  Image,
  KeyRound,
  MessageSquare,
  UserRound,
  UsersRound,
  X,
} from 'lucide-react';
import {
  Claude,
  DeepSeek,
  Gemini,
  Grok,
  Minimax,
  Moonshot,
  OpenAI,
  Qwen,
} from '../../helpers/providerIcons';
import {
  createCapabilityMotion,
  INITIAL_CAPABILITY_MOTION,
} from './capabilityMotion';

const logos = [
  OpenAI,
  Claude.Color,
  Gemini.Color,
  DeepSeek.Color,
  Qwen.Color,
  Grok,
  Moonshot,
  Minimax.Color,
];
const scenes = [
  {
    name: 'Claude',
    Icon: Claude.Color,
    suppliers: ['Anthropic', 'AWS Bedrock', 'Vertex AI'],
  },
  {
    name: 'OpenAI',
    Icon: OpenAI,
    suppliers: ['OpenAI', 'Azure', 'OpenRouter'],
  },
  {
    name: 'Gemini',
    Icon: Gemini.Color,
    suppliers: ['Google AI', 'Vertex AI', 'OpenRouter'],
  },
];
const audiences = [
  { label: '个人', Icon: UserRound },
  { label: '团队', Icon: UsersRound },
  { label: '智能体', Icon: Bot },
];
const routePaths = [
  'M0 16C22 16 24 52 42 52',
  'M0 52H42',
  'M0 88C22 88 24 52 42 52',
];

export default function HomeCapabilities({ baseUrl, t }) {
  const element = useRef(null);
  const [motion, setMotion] = useState(INITIAL_CAPABILITY_MOTION);
  useEffect(
    () =>
      createCapabilityMotion({
        element: element.current,
        onChange: setMotion,
        sceneCount: scenes.length,
      }),
    [],
  );
  const scene = scenes[motion.scene];
  const ModelIcon = scene.Icon;
  const primary = motion.scene % scene.suppliers.length;
  const fallback = (primary + 1) % scene.suppliers.length;
  const activeRoute =
    motion.phase === 'primary'
      ? primary
      : motion.phase === 'fallback'
        ? fallback
        : -1;
  const routeClass = (index) =>
    index === activeRoute
      ? 'is-active'
      : index === primary && motion.phase !== 'primary'
        ? 'is-unavailable'
        : '';
  const routeStatus =
    motion.phase === 'unavailable'
      ? '模拟连接中断'
      : motion.phase === 'fallback'
        ? '已切换备用线路'
        : '优先线路连接';

  return (
    <div
      ref={element}
      className='yh-container yh-capabilities'
      data-motion={motion.active ? 'running' : 'paused'}
    >
      <article className='yh-capability'>
        <h2>{t('一个密钥，多种可能')}</h2>
        <p>{t('从文本到图像，让创作自由切换。')}</p>
        <div className='yh-key-visual' aria-hidden='true'>
          <div className='yh-provider-pill'>
            <div className='yh-provider-window'>
              <div className='yh-provider-track'>
                {[0, 1].map((copy) => (
                  <div className='yh-provider-group' key={copy}>
                    {logos.map((Icon, index) => (
                      <span className='yh-provider-logo' key={index}>
                        <Icon size={26} />
                      </span>
                    ))}
                  </div>
                ))}
              </div>
            </div>
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
          <div className='yh-code-removed'>
            <span>−</span>
            <code>
              <del> base_url="https://api.openai.com/v1",</del>
            </code>
          </div>
          <div className='yh-code-highlight'>
            <span>+</span>
            <code title={`base_url="${baseUrl}"`}>
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
        <div className='yh-route-demo' aria-hidden='true'>
          <div className='yh-route-visual'>
            <div className='yh-route-labels'>
              {scene.suppliers.map((supplier, index) => (
                <span key={supplier} className={routeClass(index)}>
                  <i>{supplier}</i>
                  {routeClass(index) === 'is-unavailable' && <X size={10} />}
                </span>
              ))}
            </div>
            <svg className='yh-route-inputs' viewBox='0 0 42 104'>
              {routePaths.map((d, index) => (
                <path key={d} d={d} className={routeClass(index)} />
              ))}
            </svg>
            <div className='yh-route-hub' key={scene.name}>
              <span className='yh-router-label'>AUTO ROUTER</span>
              <ModelIcon size={30} />
              <span className='yh-route-model'>{scene.name}</span>
            </div>
            <svg className='yh-route-outputs' viewBox='0 0 42 104'>
              <path d='M0 52C18 52 20 16 42 16M0 52H42M0 52C18 52 20 88 42 88' />
            </svg>
            <div className='yh-route-audiences'>
              {audiences.map(({ label, Icon }) => (
                <span key={label}>
                  <Icon size={13} />
                  {t(label)}
                </span>
              ))}
            </div>
          </div>
          <div className='yh-route-caption'>
            <span>{t('接入示意')}</span>
            <i>·</i>
            <span>{t(routeStatus)}</span>
          </div>
        </div>
      </article>
    </div>
  );
}
