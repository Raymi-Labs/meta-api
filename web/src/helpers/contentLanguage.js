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

import { Lexer } from 'marked';

// Content dictionaries are separate from interface bundles and load on demand.
// Only reviewed, shipped source text is translated; administrator changes remain
// authoritative and never leave the browser for a translation service.
const HAN = /[\u3400-\u9fff]/u;
const TOKEN = /\{\{CONTENT_\d+\}\}/g;
const INTERPOLATION = /\{\{[^{}]+\}\}/g;
// These code-styled spans name visible controls, not executable code or user
// data. Keep the allowlist exact so paths, model names and commands stay opaque.
const INLINE_INTERFACE_LABELS = new Set([
  '控制台 -> 令牌管理',
  '令牌管理',
  '添加令牌',
  '分组',
  '密钥',
  '打开 CC Switch 并自动导入',
  '系统设置 -> 聊天设置',
  '聊天 -> AI 生图',
  '控制台 -> 钱包管理',
  '钱包管理 -> 账单 -> 发票',
  '控制台 -> 使用日志',
]);

export const canonicalContent = (value) =>
  typeof value === 'string' ? value.replace(/\r\n/g, '\n').trim() : '';

function emphasisBoundaries(source) {
  const openingEnds = new Set();
  const closingStarts = new Set();
  const visit = (tokens, start) => {
    let offset = start;
    for (const token of tokens) {
      const raw = token.raw || '';
      if (['strong', 'em', 'del'].includes(token.type)) {
        const width = (raw.length - token.text.length) / 2;
        const marker = raw.slice(0, width);
        if (
          (width === 1 || width === 2) &&
          /^\*{1,2}$|^_{1,2}$|^~~$/.test(marker) &&
          raw.endsWith(marker) &&
          raw.slice(width, -width) === token.text
        ) {
          openingEnds.add(offset + width);
          closingStarts.add(offset + raw.length - width);
          visit(token.tokens || [], offset + width);
        }
      } else if (token.type === 'link' || token.type === 'image') {
        const width = token.type === 'image' ? 2 : 1;
        if (raw.startsWith(`${width === 2 ? '![' : '['}${token.text}]`))
          visit(token.tokens || [], offset + width);
      }
      offset += raw.length;
    }
  };
  // The parser, rather than a star/underscore heuristic, decides which source
  // delimiters really form emphasis. Codespans and link destinations are opaque.
  if (/[*_~]/.test(source)) visit(Lexer.lexInline(source, { gfm: true }), 0);
  return { openingEnds, closingStarts };
}

