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

// Translate template-owned text before inserting account/order data. The
// translator never sees names, bank details, amounts or document identifiers.
const FIXED_TEXT = [
  '上海曜算智能科技有限公司',
  '公司公章',
  '人民币',
  'UTC+08:00',
  '曜算',
];
const PLACEHOLDERS = /\{\{PRINT_(?:VALUE|FIXED)_\d+\}\}/g;
const escapeText = (text) =>
  text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const valid = (source, result) =>
  typeof result === 'string' &&
  result.trim() &&
  JSON.stringify((source.match(PLACEHOLDERS) || []).sort()) ===
    JSON.stringify((result.match(PLACEHOLDERS) || []).sort());

export function localizeInvoiceTemplate(strings, values, translate) {
  const template = strings
    .map(
      (part, index) =>
        part + (index < strings.length - 1 ? `{{PRINT_VALUE_${index}}}` : ''),
    )
    .join('');
  const html = /<\/?[a-z][\s\S]*?>|<!doctype/i.test(template);
  let blocked = null;
  const result = template
    .split(/(<[^>]*>)/g)
    .map((part) => {
      if (html && /^</.test(part)) {
        const opening = part.match(/^<(style|script)\b/i);
        if (opening) blocked = opening[1].toLowerCase();
        if (blocked && new RegExp(`^</${blocked}\\s*>`, 'i').test(part))
          blocked = null;
        return part;
      }
      if (blocked || !/[\u3400-\u9fff]/u.test(part)) return part;
      const [, leading, body, trailing] = part.match(/^(\s*)([\s\S]*?)(\s*)$/);
      let source = body;
      const fixed = [];
      for (const protectedText of FIXED_TEXT) {
        source = source.replaceAll(protectedText, () => {
          const token = `{{PRINT_FIXED_${fixed.length}}}`;
          fixed.push(protectedText);
          return token;
        });
      }
      if (!/[\u3400-\u9fff]/u.test(source)) return part;
      const candidate = translate(source);
      let translated = valid(source, candidate) ? candidate : source;
      if (html) translated = escapeText(translated);
      translated = translated.replace(
        /\{\{PRINT_FIXED_(\d+)\}\}/g,
        (token, index) => fixed[Number(index)] ?? token,
      );
      return leading + translated + trailing;
    })
    .join('');
  // A single replacement pass prevents inserted data from becoming a second
  // template. Callers retain their existing HTML-escaping at each value boundary.
  return result.replace(/\{\{PRINT_VALUE_(\d+)\}\}/g, (token, index) =>
    Number(index) < values.length ? String(values[Number(index)]) : token,
  );
}

export const createInvoiceTemplateTranslator =
  (translate) =>
  (strings, ...values) =>
    localizeInvoiceTemplate(strings, values, translate);
