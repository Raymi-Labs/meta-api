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

export const MODEL_CAPABILITY_LABELS = {
  reasoning: '深度思考',
  streaming: '流式输出',
  function_calling: '函数调用',
  web_search: '联网搜索',
  url_context: 'URL 上下文',
  code_interpreter: '代码解释器',
  computer_use: '计算机操作',
  file_search: '文件检索',
  memory: '记忆工具',
  structured_output: '结构化输出',
  structured_decision: '结构化决策',
  citations: '引用标注',
  prompt_caching: '提示缓存',
  background: '后台执行',
  server_side_context: '服务端多轮',
};

export function modelCapabilityStatus(value) {
  return value === true ? '支持' : value === false ? '不支持' : '未知';
}

export function formatModelTokenLimit(value) {
  const tokens = Number(value);
  if (!Number.isSafeInteger(tokens) || tokens <= 0) return '—';
  if (tokens >= 1e6) return `${Number((tokens / 1e6).toFixed(2))}M tokens`;
  if (tokens >= 1e3) return `${Number((tokens / 1e3).toFixed(2))}K tokens`;
  return `${tokens} tokens`;
}

export function modelAttributeSource(model, field, configured) {
  if (!configured) return { label: '未知' };
  const source = model.attribute_sources?.[field];
  if (!source) return { label: '人工配置' };
  let url;
  try {
    const parsed = new URL(source.url);
    if (
      ['https:', 'http:'].includes(parsed.protocol) &&
      !parsed.username &&
      !parsed.password
    )
      url = parsed.href;
  } catch {
    /* Invalid source URLs remain non-clickable. */
  }
  return { label: source.kind === 'official' ? '厂商官方' : '模型库补全', url };
}
