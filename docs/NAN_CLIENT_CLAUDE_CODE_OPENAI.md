# Claude Code 使用 OpenAI / Codex 模型

> 来自飞书《新平台ClaudeCode使用OpenAi的模型》，适合在 Claude Code 里调用 OpenAI/Codex 模型。

## 1. 平台准备

1. 登录 `https://meta-api.vip/`。
2. 进入 `令牌管理` 创建令牌。
3. 在模型广场找到目标模型，选择该模型当前支持且账号可选的分组。
4. 可用模型以 `https://meta-api.vip/pricing` 为准。
5. 复制完整 API Key。

## 2. 安装 Claude Code

```bash
node --version
npm --version
npm install -g @anthropic-ai/claude-code
claude --version
```

没有 Node.js 时，去 `https://nodejs.org/` 下载 LTS。

## 3. 配置环境变量

Claude Code 的 Base URL 不带 `/v1`：

```text
ANTHROPIC_BASE_URL=https://meta-api.vip
ANTHROPIC_AUTH_TOKEN=你的API密钥
```

Windows 临时设置：

```powershell
$env:ANTHROPIC_BASE_URL = "https://meta-api.vip"
$env:ANTHROPIC_AUTH_TOKEN = "你的API密钥"
```

Windows 永久设置：

```powershell
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL", "https://meta-api.vip", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_AUTH_TOKEN", "你的API密钥", [System.EnvironmentVariableTarget]::User)
```

macOS / Linux：

```bash
export ANTHROPIC_BASE_URL="https://meta-api.vip"
export ANTHROPIC_AUTH_TOKEN="你的API密钥"
```

zsh 永久设置：

```bash
echo 'export ANTHROPIC_BASE_URL="https://meta-api.vip"' >> ~/.zshrc
echo 'export ANTHROPIC_AUTH_TOKEN="你的API密钥"' >> ~/.zshrc
source ~/.zshrc
```

## 4. 指定 OpenAI / Codex 模型

编辑 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "MODEL_OPENAI_MESSAGES",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "MODEL_OPENAI_MESSAGES",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "MODEL_OPENAI_MESSAGES"
  }
}
```

模型由在线文档按当前声明支持 Anthropic Messages 的 OpenAI 模型选择；若仍显示 MODEL\_\*，请先确认模型与协议。

## 5. 使用

```bash
claude
```

## 6. 常见问题

| 问题           | 处理                              |
| -------------- | --------------------------------- |
| 模型不可用     | 检查模型广场和令牌分组            |
| 分组不匹配     | 在模型广场核对目标模型的分组      |
| IDE 读取旧配置 | 完全退出 VSCode / IDEA 后重开     |
| 连接失败       | `ANTHROPIC_BASE_URL` 不要带 `/v1` |
