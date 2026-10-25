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

import { DOCS } from './catalog';

const DOC_LINK_REPLACEMENTS = [
  [/\.\/NAN_USAGE\.md/g, '/docs'],
  [/\.\/NAN_CLIENTS\.md/g, '/docs/clients'],
  [/\.\/NAN_CLIENT_CODEX\.md/g, '/docs/clients/codex'],
  [/\.\/NAN_CLIENT_CLAUDE_CODE\.md/g, '/docs/clients/claude-code'],
  [
    /\.\/NAN_CLIENT_CLAUDE_CODE_OPENAI\.md/g,
    '/docs/clients/claude-code-openai',
  ],
  [/\.\/NAN_CLIENT_GEMINI\.md/g, '/docs/clients/gemini'],
  [/\.\/NAN_CLIENT_OPENCODE\.md/g, '/docs/clients/opencode'],
  [/\.\/NAN_CLIENT_OPENCLAW\.md/g, '/docs/clients/openclaw'],
  [/\.\/NAN_CLIENT_CCSWITCH\.md/g, '/docs/clients/ccswitch'],
  [/\.\/NAN_IMAGE_GENERATION\.md/g, '/docs/image-generation'],
  [/\.\/NAN_API_EXAMPLES\.md/g, '/docs/api-examples'],
  [/\.\/NAN_TROUBLESHOOTING\.md/g, '/docs/troubleshooting'],
  [/\.\/NAN_MIMO_AUDIO\.md/g, '/docs/mimo-audio'],
  // 图片放 /docs-images/（不能叫 /docs/images/：dist 里出现 docs 目录会让
  // 静态文件服务把 /docs 当目录发 301，与 SPA 路由打架导致重定向死循环）
  [/\.\/images\//g, '/docs-images/'],
];

export const NAV_GROUPS = DOCS.reduce((groups, doc) => {
  if (!groups.includes(doc.group)) {
    groups.push(doc.group);
  }
  return groups;
}, []);

export function normalizeMarkdownLinks(content) {
  return DOC_LINK_REPLACEMENTS.reduce(
    (current, [pattern, replacement]) => current.replace(pattern, replacement),
    content,
  );
}

function normalizeHeadingText(value) {
  return value
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[`*_~]/g, '')
    .replace(/\s+#+\s*$/, '')
    .trim();
}

function createHeadingId(value) {
  return normalizeHeadingText(value)
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');
}

export function prepareMarkdown(content) {
  return normalizeMarkdownLinks(content).replace(/^\uFEFF?#\s+[^\n]+\n+/, '');
}

export function extractHeadings(content) {
  let inCodeBlock = false;
  return content.split('\n').reduce((headings, line) => {
    if (/^\s*```/.test(line)) {
      inCodeBlock = !inCodeBlock;
      return headings;
    }
    if (inCodeBlock) {
      return headings;
    }

    const match = line.match(/^(##|###)\s+(.+)$/);
    if (!match) {
      return headings;
    }

    const title = normalizeHeadingText(match[2]);
    const id = createHeadingId(title);
    if (title && id) {
      headings.push({
        id,
        title,
        level: match[1].length,
      });
    }
    return headings;
  }, []);
}

export const DOCS_ORIGIN = 'https://docs.meta-api.vip';
export const MAIN_ORIGIN = 'https://meta-api.vip';

export function docPath(path, standalone = false) {
  return standalone ? path.replace(/^\/docs(?=\/|$)/, '') || '/' : path;
}

export function getActiveDoc(pathname, standalone = false) {
  const path = pathname.replace(/\/+$/, '') || '/';
  const canonical =
    standalone && !/^\/docs(?:\/|$)/.test(path)
      ? `/docs${path === '/' ? '' : path}`
      : path;
  return DOCS.find((doc) => doc.path === canonical) || null;
}

export function searchDocs(query, documents) {
  const words = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return [];
  return documents
    .map((doc) => {
      const title = doc.title.toLocaleLowerCase();
      const description = doc.description.toLocaleLowerCase();
      const text = `${title} ${description} ${doc.content}`.toLocaleLowerCase();
      return {
        ...doc,
        score: words.reduce(
          (sum, word) =>
            sum +
            (title.includes(word) ? 10 : description.includes(word) ? 3 : 1),
          0,
        ),
        matches: words.every((word) => text.includes(word)),
      };
    })
    .filter((doc) => doc.matches)
    .sort((a, b) => b.score - a.score);
}

// Rewrite only Markdown destinations, never code or API example addresses.
export function standaloneMarkdown(content, language) {
  let fenced = false;
  return content
    .split('\n')
    .map((line) => {
      if (/^\s*```/.test(line)) {
        fenced = !fenced;
        return line;
      }
      if (fenced) return line;
      return line.replace(
        /(\]\()(https:\/\/meta-api\.vip)?(\/(?!\/)[^\s)]*)(\))/g,
        (all, open, origin, path, close) => {
          if (/^\/docs(?:\/|$|[?#])/.test(path))
            return `${open}${path.replace(/^\/docs(?=\/|$|[?#])/, '') || '/'}${close}`;
          if (/^\/docs-images\//.test(path)) return all;
          const url = new URL(path, MAIN_ORIGIN);
          if (!/^\/(downloads|api|v1)(?:\/|$)/.test(path))
            url.searchParams.set('lang', language);
          return `${open}${url.href}${close}`;
        },
      );
    })
    .join('\n');
}
