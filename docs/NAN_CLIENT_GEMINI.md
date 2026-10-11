# Gemini CLI 配置教程

> 来自飞书《新平台配置Gemini的教程》。

## 1. 平台准备

1. 登录 `https://meta-api.vip/`。
2. 进入 `令牌管理` 创建令牌。
3. 在模型广场找到目标模型，选择该模型当前支持且账号可选的分组。
4. 可用模型以 `https://meta-api.vip/pricing` 为准。
5. 复制完整 API Key。

## 2. 安装环境

```bash
node --version
npm --version
npm install -g @google/gemini-cli
gemini --version
```

没有 Node.js 时，去 `https://nodejs.org/` 下载 LTS。

Linux / macOS 如遇权限问题，命令前加 `sudo`。

## 3. 配置环境变量

需要三个变量：

```text
GOOGLE_GEMINI_BASE_URL=https://meta-api.vip
GEMINI_API_KEY=你的API密钥
GEMINI_MODEL=MODEL_GEMINI
```

### 3.1 Windows PowerShell

临时设置：

```powershell
$env:GOOGLE_GEMINI_BASE_URL = "https://meta-api.vip"
$env:GEMINI_API_KEY = "你的API密钥"
$env:GEMINI_MODEL = "MODEL_GEMINI"
```

永久设置：

```powershell
[System.Environment]::SetEnvironmentVariable("GOOGLE_GEMINI_BASE_URL", "https://meta-api.vip", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("GEMINI_API_KEY", "你的API密钥", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("GEMINI_MODEL", "MODEL_GEMINI", [System.EnvironmentVariableTarget]::User)
```

### 3.2 macOS / Linux

临时设置：

```bash
export GOOGLE_GEMINI_BASE_URL="https://meta-api.vip"
export GEMINI_API_KEY="你的API密钥"
export GEMINI_MODEL="MODEL_GEMINI"
```

zsh 永久设置：

```bash
echo 'export GOOGLE_GEMINI_BASE_URL="https://meta-api.vip"' >> ~/.zshrc
echo 'export GEMINI_API_KEY="你的API密钥"' >> ~/.zshrc
echo 'export GEMINI_MODEL="MODEL_GEMINI"' >> ~/.zshrc
source ~/.zshrc
```

bash 永久设置：

```bash
echo 'export GOOGLE_GEMINI_BASE_URL="https://meta-api.vip"' >> ~/.bash_profile
echo 'export GEMINI_API_KEY="你的API密钥"' >> ~/.bash_profile
echo 'export GEMINI_MODEL="MODEL_GEMINI"' >> ~/.bash_profile
source ~/.bash_profile
```

## 4. 验证

Windows：

```powershell
echo $env:GOOGLE_GEMINI_BASE_URL
echo $env:GEMINI_API_KEY
echo $env:GEMINI_MODEL
```

macOS / Linux：

```bash
echo $GOOGLE_GEMINI_BASE_URL
echo $GEMINI_API_KEY
echo $GEMINI_MODEL
```

## 5. 使用

```bash
gemini
```

可以在 Gemini CLI 内通过 `/model` 切换模型。
