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

import { useEffect, useState } from 'react';
import { normalizeLocale } from './localeNormalize';

const LANGUAGE_STORAGE_KEY = 'i18nextLng';
const LANGUAGE_CHANGE_EVENT = 'public-language-change';

const en = {
  产品: 'Products',
  开发者: 'Developers',
  关于我们: 'About us',
  条款与政策: 'Terms and policies',
  页脚导航: 'Footer navigation',
  快速开始: 'Quick start',
  常见问题: 'FAQ',
  用户协议: 'User agreement',
  隐私政策: 'Privacy policy',
  用量查询: 'Usage lookup',
  个人: 'Individual',
  团队: 'Team',
  智能体: 'Agent',
  优先线路连接: 'Primary route connected',
  模拟连接中断: 'Simulated interruption',
  已切换备用线路: 'Backup route connected',
  '示例使用 Chat Completions，其他接口请参阅文档。':
    'This example uses Chat Completions. See the docs for other APIs.',
  主导航: 'Main navigation',
  导航菜单: 'Navigation menu',
  // 默认首页：轻量公开页的中英文文案。
  '让好模型，为你所用': 'Great models, ready for your ideas',
  '一个 API，': 'One API. ',
  '连接万千可能。': 'Endless possibilities.',
  '用熟悉的工具，调用你需要的模型。':
    'Your favorite tools. The models you need. ',
  '从第一行代码，到下一个好产品。':
    'From your first line of code to your next great product.',
  开始构建: 'Start building',
  浏览模型: 'Explore models',
  多模型接入: 'Multiple models',
  '兼容主流 SDK': 'Familiar SDKs',
  按量计费: 'Pay as you go',
  '一个密钥，多种可能': 'One key, more possibilities',
  '从文本到图像，让创作自由切换。': 'Move freely between text and images.',
  '熟悉的 SDK，更简单的接入': 'Your SDK, a simpler setup',
  '配置地址与密钥，即可开始。': 'Set your endpoint and key. Start building.',
  '多条线路，一个入口': 'Multiple routes. One endpoint.',
  '统一调度，让应用专注业务。':
    'Unified routing, so you can focus on your app.',
  你的应用: 'Your app',
  '不止于 API': 'BEYOND THE API',
  '你的 AI 工具，': 'Your AI tools. ',
  '在这里汇合。': 'Together at last.',
  '在电脑上管理 AI 工具，在手机上随时对话。把接入的时间，留给真正想做的事。':
    'Manage AI tools on your computer and chat on your phone. Spend less time on setup and more on what you want to create.',
  '统一管理客户端、模型与分组':
    'Manage clients, models and groups in one place',
  沿用熟悉的开发方式: 'Keep the workflow you know',
  接入示意: 'Illustration',
  '我的 AI 工具': 'My AI tools',
  终端编程: 'Terminal coding',
  代码协作: 'Coding companion',
  命令行助手: 'Command-line assistant',
  '一个工作区，连接你的工作流': 'One workspace for your workflow',
  '从灵感，到实现': 'From idea to reality',
  '好模型，不必只选一个。': 'The right model for every idea.',
  '为不同任务，找到合适的模型。具体可用模型与价格，以模型广场为准。':
    'Choose a model for each task. Check the model marketplace for availability and pricing.',
  查看模型与价格: 'Explore models and pricing',
  通用对话与代码开发: 'Conversation and coding',
  长文理解与编程协作: 'Long context and code collaboration',
  多模态理解与创作: 'Multimodal understanding and creation',
  推理与问题求解: 'Reasoning and problem solving',
  语言理解与应用开发: 'Language and app development',
  对话与内容探索: 'Conversation and exploration',
  '少一点配置，': 'Less configuration. ',
  '多一点创造。': 'More creation.',
  '保留你的开发习惯，从一次简单的 API 调用开始。':
    'Keep your workflow. Start with a simple API call.',
  '创建 API Key': 'Create an API key',
  '在控制台创建令牌，按需设置额度。':
    'Create a key in the console and set a spending limit.',
  配置接入地址: 'Set your endpoint',
  '将示例中的 API_KEY 和 MODEL_ID 替换为你的配置。':
    'Set API_KEY and replace MODEL_ID with your chosen model.',
  发起第一次调用: 'Make your first call',
  '在使用日志中查看请求和费用。':
    'Review your requests and costs in usage logs.',
  代码语言: 'Code language',
  复制代码: 'Copy code',
  接口地址: 'Endpoint',
  已复制: 'Copied',
  '复制失败，请手动选择': 'Copy failed. Please select manually.',
  '开始之前，': 'Before you begin. ',
  '你可能想了解。': 'A few useful answers.',
  '已有的 SDK 和工具可以继续用吗？': 'Can I keep using my SDK and tools?',
  '兼容 OpenAI、Claude、Gemini 等常见接口。按接入文档配置地址、密钥和模型即可；不同模型支持的参数可能有所不同。':
    'Common OpenAI, Claude and Gemini API formats are supported. Follow the docs to set the endpoint, key and model. Supported parameters may vary by model.',
  '在哪里查看模型和价格？': 'Where can I find models and pricing?',
  '模型广场提供可用模型、接口类型和价格信息。登录后可查看账号可用分组，具体费用以模型、分组和实际用量为准。':
    'The model marketplace lists models, API types and prices. Sign in to see groups available to your account. Costs depend on the model, group and actual usage.',
  '如何管理 API Key 和用量？': 'How do I manage API keys and usage?',
  '在控制台创建和管理令牌，按项目设置额度、模型限制与有效期，并通过使用日志查看调用和费用。':
    'Manage keys in the console, set spending limits, model restrictions and expiration by project, and review requests and costs in usage logs.',
  '可以在手机和电脑上使用吗？': 'Can I use it on my phone and computer?',
  '下载页提供桌面客户端和安卓公开测试版，具体系统要求、可用版本与安装说明以下载页为准。':
    'The download page provides desktop apps and an Android public alpha, along with current versions, system requirements and installation instructions.',
  '下一个好想法，从这里开始。': 'Your next great idea starts here.',
  '模型已经就位，只等你的创意。': 'The models are ready. Bring your ideas.',

  首页: 'Home',
  控制台: 'Console',
  模型广场: 'Model Marketplace',
  文档: 'Docs',
  关于: 'About',
  登录: 'Sign in',
  注册: 'Sign up',
  切换主题: 'Switch Theme',
  切换语言: 'Change Language',
  'common.changeLanguage': 'Change Language',
  版权所有: 'All rights reserved',
  设计与开发由: 'Designed & Developed by',
  统一的: 'The Unified',
  大模型接口网关: 'LLM API Gateway',
  '更好的价格，更好的稳定性，只需要将模型基址替换为：':
    'Better prices and better stability. Just replace the model base URL with:',
  '一衡调山海，万模皆可达': 'Balance the vast — every model within reach.',
  '元起万模，衡定全局':
    'One origin for every model, one balance for the whole system.',
  '元衡 API：一口接入多模型，':
    'Yuanheng API: one integration for many models,',
  智能调度更稳定: 'smarter routing, steadier delivery',
  '更强模型，更低价格，更易落地。':
    'Stronger models, lower prices, easier deployment.',
  '统一接入 OpenAI、Claude、Gemini、DeepSeek、Qwen、Midjourney 等主流模型，通过智能路由与负载均衡，为企业提供高可用、低延迟、透明计费的全托管 AI API 网关服务，助力业务快速集成与规模化落地。':
    'Unify OpenAI, Claude, Gemini, DeepSeek, Qwen, Midjourney and more behind a managed AI API gateway with smart routing, high availability, low latency and transparent billing.',
  复制完整地址: 'Copy full endpoint',
  '开启 AI 新体验': 'Start a New AI Experience',
  查看模型广场: 'View Model Marketplace',
  已接入模型: 'Models integrated',
  稳定性保障: 'Stability guarantee',
  服务支持: 'Support',
  运行正常: 'Healthy',
  '近 24 小时': 'Last 24 hours',
  稳定可用: 'Availability',
  智能路由: 'Smart routing',
  透明计费: 'Transparent billing',
  策略状态: 'Strategy status',
  我们的优势: 'Our Advantages',
  '把模型多、接入快、可部署、可观测放到同一套控制平面，首页表达更接近企业级 API 网关。':
    'Bring model coverage, fast integration, deployment and observability into one control plane for an enterprise-grade API gateway story.',
  企业级高速通道: 'Enterprise-grade Fast Channels',
  '多供应商、多账号、多线路汇聚，优先保证请求稳定落地，适合高频调用场景。':
    'Aggregate multiple providers, accounts and routes to keep high-frequency requests stable.',
  协议完全兼容: 'Protocol Compatible',
  '兼容 OpenAI、Claude、Gemini 常见接口形态，现有 SDK 和客户端低成本迁移。':
    'Compatible with common OpenAI, Claude and Gemini API shapes so existing SDKs and clients migrate easily.',
  智能负载均衡: 'Smart Load Balancing',
  '按模型、分组、倍率、渠道健康度与失败状态动态调度，减少单点波动。':
    'Dynamically route by model, group, ratio, channel health and failures to reduce single-point volatility.',
  'Token、缓存、图片、按次任务统一折算，余额、日志与账单链路可追踪。':
    'Unify token, cache, image and per-call billing with traceable balance, logs and invoices.',
  多节点容灾: 'Multi-node Failover',
  '线路异常时自动切换与重试，结合渠道健康度降低超时、断流和上游波动影响。':
    'Automatically switch and retry on route failures, reducing timeout, stream and upstream volatility.',
  '7×24 服务保障': '24/7 Support',
  '面向团队、代理和商业用户提供持续支持，关键问题可快速定位到模型、渠道或用户。':
    'Continuous support for teams, agents and business users, with issues located by model, channel or user.',
  多模态覆盖: 'Multimodal Coverage',
  '文本、视觉、图片生成、Claude Code、Codex、Gemini CLI 等场景一站接入。':
    'One-stop access for text, vision, image generation, Claude Code, Codex, Gemini CLI and more.',
  代理与团队运营: 'Agent & Team Operations',
  '支持分组、密钥、额度、倍率与用量视图，方便代理加盟和团队级成本管理。':
    'Groups, keys, quotas, ratios and usage views support agent programs and team cost management.',
  '一个入口，覆盖多种客户端': 'One Entry for Many Clients',
  '统一 Base URL 与 API Key，覆盖 OpenAI-compatible、Claude、Gemini、Codex、Claude Code 等接入方式。':
    'One Base URL and API key for OpenAI-compatible, Claude, Gemini, Codex, Claude Code and more.',
  '500+ 模型能力池': '500+ Model Capability Pool',
  '从通用对话、代码编程、长文本、图像生成到多模态理解，都可以通过统一入口接入。':
    'Access chat, coding, long context, image generation and multimodal understanding through one endpoint.',
  稳定开始调用: 'Start Stable Calls',
  '登录控制台创建 API Key，按团队、用户或项目设置额度、可用模型和过期时间。':
    'Create API keys in the console and configure quotas, models and expiration by team, user or project.',
  '把原客户端的 Base URL 改成元衡 API 地址，保留原有 SDK、参数和调用结构。':
    'Change the client Base URL to Yuanheng API while keeping your SDK, parameters and request shape.',
  '通过日志、账单、渠道状态和模型广场持续观测请求表现，按需切换策略。':
    'Observe requests through logs, billing, channel status and the model marketplace, then switch strategy as needed.',
  全天候支持: 'Always-on Support',
  我们时刻恭候您: 'We are ready whenever you need us',
  '从模型选型、客户端接入、代理运营到异常排查，元衡 API 都提供更清晰的控制台和更稳定的调用链路。':
    'From model selection and client integration to agent operations and incident diagnosis, Yuanheng API provides a clearer console and steadier call path.',
  立即进入控制台: 'Enter Console',
  '阅读 API 文档': 'Read API Docs',
  '聚合万模，平衡调度，一口稳定接入，只需将模型基址替换为：':
    'Aggregate every model, balance every request, one stable integration. Just replace the model base URL with:',
  '一口接入多模型，智能调度更稳定，只需将模型基址替换为：':
    'One integration for many models, with smarter and steadier routing. Just replace the model base URL with:',
  已复制到剪切板: 'Copied',
  获取密钥: 'Get Key',
  支持众多的大模型供应商: 'Supporting various LLM providers',
  系统公告: 'System Notice',
  今日关闭: 'Close Today',
  关闭公告: 'Close Notice',
  // 首页内容板块
  '为什么选择元衡 API': 'Why Choose Yuanheng API',
  '聚合 40+ 大模型': 'Aggregate 40+ Models',
  'OpenAI、Claude、Gemini、DeepSeek、Qwen… 一个统一 API 全部调用，无需多处对接。':
    'OpenAI, Claude, Gemini, DeepSeek, Qwen… call them all through one unified API, no multiple integrations needed.',
  更优的价格: 'Better Pricing',
  '批量议价、按量计费、缓存命中优惠，成本更低、账单更透明。':
    'Bulk pricing, pay-as-you-go, cache-hit discounts — lower costs and clearer bills.',
  稳定高可用: 'Highly Available',
  '多节点容灾、智能路由、自动重试，保障调用稳定不中断。':
    'Multi-node failover, smart routing and auto-retry keep your calls stable and uninterrupted.',
  安全可控: 'Secure & Controllable',
  '密钥隔离、用量管控、调用日志与账单全程可查，合规放心。':
    'Key isolation, usage control, and fully auditable logs and bills — compliant and worry-free.',
  三步快速接入: 'Get Started in Three Steps',
  注册并获取密钥: 'Register & Get a Key',
  '登录控制台，创建专属 API Key。':
    'Sign in to the console and create your own API key.',
  替换模型基址: 'Replace the Base URL',
  '把原有 Base URL 换成本平台地址。':
    'Swap your existing Base URL for this platform’s address.',
  立即开始调用: 'Start Calling',
  '沿用原有 SDK 与参数，直接调用 40+ 模型。':
    'Keep your existing SDK and parameters, and call 40+ models right away.',
  '现在就接入，开启统一的大模型调用体验':
    'Integrate now and unlock a unified LLM experience.',
  立即开始: 'Get Started',
  下载客户端: 'Download App',
  下载元衡客户端: 'Download YuanHeng',
  '在手机上直接使用 AI，在电脑上统一管理你的 AI 工具。':
    'Use AI on your phone and manage your AI tools on your computer.',
  公开测试版: 'Public alpha',
  '元衡 Android': 'YuanHeng for Android',
  '在手机上聊天、生图、查看额度和管理令牌，支持兑换码与官网充值。':
    'Chat, create images, view your balance and manage API tokens. Redeem codes and top up on our website.',
  '下载安卓 APK': 'Download Android APK',
  'GitHub 备用下载': 'Download from GitHub',
  '下载后打开 APK，按系统提示允许当前浏览器安装应用。':
    'Open the downloaded APK and allow your browser to install apps when Android prompts you.',
  '安卓测试版暂不支持应用内更新，新版本需重新下载安装。':
    'This Android alpha does not support in-app updates. Download and install new versions when available.',
  查看安装包校验信息: 'View package checksum',
  下载校验文件: 'Download checksum file',
  桌面版当前发布版本: 'Current desktop release',
  桌面版支持应用内自动更新: 'Desktop apps support in-app updates',
  返回首页: 'Back to Home',
  '把你的 AI 工具，放进一个控制中心':
    'Bring Your AI Tools Into One Control Center',
  '下载安装 YuanHeng Desktop，统一管理终端、客户端、模型和分组，后续版本可在应用内直接更新。':
    'Download YuanHeng Desktop to manage terminals, clients, models and groups in one place, with in-app updates for future releases.',
  当前发布版本: 'Current release',
  最新稳定版: 'Latest stable',
  选择安装包: 'Choose Installer',
  选择与你的电脑匹配的版本: 'Choose the Version for Your Computer',
  '检测到 macOS，请根据芯片类型选择。':
    'macOS detected. Choose the installer that matches your chip.',
  '检测到 Windows，已优先显示对应版本。':
    'Windows detected. The matching installer is shown first.',
  正在获取最新版本: 'Loading latest release',
  'macOS · Apple 芯片': 'macOS · Apple Silicon',
  '适用于 Apple 芯片的 Mac': 'For Macs with Apple silicon',
  'macOS · Intel 芯片': 'macOS · Intel',
  '适用于 Intel 芯片的 Mac': 'For Macs with an Intel processor',
  'Windows · 64 位': 'Windows · 64-bit',
  '适用于 64 位 Windows 电脑': 'For 64-bit Windows PCs',
  '下载 DMG': 'Download DMG',
  下载安装程序: 'Download Installer',
  备用线路: 'Alternative downloads',
  当前系统: 'Your System',
  推荐版本: 'Recommended',
  客户端暂未开放下载: 'Desktop App Is Not Available Yet',
  '当前还没有可公开下载的稳定版本，请稍后再来。':
    'There is no public stable release yet. Please check back later.',
  暂时无法获取下载信息: 'Download Information Is Temporarily Unavailable',
  '网络或更新服务暂时不可用，请稍后重试。':
    'The network or update service is temporarily unavailable. Please try again later.',
  重新加载: 'Reload',
  安装包由元衡平台直接托管: 'Installers are hosted directly by YuanHeng',
  安装后支持应用内自动更新: 'In-app updates are available after installation',
  正在加载: 'Loading',
};

