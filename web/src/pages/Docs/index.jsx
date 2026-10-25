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

import React, { useMemo } from 'react';
import { Button, Card, Select, Space, Typography } from '@douyinfe/semi-ui';
import { IconBookOpenStroked, IconLink } from '@douyinfe/semi-icons';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import MarkdownRenderer from '../../components/common/markdown/MarkdownRenderer';
import usageMarkdown from '../../../../docs/NAN_USAGE.md?raw';
import clientsMarkdown from '../../../../docs/NAN_CLIENTS.md?raw';
import codexMarkdown from '../../../../docs/NAN_CLIENT_CODEX.md?raw';
import claudeCodeMarkdown from '../../../../docs/NAN_CLIENT_CLAUDE_CODE.md?raw';
import claudeCodeOpenAIMarkdown from '../../../../docs/NAN_CLIENT_CLAUDE_CODE_OPENAI.md?raw';
import geminiMarkdown from '../../../../docs/NAN_CLIENT_GEMINI.md?raw';
import opencodeMarkdown from '../../../../docs/NAN_CLIENT_OPENCODE.md?raw';
import openclawMarkdown from '../../../../docs/NAN_CLIENT_OPENCLAW.md?raw';
import ccSwitchMarkdown from '../../../../docs/NAN_CLIENT_CCSWITCH.md?raw';
import imageGenerationMarkdown from '../../../../docs/NAN_IMAGE_GENERATION.md?raw';
import apiExamplesMarkdown from '../../../../docs/NAN_API_EXAMPLES.md?raw';
import troubleshootingMarkdown from '../../../../docs/NAN_TROUBLESHOOTING.md?raw';
import mimoAudioMarkdown from '../../../../docs/NAN_MIMO_AUDIO.md?raw';
import { renderRuntimeDocs } from './runtimeDocs';
import useRuntimeDocs from './useRuntimeDocs';
import { useContentLanguage } from '../../helpers/contentLanguageReact';
import './index.css';

const { Title, Text } = Typography;
const DOCS_EYEBROW = 'NAN Docs';
const CARD_SHADOWS = 'always';

const DOCS = [
  {
    key: 'usage',
    path: '/docs',
    group: '基础',
    title: '平台教程',
    description: '注册、令牌、调用、充值、发票、扣费和安全提醒',
    content: usageMarkdown,
  },
  {
    key: 'clients',
    path: '/docs/clients',
    group: '客户端',
    title: '客户端接入',
    description: 'Codex、Claude Code、Gemini、OpenCode、OpenClaw、CC Switch',
    content: clientsMarkdown,
  },
  {
    key: 'client-codex',
    path: '/docs/clients/codex',
    group: '客户端',
    title: 'Codex',
    description: 'Codex 一键配置与手动配置',
    content: codexMarkdown,
  },
  {
    key: 'client-claude-code',
    path: '/docs/clients/claude-code',
    group: '客户端',
    title: 'Claude Code',
    description: 'Claude Code 使用 Claude 模型',
    content: claudeCodeMarkdown,
  },
  {
    key: 'client-claude-code-openai',
    path: '/docs/clients/claude-code-openai',
    group: '客户端',
    title: 'Claude Code OpenAI',
    description: 'Claude Code 使用 OpenAI / Codex 模型',
    content: claudeCodeOpenAIMarkdown,
  },
  {
    key: 'client-gemini',
    path: '/docs/clients/gemini',
    group: '客户端',
    title: 'Gemini CLI',
    description: 'Gemini CLI 配置与验证',
    content: geminiMarkdown,
  },
  {
    key: 'client-opencode',
    path: '/docs/clients/opencode',
    group: '客户端',
    title: 'OpenCode',
    description: 'OpenCode 环境变量与配置文件',
    content: opencodeMarkdown,
  },
  {
    key: 'client-openclaw',
    path: '/docs/clients/openclaw',
    group: '客户端',
    title: 'OpenClaw',
    description: 'OpenClaw 本地网关与多模型配置',
    content: openclawMarkdown,
  },
  {
    key: 'client-ccswitch',
    path: '/docs/clients/ccswitch',
    group: '客户端',
    title: 'CC Switch',
    description: '一键导入配置与聊天入口边界',
    content: ccSwitchMarkdown,
  },
  {
    key: 'image-generation',
    path: '/docs/image-generation',
    group: '能力',
    title: '元衡生图',
    description: '平台工作台、本地技能包与 API 请求三种使用方式',
    content: imageGenerationMarkdown,
  },
  {
    key: 'mimo-audio',
    path: '/docs/mimo-audio',
    group: '能力',
    title: 'MiMo 语音',
    description: 'TTS、音色设计、音色克隆和语音识别',
    content: mimoAudioMarkdown,
  },
  {
    key: 'api-examples',
    path: '/docs/api-examples',
    group: '开发',
    title: '模型请求样例',
    description: '各厂商模型、协议地址与可直接复制的调用样例',
    content: apiExamplesMarkdown,
  },
  {
    key: 'troubleshooting',
    path: '/docs/troubleshooting',
    group: '排查',
    title: '常见报错排查',
    description: '按错误文本排查注册、认证、模型、客户端、网络、支付问题',
    content: troubleshootingMarkdown,
  },
];

