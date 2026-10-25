import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { localizeInvoiceTemplate } from '../src/helpers/invoiceTemplateLanguage.js';

const require = createRequire(import.meta.url);
const { parse } = require('@babel/parser');
const traverse = require('@babel/traverse').default;

export function collectInvoiceTemplateSources() {
  const source = readFileSync(
    new URL(
      '../src/components/topup/modals/TopupHistoryModal.jsx',
      import.meta.url,
    ),
    'utf8',
  );
  const ast = parse(source, { sourceType: 'module', plugins: ['jsx'] });
  const keys = {};
  traverse(ast, {
    TaggedTemplateExpression({ node }) {
      if (node.tag.type !== 'Identifier' || node.tag.name !== 'invoiceText')
        return;
      localizeInvoiceTemplate(
        node.quasi.quasis.map((item) => item.value.cooked),
        [],
        (key) => {
          keys[key] = key;
          return key;
        },
      );
    },
  });
  return keys;
}
