# OpenCode 配置教程

> 来自飞书《新平台OpenCode 配置教程》。

## 1. 平台准备

1. 登录 `https://meta-api.vip/`。
2. 进入 `令牌管理` 创建令牌。
3. 在模型广场找到目标模型，选择该模型当前支持且账号可选的分组。
4. 复制完整 API Key。

## 2. 安装环境

```bash
node --version
npm --version
npm install -g opencode-ai
opencode --version
```

Windows 如遇权限问题，用管理员 PowerShell；macOS/Linux 如遇权限问题：

```bash
sudo npm install -g opencode-ai
```

## 3. 配置环境变量

OpenCode 配置里使用 `CRS_OAI_KEY`。

Windows cmd：

```bat
set CRS_OAI_KEY=你的API密钥
```

macOS zsh：

```bash
echo 'export CRS_OAI_KEY="你的API密钥"' >> ~/.zshrc
source ~/.zshrc
```

Linux bash：

```bash
echo 'export CRS_OAI_KEY="你的API密钥"' >> ~/.bashrc
source ~/.bashrc
```

## 4. 配置 OpenCode

在 OpenCode 配置文件中添加。模型名由在线文档同步；上下文长度、模态及思考参数请按目标模型能力另行设置：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "nan": {
      "npm": "@ai-sdk/openai",
      "name": "NAN OpenAI Proxy",
      "options": {
        "baseURL": "https://meta-api.vip/v1",
        "apiKey": "{env:CRS_OAI_KEY}"
      },
      "models": {
        "MODEL_OPENAI": {
          "name": "MODEL_OPENAI"
        }
      }
    }
  },
  "model": "nan/MODEL_OPENAI"
}
```

## 5. 使用

```bash
opencode
```

## 6. 常见问题

| 问题       | 处理                    |
| ---------- | ----------------------- |
| 401        | 确认 `CRS_OAI_KEY` 生效 |
| 404        | `baseURL` 必须带 `/v1`  |
| 模型不可用 | 检查模型广场和令牌分组  |
