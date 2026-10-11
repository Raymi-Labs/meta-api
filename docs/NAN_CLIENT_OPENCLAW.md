# OpenClaw 配置教程

> 来自飞书《新平台配置openclaw教程》。OpenClaw 有本地网关和安全风险，使用前确认你理解本地服务、端口暴露和 API Key 泄露风险。

## 1. 平台准备

1. 登录 `https://meta-api.vip/`。
2. 进入 `令牌管理` 创建令牌。
3. 在模型广场找到目标模型，选择该模型当前支持且账号可选的分组。
4. 可用模型以 `https://meta-api.vip/pricing` 为准。
5. 复制完整 API Key。

## 2. 安装前准备

### 2.1 Node.js 和 Git

Git 下载地址：`https://git-scm.cn/`。

```bash
node --version
npm --version
```

没有 Node.js 时，去 `https://nodejs.org/` 下载 LTS。

### 2.2 环境变量

也可以把 API Key 明文写进配置文件，但更建议用环境变量。

Windows：

```bat
setx OPI_AUTH_TOKEN "你的key"
```

macOS zsh：

```bash
echo 'export OPI_AUTH_TOKEN="你的key"' >> ~/.zshrc
source ~/.zshrc
```

Linux bash：

```bash
echo 'export OPI_AUTH_TOKEN="你的key"' >> ~/.bashrc
source ~/.bashrc
```

## 3. 安装 OpenClaw

Windows 用 CMD 或 PowerShell；权限不足时用管理员运行。

```bash
npm i -g openclaw
openclaw onboard
```

onboard 时建议：

- 认真阅读安全风险提示。
- 选择快速开始配置。
- 第三方聊天平台按需配置，不需要可以跳过。
- 服务模式可选；担心风险时先不要安装服务，用前台运行。

## 4. 配置文件位置

| 系统    | 路径                                             |
| ------- | ------------------------------------------------ |
| Windows | `C:\Users\Administrator\.openclaw\openclaw.json` |
| macOS   | `~/.openclaw/openclaw.json`                      |
| Linux   | `/home/你的用户名/.openclaw/openclaw.json`       |

不要整份覆盖已有复杂配置；只替换或合并差异字段。

## 5. 单独配置 Codex / OpenAI Responses

```json
{
  "agents": {
    "defaults": {
      "model": { "primary": "crs/MODEL_CODEX" },
      "models": {
        "crs/MODEL_CODEX": { "alias": "codex" }
      }
    }
  },
  "models": {
    "mode": "replace",
    "providers": {
      "crs": {
        "baseUrl": "https://meta-api.vip/v1",
        "api": "openai-responses",
        "auth": "api-key",
        "authHeader": true,
        "headers": { "User-Agent": "macos-terminal-test" },
        "apiKey": "你的key",
        "models": [
          {
            "id": "MODEL_CODEX",
            "name": "MODEL_CODEX",
            "api": "openai-responses",
            "cost": { "input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0 }
          }
        ]
      }
    }
  }
}
```

## 6. 单独配置 Opus / Claude

```json
{
  "agents": {
    "defaults": {
      "model": { "primary": "opus/MODEL_CLAUDE" },
      "models": {
        "opus/MODEL_CLAUDE": { "alias": "opus" }
      }
    }
  },
  "models": {
    "mode": "replace",
    "providers": {
      "opus": {
        "baseUrl": "https://meta-api.vip/",
        "api": "anthropic-messages",
        "auth": "api-key",
        "authHeader": true,
        "apiKey": "你的key",
        "models": [
          {
            "id": "MODEL_CLAUDE",
            "name": "MODEL_CLAUDE",
            "api": "anthropic-messages"
          }
        ]
      }
    }
  }
}
```

## 7. Codex 和 Opus 合并配置

```json
{
  "agents": {
    "defaults": {
      "model": { "primary": "opus/MODEL_CLAUDE" },
      "models": {
        "opus/MODEL_CLAUDE": { "alias": "opus" },
        "crs/MODEL_CODEX": { "alias": "codex" }
      }
    }
  },
  "models": {
    "mode": "replace",
    "providers": {
      "crs": {
        "baseUrl": "https://meta-api.vip/v1",
        "api": "openai-responses",
        "auth": "api-key",
        "authHeader": true,
        "headers": { "User-Agent": "macos-terminal-test" },
        "apiKey": "你的key",
        "models": [
          {
            "id": "MODEL_CODEX",
            "name": "MODEL_CODEX",
            "api": "openai-responses",
            "cost": { "input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0 }
          }
        ]
      },
      "opus": {
        "baseUrl": "https://meta-api.vip/",
        "api": "anthropic-messages",
        "auth": "api-key",
        "authHeader": true,
        "apiKey": "你的key",
        "models": [
          {
            "id": "MODEL_CLAUDE",
            "name": "MODEL_CLAUDE",
            "api": "anthropic-messages"
          }
        ]
      }
    }
  }
}
```

字段说明：

- 输入模态和思考参数请按目标模型能力另行设置。
- `cost`：仅用于成本统计/显示，不影响请求。
- 模型名由在线文档按协议同步；仍显示 MODEL\_\* 时，须先确认对应协议的模型。

## 8. 启动和检查

服务模式：

```bash
openclaw gateway stop
openclaw gateway start
openclaw gateway restart
```

如果提示 `Scheduled Task not installed`，先安装服务：

```bash
openclaw gateway install
openclaw gateway start
```

前台运行：

```bash
openclaw gateway
# 或
openclaw gateway run
```

检查状态：

```bash
openclaw status
```

看到 `Gateway ... reachable` 才算成功。

## 9. 常用命令

```bash
openclaw status
openclaw logs --follow
openclaw dashboard --no-open
openclaw tui
```

| 方式    | 命令                           | 说明       |
| ------- | ------------------------------ | ---------- |
| WebChat | `openclaw dashboard --no-open` | 浏览器页面 |
| CLI     | `openclaw tui`                 | 终端聊天   |

## 10. 常见问题

| 问题                            | 处理                                          |
| ------------------------------- | --------------------------------------------- |
| 权限不足                        | Windows 用管理员命令行                        |
| `gateway not connected`         | 先执行 `openclaw status`                      |
| `LLM request timed out`         | 关闭代理后重试，检查网络和防火墙              |
| `400 Instructions are required` | 确认 OpenClaw 已更新，配置是 Responses 新格式 |
| 配置脚本失败                    | 手动合并 `openclaw.json`，不要覆盖多模型配置  |
