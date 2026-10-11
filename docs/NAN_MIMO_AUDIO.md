# MiMo V2.5 语音模型调用指南

> 本文面向通过 NAN API 网关调用小米 MiMo V2.5 语音模型的开发者，涵盖精品音色、音色设计、音色克隆和语音识别四种能力。

## 1. 模型与能力

| 模型 | 作用 | 输入 | 输出 | 推荐模式 |
| --- | --- | --- | --- | --- |
| `mimo-v2.5-tts` | 使用预置精品音色合成语音 | 风格指令、待朗读文本、音色名 | Base64 音频 | 非流式；需要时可流式 |
| `mimo-v2.5-tts-voicedesign` | 根据文字描述设计新音色并朗读 | 音色描述、待朗读文本 | Base64 音频 | 非流式 |
| `mimo-v2.5-tts-voiceclone` | 模仿参考音频中的说话人音色 | 参考音频、待朗读文本 | Base64 音频 | 非流式 |
| `mimo-v2.5-asr` | 把语音转成文字 | WAV/MP3 音频 | 文本 | 非流式 |

TTS 是 Text-to-Speech，即文字转语音；ASR 是 Automatic Speech Recognition，即语音转文字。

## 2. 协议和基础配置

四个模型统一使用 OpenAI Chat Completions 兼容协议：

```text
POST /v1/chat/completions
```

不要改用下面两个传统音频端点：

```text
/v1/audio/speech
/v1/audio/transcriptions
```

客户端配置：

| 配置项 | 填写内容 |
| --- | --- |
| API 类型 | OpenAI Compatible |
| Base URL | `https://meta-api.vip/v1` |
| API Key | 控制台令牌管理中创建的 `sk-...` |
| REST 请求地址 | `https://meta-api.vip/v1/chat/completions` |

设置环境变量：

```bash
export NAN_BASE_URL="https://meta-api.vip"
export NAN_API_KEY="sk-替换为你的平台令牌"
```

注意：平台令牌只能请求 NAN API 网关，不能作为小米官方上游令牌直接请求 `api.xiaomimimo.com`。

## 3. 通用请求结构

MiMo TTS 的 `messages` 不是普通聊天含义：

- `user`：音色、情绪、语速和表达方式等要求。
- `assistant`：真正需要合成为音频的文字。
- `audio`：输出格式、精品音色或参考音频。

基础结构：

```json
{
  "model": "mimo-v2.5-tts",
  "messages": [
    {
      "role": "user",
      "content": "请使用自然、清晰、温和的中文女声，语速适中。"
    },
    {
      "role": "assistant",
      "content": "你好，这是一段语音合成测试。"
    }
  ],
  "audio": {
    "format": "wav",
    "voice": "冰糖"
  },
  "stream": false
}
```

不要给 TTS 请求照搬普通聊天参数，例如：

```json
{
  "max_tokens": 16
}
```

小米 TTS 对参数比较严格，多余参数或缺少 `audio` 时可能返回 `Param Incorrect`。

## 4. 精品音色：mimo-v2.5-tts

### 4.1 cURL 请求

```bash
curl -sS "$NAN_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mimo-v2.5-tts",
    "messages": [
      {
        "role": "user",
        "content": "请使用自然、清晰、温和的中文女声，语速适中。"
      },
      {
        "role": "assistant",
        "content": "你好，这是小米 MiMo 语音合成模型的接口测试。"
      }
    ],
    "audio": {
      "format": "wav",
      "voice": "冰糖"
    },
    "stream": false
  }' > mimo-tts-response.json
```

`voice` 可以省略，默认使用 `mimo_default`。为了明确目标音色，建议显式填写小米支持的精品音色名；本文使用已经实测成功的 `冰糖`。

### 4.2 返回结构