function maskInline(source, transform) {
  const values = [];
  const emphasis = [];
  const { openingEnds, closingStarts } = emphasisBoundaries(source);
  const protect = (value, offset, sourceLength = value.length) => {
    const token = `{{CONTENT_${values.length}}}`;
    values.push(value);
    emphasis.push({
      trimBefore: closingStarts.has(offset),
      trimAfter: openingEnds.has(offset + sourceLength),
    });
    return token;
  };
  // Protect code, destinations, URLs, variables and Markdown punctuation. The
  // labels between link delimiters remain translatable, while hrefs never are.
  const text = source.replace(
    /(`+)[\s\S]*?\1|\{\{[^{}]+\}\}|\]\([^\n]*?\)|https?:\/\/[^\s<>，。；）]+|<[^>]+>|\bMODEL_[A-Z_]+\b|!?\[|\]|\*{1,3}|_{1,3}|~~|\\[\\`*_{}\[\]()#+\-.!|]/g,
    (value, _backticks, offset) => {
      const code = value.match(/^(`+)([\s\S]*?)\1$/);
      if (code && INLINE_INTERFACE_LABELS.has(code[2])) {
        const translated = transform(code[2]);
        if (hasValidContentTranslation(code[2], translated))
          return protect(code[1] + translated + code[1], offset, value.length);
      }
      return protect(value, offset);
    },
  );
  return { text, values, emphasis };
}

function restoreInline(text, values, emphasis) {
  // Translation services may insert spaces next to protected punctuation. Only
  // remove inner whitespace at emphasis boundaries known to be valid in the
  // source; do this before restoring opaque code/URLs or other literal values.
  return text
    .replace(/(\{\{CONTENT_(\d+)\}\})(\s+)/g, (match, token, index) =>
      emphasis[Number(index)]?.trimAfter ? token : match,
    )
    .replace(/(\s+)(\{\{CONTENT_(\d+)\}\})/g, (match, _space, token, index) =>
      emphasis[Number(index)]?.trimBefore ? token : match,
    )
    .replace(TOKEN, (token) => {
      const index = Number(token.match(/\d+/)[0]);
      return values[index] ?? token;
    });
}

function sameTokens(source, target, pattern) {
  return (
    JSON.stringify([...(source.match(pattern) || [])].sort()) ===
    JSON.stringify([...(target.match(pattern) || [])].sort())
  );
}

export function hasValidContentTranslation(source, target) {
  return (
    typeof target === 'string' &&
    target.trim().length > 0 &&
    sameTokens(source, target, INTERPOLATION)
  );
}

export function translateContentText(source, dictionary, values = {}) {
  const candidate = dictionary?.[source];
  const translated = hasValidContentTranslation(source, candidate)
    ? candidate
    : source;
  return translated.replace(/\{\{([^{}]+)\}\}/g, (token, key) =>
    Object.prototype.hasOwnProperty.call(values, key)
      ? String(values[key])
      : token,
  );
}

function mapSegment(source, transform) {
  const [, leading, body, trailing] = source.match(/^(\s*)([\s\S]*?)(\s*)$/);
  const { text, values, emphasis } = maskInline(body, transform);
  const candidate = HAN.test(text) ? transform(text) : text;
  const translated = hasValidContentTranslation(text, candidate)
    ? candidate
    : text;
  return leading + restoreInline(translated, values, emphasis) + trailing;
}

function splitMarkdownCells(line) {
  const parts = [];
  let start = 0;
  let code = '';
  for (let index = 0; index < line.length; index++) {
    if (line[index] === '\\') {
      index++;
      continue;
    }
    if (line[index] === '`') {
      const run = line.slice(index).match(/^`+/)[0];
      if (!code) code = run;
      else if (code === run) code = '';
      index += run.length - 1;
    } else if (line[index] === '|' && !code) {
      parts.push(line.slice(start, index), '|');
      start = index + 1;
    }
  }
  parts.push(line.slice(start));
  return parts;
}

function mapMarkdown(source, transform, postprocess = (line) => line) {
  let fence = null;
  let comment = false;
  return source
    .split(/(\r?\n)/)
    .map((line) => {
      if (/^\r?\n$/.test(line)) return line;
      const marker = line.match(/^\s*(`{3,}|~{3,})(.*)$/);
      if (marker) {
        if (!fence)
          fence = {
            marker: marker[1],
            template: /^(?:text|plaintext)\s+i18n-template$/.test(
              marker[2].trim(),
            ),
          };
        else if (
          marker[1][0] === fence.marker[0] &&
          marker[1].length >= fence.marker.length &&
          !marker[2].trim()
        )
          fence = null;
        return line;
      }
      if (fence) {
        if (!fence.template) return line;
        // Only deliberately marked support templates translate. Labels and the
        // yes/no choice are prose; any supplied example value remains literal.
        const field = line.match(/^(\s*)([^:：]+[:：])(\s*)(.*)$/);
        if (!field) return line;
        return (
          field[1] +
          mapSegment(field[2], transform) +
          field[3] +
          (field[4] === '是/否' ? mapSegment(field[4], transform) : field[4])
        );
      }
      if (comment || line.includes('<!--')) {
        comment = !line.includes('-->');
        return line;
      }
      if (/^( {4}|\t)/.test(line) || /^\s*(?:[-*_]\s*){3,}$/.test(line))
        return line;
      // Table cells are separate prose units, including cells without outer pipes.
      if (line.includes('|')) {
        return postprocess(
          splitMarkdownCells(line)
            .map((cell) =>
              cell === '|' || /^\s*:?-+:?\s*$/.test(cell)
                ? cell
                : mapSegment(cell, transform),
            )
            .join(''),
        );
      }
      const prefix =
        line.match(
          /^(?:\s*>\s*)*(?:\s*#{1,6}\s+|\s*[-+*]\s+|\s*\d+[.)]\s+)?/,
        )?.[0] || '';
      return postprocess(
        prefix + mapSegment(line.slice(prefix.length), transform),
      );
    })
    .join('');
}

export function extractMarkdownSources(source) {
  const keys = new Set();
  mapMarkdown(source, (text) => {
    keys.add(text);
    return text;
  });
  return [...keys];
}

function headingId(text) {
  return text
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[`*_~]/g, '')
    .replace(/\s+#+\s*$/, '')
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

export function translateMarkdown(source, dictionary) {
  if (!dictionary) return source;
  const translated = mapMarkdown(source, (text) => dictionary[text]);
  // Internal links follow translated heading IDs; external paths are preserved.
  const originalHeadings = [...source.matchAll(/^#{1,6}\s+(.+)$/gm)];
  const translatedHeadings = [...translated.matchAll(/^#{1,6}\s+(.+)$/gm)];
  const anchors = new Map(
    originalHeadings.map((match, index) => [
      headingId(match[1]),
      headingId(translatedHeadings[index]?.[1] || match[1]),
    ]),
  );
  return mapMarkdown(
    translated,
    (text) => text,
    (line) =>
      line.replace(/\]\(#([^)]*)\)/g, (link, fragment) => {
        let decoded = fragment;
        try {
          decoded = decodeURIComponent(fragment);
        } catch {
          /* Preserve invalid fragments. */
        }
        return anchors.has(decoded) ? `](#${anchors.get(decoded)})` : link;
      }),
  );
}

export function translateConfiguredDocument(
  source,
  cacheKey,
  dictionary,
  originals,
  language,
) {
  if (language === 'zh-CN')
    return { content: source, translated: false, original: false };
  const known = originals?.[cacheKey];
  if (!known || canonicalContent(source) !== canonicalContent(known))
    return { content: source, translated: false, original: true };
  const keys = extractMarkdownSources(source);
  if (
    !dictionary ||
    !keys.every((key) => hasValidContentTranslation(key, dictionary[key]))
  )
    return { content: source, translated: false, original: true };
  return {
    content: translateMarkdown(source, dictionary),
    translated: true,
    original: false,
  };
}
