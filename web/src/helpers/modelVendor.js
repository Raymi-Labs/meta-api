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

const MIMO_MODEL_PATTERN = /(^|[/:._-])mimo($|[/:._-])/i;

// MiMo models contain a standalone mimo segment. This avoids classifying all
// models in a broad xiaomi namespace as MiMo.
export const isMiMoModel = (modelName) => {
  const normalizedName = String(modelName ?? '').trim();
  return MIMO_MODEL_PATTERN.test(normalizedName);
};

const MERCURY_MODEL_PATTERN = /(^|[/:._-])mercury($|[/:._-])/i;
// Match the other channelVendorSegments in model/channel_vendor.go: mixed
// vendor names must not become an Inception model merely due to a namespace.
const OTHER_VENDOR_PATTERN =
  /(^|[/:._-])(mimo|deepseek|gpt|chatgpt|o1|o3|o4|dall|whisper|claude|gemini|gemma|qwen|qwen2|qwen3|qwq|qvq|kimi|moonshot|glm|chatglm|grok|minimax|mistral|mixtral|codestral|pixtral|ministral|devstral)($|[/:._-])/i;

export const isInceptionModel = (modelName) => {
  const name = String(modelName ?? '').trim();
  return MERCURY_MODEL_PATTERN.test(name) && !OTHER_VENDOR_PATTERN.test(name);
};