音频不是直接作为 HTTP WAV 文件返回，而是在 JSON 中以 Base64 返回：

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "",
        "audio": {
          "id": "audio-id",
          "data": "UklGR..."
        }
      }
    }
  ]
}
```

### 4.3 保存音频

Linux：

```bash
jq -r '.choices[0].message.audio.data' mimo-tts-response.json \
  | base64 -d > mimo-tts.wav
```

macOS：

```bash
jq -r '.choices[0].message.audio.data' mimo-tts-response.json \
  | base64 -D > mimo-tts.wav
```

如果返回值带有 `data:audio/wav;base64,` 前缀，应先移除逗号前面的部分再解码。本文后面的 Node.js 脚本已经自动处理这两种形式。

## 5. 音色设计：mimo-v2.5-tts-voicedesign

音色设计根据自然语言描述生成一个新音色。

### 5.1 请求样例

```bash
curl -sS "$NAN_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mimo-v2.5-tts-voicedesign",
    "messages": [
      {
        "role": "user",
        "content": "二十五岁左右的中国女性声音，温柔自然、清晰有亲和力，语速适中，录音棚音质。"
      },
      {
        "role": "assistant",
        "content": "你好，这是一段根据文字描述设计出来的全新音色。"
      }
    ],
    "audio": {
      "format": "wav",
      "optimize_text_preview": true
    },
    "stream": false
  }' > mimo-voicedesign-response.json
```

保存音频：

```bash
jq -r '.choices[0].message.audio.data' mimo-voicedesign-response.json \
  | base64 -d > mimo-voicedesign.wav
```

macOS 将 `base64 -d` 改为 `base64 -D`。

### 5.2 optimize_text_preview

```json
{
  "optimize_text_preview": true
}
```

启用后，模型可能优化或扩写 `assistant.content`。对于 `mimo-v2.5-tts-voicedesign`，此时也可以省略 `assistant` 消息，由模型根据音色描述生成适合播报的文本。实际朗读文本可在下面的字段中查看：

```text
choices[0].message.final_text_preview
```

如果业务要求逐字朗读原文，应关闭该选项：

```json
{
  "optimize_text_preview": false
}
```

## 6. 音色克隆：mimo-v2.5-tts-voiceclone

音色克隆需要把参考音频作为 Data URI 放入 `audio.voice`：

```text
data:audio/wav;base64,<参考音频Base64>
```

建议使用：

- 单人、无背景音乐的清晰录音。
- WAV 或 MP3。
- 正常说话速度，不要使用明显变声、混响或电流噪声。
- 当前网关实测 12 秒 WAV 可以正常完成克隆。

小米官方当前限制为：参考音频仅支持 WAV 或 MP3，转换后的 Base64 编码字符串不得超过 10 MB。该限制针对编码后的字符串；由于 Base64 会让数据体积增大约三分之一，原始音频文件应明显小于 10 MB。

### 6.1 使用 Node.js 生成请求文件

```bash
export REFERENCE_AUDIO="./reference.wav"

node <<'NODE'
const fs = require('node:fs');

const file = process.env.REFERENCE_AUDIO;
const base64 = fs.readFileSync(file).toString('base64');
const request = {
  model: 'mimo-v2.5-tts-voiceclone',
  messages: [
    {
      role: 'user',
      content: '保持参考声音的自然语气和正常语速。',
    },
    {
      role: 'assistant',
      content: '你好，这是使用参考音频完成的音色克隆测试。',
    },
  ],
  audio: {
    format: 'wav',
    voice: `data:audio/wav;base64,${base64}`,
  },
  stream: false,
};

fs.writeFileSync('mimo-voiceclone-request.json', JSON.stringify(request));
NODE
```

### 6.2 发送请求

```bash
curl -sS "$NAN_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @mimo-voiceclone-request.json \
  > mimo-voiceclone-response.json
```

保存结果：

```bash
jq -r '.choices[0].message.audio.data' mimo-voiceclone-response.json \
  | base64 -d > mimo-voiceclone.wav
