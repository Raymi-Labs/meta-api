# Codex 配置教程

> 来自飞书《新平台Codex配置教程》，已整理为简洁版。

## 1. 平台准备

1. 登录 `https://meta-api.vip/`。
2. 进入 `令牌管理` 创建令牌。
3. 在模型广场找到目标模型，选择该模型当前支持且账号可选的分组。
4. 可用模型以 `https://meta-api.vip/pricing` 为准。
5. 复制完整 API Key。

令牌管理中，点击 `添加令牌`，按模型广场选择 `分组`，创建后复制 `密钥`。

## 2. 方法一：npx zcf 一键配置

### 2.1 安装 Node.js

```bash
node --version
npm --version
```

没有 Node.js 时，去 `https://nodejs.org/` 下载 LTS。

### 2.2 运行配置向导

```bash
npx zcf
```

按菜单选择：

1. 输入 `s` 切换到 `codex` 平台。
2. 首次安装选 `1`；已有配置/更新密钥选 `3`。
3. 供应商选择“自定义”。
4. 供应商名称填 `nan`。
5. URL 填 `https://meta-api.vip/v1`。
6. Key 填平台生成的 API Key。
7. MCP 建议直接回车跳过。
8. 配置完成后设为默认供应商。

## 3. 方法二：手动配置

### 3.1 安装 Codex

```bash
npm install -g @openai/codex
```

### 3.2 写入配置文件

在 `~/.codex/config.toml` 文件开头添加：

```toml
model_provider = "crs"
model = "MODEL_CODEX"
disable_response_storage = true
preferred_auth_method = "apikey"

[model_providers.crs]
name = "crs"
base_url = "https://meta-api.vip/v1"
wire_api = "responses"
requires_openai_auth = true
env_key = "NAN_API_KEY"
```

### 3.3 设置环境变量

Windows cmd：

```bat
setx NAN_API_KEY "你的API密钥"
```

macOS zsh：

```bash
echo 'export NAN_API_KEY="你的API密钥"' >> ~/.zshrc
source ~/.zshrc
```

Linux bash：

```bash
echo 'export NAN_API_KEY="你的API密钥"' >> ~/.bashrc
source ~/.bashrc
```

## 4. 验证

```bash
codex --version
codex
```

## 5. 常见问题

| 问题            | 处理                                     |
| --------------- | ---------------------------------------- |
| 401             | 确认 `NAN_API_KEY` 已生效                |
| 404             | `base_url` 必须带 `/v1`                  |
| 模型不可用      | 检查令牌分组和模型广场                   |
| Claude 模型失败 | 在模型广场核对该模型当前支持的分组和协议 |
