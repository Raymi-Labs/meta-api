# Claude Code 使用 Claude 模型

> 来自飞书《新平台配置Claude Code 教程》，适合 Claude Code 调 Claude 模型。

## 1. 平台准备

1. 登录 `https://meta-api.vip/`。
2. 进入 `令牌管理` 创建令牌。
3. 在模型广场找到目标模型，选择该模型当前支持且账号可选的分组。
4. 可用模型以 `https://meta-api.vip/pricing` 为准。
5. 复制完整 API Key。

## 2. 安装环境

### 2.1 Node.js

```bash
node --version
npm --version
```

没有 Node.js 时，去 `https://nodejs.org/` 下载 LTS。

### 2.2 Claude Code

```bash
npm install -g @anthropic-ai/claude-code
```

Linux / macOS 如遇权限问题，命令前加 `sudo`。

验证：

```bash
claude --version
```

## 3. 配置环境变量

需要两个变量：

```text
ANTHROPIC_BASE_URL=https://meta-api.vip
ANTHROPIC_AUTH_TOKEN=你的API密钥
```

### 3.1 Windows PowerShell

临时设置：

```powershell
$env:ANTHROPIC_BASE_URL = "https://meta-api.vip"
$env:ANTHROPIC_AUTH_TOKEN = "你的API密钥"
```

永久设置：

```powershell
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL", "https://meta-api.vip", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_AUTH_TOKEN", "你的API密钥", [System.EnvironmentVariableTarget]::User)
```

永久设置后，重新打开 PowerShell。

### 3.2 macOS / Linux

临时设置：

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

bash 永久设置：

```bash
echo 'export ANTHROPIC_BASE_URL="https://meta-api.vip"' >> ~/.bash_profile
echo 'export ANTHROPIC_AUTH_TOKEN="你的API密钥"' >> ~/.bash_profile
source ~/.bash_profile
```

## 4. 可选：指定 Sonnet 模型

编辑 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "MODEL_CLAUDE"
  }
}
```

## 5. 使用

进入项目目录：

```bash
cd /path/to/your/project
claude
```

Windows 示例：

```powershell
cd C:\path\to\your\project
claude
```

## 6. IDE 提醒

VSCode / IDEA 插件可以显示图形化代码差异。若 IDE 一直读取旧配置，彻底退出 IDE 后重新打开。