```

## 7. 语音识别：mimo-v2.5-asr

ASR 将语音转换成文字。当前网关建议输入 WAV 或 MP3，以便在转发前准确计算音频时长和费用。

### 7.1 构建 Data URI

```bash
export INPUT_AUDIO="./input.wav"

node <<'NODE'
const fs = require('node:fs');

const file = process.env.INPUT_AUDIO;
const ext = file.toLowerCase().endsWith('.mp3') ? 'mpeg' : 'wav';
const base64 = fs.readFileSync(file).toString('base64');
const request = {
  model: 'mimo-v2.5-asr',
  messages: [
    {
      role: 'user',
      content: [
        {
          type: 'input_audio',
          input_audio: {
            data: `data:audio/${ext};base64,${base64}`,
          },
        },
      ],
    },
  ],
  stream: false,
};

fs.writeFileSync('mimo-asr-request.json', JSON.stringify(request));
NODE
```

### 7.2 发送请求

```bash
curl -sS "$NAN_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @mimo-asr-request.json \
  > mimo-asr-response.json
```

查看识别结果：

```bash
jq -r '.choices[0].message.content' mimo-asr-response.json
```

返回示例：

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "你好，这是一段语音识别测试。"
      }
    }
  ],
  "usage": {
    "seconds": 13
  }
}
```

### 7.3 ASR 计费

当前项目按照输入音频的实际时长计费：

```text
mimo-v2.5-asr：0.074 美元/小时，约合 0.5 元/小时
```

计费过程：

1. 网关解析输入音频。
2. 读取真实时长。
3. 按整秒向上取整。
4. 结合用户分组倍率和时段倍率结算。

为了避免无法测量时长，优先使用带完整文件头的 WAV 或 MP3，不建议发送裸 PCM。

### 7.4 asr_options 注意事项

小米支持通过 `asr_options.language` 指定语言，例如：

```json
{
  "asr_options": {
    "language": "zh"
  }
}
```

当前项目版本的通用 Chat 请求 DTO 尚未显式定义 `asr_options`。在默认请求转换模式下，该字段会被过滤；只有开启全局或渠道的“请求体透传”后，字段才会原样发送到上游。

未开启请求体透传时，建议省略 `asr_options`，由上游使用默认的 `auto` 自动识别。开启请求体透传后，可使用 `auto`、`zh` 或 `en`。

## 8. 一键测试四个模型

仓库提供了可直接运行的 Node.js 示例：

```text
scripts/mimo_audio_smoke_test.mjs
```

运行条件：Node.js 18 或更高版本。

```bash
export NAN_BASE_URL="https://meta-api.vip"
export NAN_API_KEY="sk-替换为你的平台令牌"
export MIMO_TTS_VOICE="冰糖"

node scripts/mimo_audio_smoke_test.mjs
```

默认输出目录：

```text
./mimo-audio-output
```

脚本执行顺序：

1. 用 `mimo-v2.5-tts` 生成精品音色 WAV。
2. 用 `mimo-v2.5-tts-voicedesign` 生成设计音色 WAV。
3. 把第一步音频作为参考，调用 `mimo-v2.5-tts-voiceclone`。
4. 把第一步音频交给 `mimo-v2.5-asr` 转写。

脚本不会把 API Key 写入请求文件或输出文件。

## 9. 流式音频

### 9.1 低延迟流式 TTS

`mimo-v2.5-tts` 支持低延迟流式输出。流式请求应使用 `pcm16`：

```json
{
  "model": "mimo-v2.5-tts",
  "messages": [
    {
      "role": "user",
      "content": "请使用自然、清晰的中文女声。"
    },
    {
      "role": "assistant",
      "content": "你好，这是一段流式语音测试。"
    }
  ],
  "audio": {
    "format": "pcm16",
    "voice": "冰糖"
  },
  "stream": true
}
```

每个 SSE 分片的音频位于：

```text
choices[0].delta.audio.data
```

其中 `data` 是 Base64 编码的 24 kHz、16-bit、单声道 PCM16LE 数据。它不是带文件头的完整 WAV，需要依次解码并拼接；如需保存为 WAV，还要补充 WAV 文件头或使用音频库封装。