// 公开首页仅内置 zh / en 两套词条：先走与控制台一致的归一化（localeNormalize），
// 再把非中文语言收敛到 en，保证与 i18next 检测结果方向一致、跨页不跳变。
export const normalizePublicLanguage = (language) => {
  const normalized = normalizeLocale(language);
  return normalized.startsWith('zh') ? normalized : 'en';
};

export const getPublicLanguage = () => {
  if (typeof window === 'undefined') return 'zh-CN';
  const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY);
  return normalizePublicLanguage(stored || navigator.language || 'zh-CN');
};

export const setPublicLanguage = (language) => {
  const normalizedLanguage = normalizePublicLanguage(language);
  localStorage.setItem(LANGUAGE_STORAGE_KEY, normalizedLanguage);
  window.dispatchEvent(
    new CustomEvent(LANGUAGE_CHANGE_EVENT, { detail: normalizedLanguage }),
  );
  return normalizedLanguage;
};

export const usePublicTranslation = () => {
  const [language, setLanguage] = useState(getPublicLanguage);

  useEffect(() => {
    const handleLanguageChange = (event) => {
      setLanguage(normalizePublicLanguage(event.detail || getPublicLanguage()));
    };

    window.addEventListener(LANGUAGE_CHANGE_EVENT, handleLanguageChange);
    window.addEventListener('storage', handleLanguageChange);
    return () => {
      window.removeEventListener(LANGUAGE_CHANGE_EVENT, handleLanguageChange);
      window.removeEventListener('storage', handleLanguageChange);
    };
  }, []);

  const isChinese = language.startsWith('zh');
  const t = (key) => (isChinese ? key : en[key] || key);

  return { t, language, isChinese, setLanguage: setPublicLanguage };
};
