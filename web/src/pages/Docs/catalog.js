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

import { HANDBOOK_DOCS } from './handbook.generated';

const LEGACY_DOCS = [
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

// Keep the original deep links while grouping the expanded handbook by workflow.
const GROUP_ORDER = [
  '开始使用',
  '基础',
  '控制台使用',
  'API 基础',
  '协议参考',
  '模型能力',
  '网关能力',
  '集成指南',
  '客户端',
  '能力',
  '开发',
  '企业与安全',
  '排查与运维',
  '排查',
];
export const DOCS = [...LEGACY_DOCS, ...HANDBOOK_DOCS].sort(
  (a, b) => GROUP_ORDER.indexOf(a.group) - GROUP_ORDER.indexOf(b.group),
);