### 9.2 网关渠道设置

所有 MiMo TTS 请求，包括流式和非流式，都必须关闭：

```text
强制格式化
```

流式 TTS 还必须关闭：

```text
思维内容转正文
```

原因是 MiMo 音频位于普通 Chat 文本响应 DTO 未定义的扩展字段中；如果网关解析后重新序列化，可能丢失 `message.audio`、`final_text_preview` 或 `delta.audio`。

### 9.3 其他模型

- `mimo-v2.5-tts-voicedesign` 和 `mimo-v2.5-tts-voiceclone` 接受 `stream: true`，但目前属于兼容模式：推理全部完成后，再一次性以流式格式返回，不具备真正的低延迟效果，因此建议使用非流式。
- `mimo-v2.5-asr` 支持 SSE 流式识别，可按业务需要选择流式或非流式。

## 10. 常见问题

### 10.1 Param Incorrect

常见原因：

- 使用了普通文本模型测试请求。
- 缺少 `audio`。
- 没有 `assistant` 消息或待朗读文本为空；但 `mimo-v2.5-tts-voicedesign` 配合 `optimize_text_preview: true` 时允许省略 `assistant`。
- `audio.voice` 填写了上游不支持的精品音色名。
- 请求携带了 TTS 不接受的普通聊天参数，例如 `max_tokens`。

当前版本的管理后台已经为三个 MiMo TTS 模型构建专用测试请求，并将音色设计、音色克隆测试固定为非流式。如果仍然返回 `Param Incorrect`，请检查模型映射、渠道配置和部署版本。

### 10.2 401 Invalid API Key

检查：

- 是否使用平台生成的完整 `sk-...` 令牌。
- Base URL 是否为 `https://meta-api.vip/v1`。
- 是否误把平台令牌发给小米官方域名。

### 10.3 返回成功但没有音频

TTS 音频位于：

```text
choices[0].message.audio.data
```

不要只读取：

```text
choices[0].message.content
```

TTS 的 `message.content` 通常为空字符串。

### 10.4 ASR 模型没有出现在 /v1/models

现网可能出现 `mimo-v2.5-asr` 未在模型列表展示、但直接请求可以成功的情况。调用前可以先测试真实请求；管理员应同步补齐模型目录和厂商分类。

### 10.5 音频时长计费报错

如果出现“无法测量输入音频时长”：

- 改用 WAV 或 MP3。
- 检查 Base64 是否完整。
- Data URI 的 MIME 类型是否与实际格式一致。
- 不要把损坏文件或纯文本伪装成音频。

### 10.6 音色克隆请求过大

Base64 会增加请求体积。建议：

- 裁剪无关静音片段。
- 使用足够清晰但不过长的参考音频。
- 使用 `--data-binary @request.json`，不要把超长 Base64 直接写入命令行参数。

## 11. 已验证结果

当前网关已经使用真实请求验证：

| 模型 | HTTP 状态 | 验证内容 |
| --- | --- | --- |
| `mimo-v2.5-tts` | 200 | 成功生成 24 kHz、16-bit、单声道 WAV |
| `mimo-v2.5-tts-voicedesign` | 200 | 成功生成音色设计 WAV，并返回 `final_text_preview` |
| `mimo-v2.5-tts-voiceclone` | 200 | 使用前一步 12 秒参考音频成功克隆并生成 WAV |
| `mimo-v2.5-asr` | 200 | 成功识别生成音频中的完整中文内容 |

## 12. 官方参考

- [MiMo V2.5 语音合成](https://mimo.mi.com/docs/quick-start/usage-guide/audio/speech-synthesis-v2.5)
- [MiMo V2.5 ASR](https://mimo.mi.com/docs/quick-start/usage-guide/audio/asr-v2.5)
- [MiMo 模型概览](https://mimo.mi.com/docs/quick-start/summary/model)