const DOC_LINK_REPLACEMENTS = [
  [/\.\/NAN_USAGE\.md/g, '/docs'],
  [/\.\/NAN_CLIENTS\.md/g, '/docs/clients'],
  [/\.\/NAN_CLIENT_CODEX\.md/g, '/docs/clients/codex'],
  [/\.\/NAN_CLIENT_CLAUDE_CODE\.md/g, '/docs/clients/claude-code'],
  [
    /\.\/NAN_CLIENT_CLAUDE_CODE_OPENAI\.md/g,
    '/docs/clients/claude-code-openai',
  ],
  [/\.\/NAN_CLIENT_GEMINI\.md/g, '/docs/clients/gemini'],
  [/\.\/NAN_CLIENT_OPENCODE\.md/g, '/docs/clients/opencode'],
  [/\.\/NAN_CLIENT_OPENCLAW\.md/g, '/docs/clients/openclaw'],
  [/\.\/NAN_CLIENT_CCSWITCH\.md/g, '/docs/clients/ccswitch'],
  [/\.\/NAN_IMAGE_GENERATION\.md/g, '/docs/image-generation'],
  [/\.\/NAN_API_EXAMPLES\.md/g, '/docs/api-examples'],
  [/\.\/NAN_TROUBLESHOOTING\.md/g, '/docs/troubleshooting'],
  [/\.\/NAN_MIMO_AUDIO\.md/g, '/docs/mimo-audio'],
  // 图片放 /docs-images/（不能叫 /docs/images/：dist 里出现 docs 目录会让
  // 静态文件服务把 /docs 当目录发 301，与 SPA 路由打架导致重定向死循环）
  [/\.\/images\//g, '/docs-images/'],
];

const NAV_GROUPS = DOCS.reduce((groups, doc) => {
  if (!groups.includes(doc.group)) {
    groups.push(doc.group);
  }
  return groups;
}, []);

function normalizeMarkdownLinks(content) {
  return DOC_LINK_REPLACEMENTS.reduce(
    (current, [pattern, replacement]) => current.replace(pattern, replacement),
    content,
  );
}

function normalizeHeadingText(value) {
  return value
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[`*_~]/g, '')
    .replace(/\s+#+\s*$/, '')
    .trim();
}

function createHeadingId(value) {
  return normalizeHeadingText(value)
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

function prepareMarkdown(content) {
  return normalizeMarkdownLinks(content).replace(/^\uFEFF?#\s+[^\n]+\n+/, '');
}

function extractHeadings(content) {
  let inCodeBlock = false;
  return content.split('\n').reduce((headings, line) => {
    if (/^\s*```/.test(line)) {
      inCodeBlock = !inCodeBlock;
      return headings;
    }
    if (inCodeBlock) {
      return headings;
    }

    const match = line.match(/^(##|###)\s+(.+)$/);
    if (!match) {
      return headings;
    }

    const title = normalizeHeadingText(match[2]);
    const id = createHeadingId(title);
    if (title && id) {
      headings.push({
        id,
        title,
        level: match[1].length,
      });
    }
    return headings;
  }, []);
}

function getActiveDoc(pathname) {
  const normalizedPath = pathname.replace(/\/+$/, '') || '/docs';
  const matched = [...DOCS]
    .sort((a, b) => b.path.length - a.path.length)
    .find(
      (doc) =>
        normalizedPath === doc.path ||
        normalizedPath.startsWith(`${doc.path}/`),
    );
  return matched || DOCS[0];
}

export default function DocsPage() {
  const { t, i18n } = useTranslation();
  const bodyLanguage = useContentLanguage(
    i18n.resolvedLanguage || i18n.language,
  );
  const location = useLocation();
  const navigate = useNavigate();
  const activeDoc = getActiveDoc(location.pathname);
  const runtime = useRuntimeDocs();
  const content = useMemo(
    () =>
      prepareMarkdown(
        renderRuntimeDocs(bodyLanguage.markdown(activeDoc.content), {
          ...runtime,
          translate: bodyLanguage.markdown,
          origin: window.location.origin,
        }),
      ),
    [
      activeDoc.content,
      bodyLanguage.markdown,
      runtime.serverAddress,
      runtime.pricing,
      runtime.loading,
    ],
  );
  const headings = useMemo(() => extractHeadings(content), [content]);

  return (
    <div className='nan-docs-page' id='docs-top'>
      <section className='nan-docs-hero'>
        <div>
          <Text className='nan-docs-eyebrow'>{DOCS_EYEBROW}</Text>
          <Title heading={2} className='nan-docs-title'>
            {t('平台使用文档')}
          </Title>
          <Text type='tertiary'>
            {t('新手先看平台教程和配置速查；遇到错误直接查常见报错排查。')}
          </Text>
        </div>
        <Button
          icon={<IconLink />}
          theme='borderless'
          onClick={() =>
            window.open('https://docs.newapi.pro/zh/docs', '_blank')
          }
        >
          {t('上游文档')}
        </Button>
      </section>

      <div className='nan-docs-layout'>
        <div className='nan-docs-mobile-nav'>
          <Text className='nan-docs-mobile-nav-label'>{t('选择文档')}</Text>
          <Select
            value={activeDoc.path}
            optionList={DOCS.map((doc) => ({
              value: doc.path,
              label: `${t(doc.group)} · ${t(doc.title)}`,
            }))}
            onChange={(value) => navigate(value)}
          />
        </div>

        <aside className='nan-docs-sidebar'>
          {NAV_GROUPS.map((group) => (
            <div className='nan-docs-nav-group' key={group}>
              <Text className='nan-docs-nav-group-title'>{t(group)}</Text>
              <Space vertical align='start' style={{ width: '100%' }}>
                {DOCS.filter((doc) => doc.group === group).map((doc) => (
                  <Button
                    key={doc.key}
                    block
                    icon={<IconBookOpenStroked />}
                    theme={doc.key === activeDoc.key ? 'solid' : 'borderless'}
                    type={doc.key === activeDoc.key ? 'primary' : 'tertiary'}
                    className='nan-docs-nav-button'
                    onClick={() => navigate(doc.path)}
                  >
                    {t(doc.title)}
                  </Button>
                ))}
              </Space>
            </div>
          ))}
        </aside>

        <main className='nan-docs-content'>
          <Card className='nan-docs-card' shadows={CARD_SHADOWS}>
            <div className='nan-docs-card-header'>
              <div>
                <Title heading={3}>{t(activeDoc.title)}</Title>
                <Text type='tertiary'>{t(activeDoc.description)}</Text>
              </div>
            </div>
            <div className='nan-docs-sync' role='status'>
              <Text type={runtime.failed ? 'warning' : 'tertiary'}>
                {runtime.loading
                  ? t('正在更新文档配置…')
                  : runtime.failed
                    ? t(
                        '部分配置加载失败，请重试；地址暂用当前站点，模型以模型广场为准。',
                      )
                    : `${t('地址、分组和请求样例随站点配置自动更新')} · ${new Date(runtime.updatedAt).toLocaleTimeString(bodyLanguage.language)}`}
              </Text>
              <Button
                size='small'
                loading={runtime.loading}
                onClick={runtime.refresh}
              >
                {t('刷新')}
              </Button>
            </div>
            {(bodyLanguage.pending || bodyLanguage.failed) && (
              <div className='nan-docs-sync' role='status'>
                <Text type={bodyLanguage.failed ? 'warning' : 'tertiary'}>
                  {t(
                    bodyLanguage.failed
                      ? '正文翻译加载失败，请重试。'
                      : '正在加载正文翻译…',
                  )}
                </Text>
                {bodyLanguage.failed && (
                  <Button size='small' onClick={bodyLanguage.retry}>
                    {t('重试')}
                  </Button>
                )}
              </div>
            )}
            <div lang={bodyLanguage.ready ? bodyLanguage.language : 'zh-CN'}>
              <MarkdownRenderer content={content} fontSize={15} />
            </div>
          </Card>
        </main>

        <aside className='nan-docs-toc'>
          <Text className='nan-docs-toc-title'>{t('本页目录')}</Text>
          <nav aria-label={t('本页目录')}>
            {headings.map((heading, index) => (
              <a
                key={`${heading.id}-${index}`}
                href={`#${heading.id}`}
                className={
                  heading.level === 3
                    ? 'nan-docs-toc-link nan-docs-toc-link-sub'
                    : 'nan-docs-toc-link'
                }
              >
                {heading.title}
              </a>
            ))}
          </nav>
          <a className='nan-docs-toc-back' href='#docs-top'>
            {t('返回顶部')}
          </a>
        </aside>
      </div>
    </div>
  );
}
