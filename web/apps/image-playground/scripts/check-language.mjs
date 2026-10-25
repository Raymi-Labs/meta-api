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

// Run from any directory:
// node scripts/check-language.mjs               Check source and all 16 targets.
// node scripts/check-language.mjs --source-only Check source coverage only.
// node scripts/check-language.mjs --extract     Add new source keys for translation.
// Extraction never removes keys or rewrites a translated resource.
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { SITE_LANGUAGES } from '../../../src/helpers/localeNormalize.js'

const appRoot = fileURLToPath(new URL('../', import.meta.url))
const han = /\p{Script=Han}/u
const dataSources = {
  'lib/ecommerce.ts': {
    ECOMMERCE_GROUPS: ['label', 'description'],
    PRODUCT_ASSET_BUCKETS: ['label', 'hint'],
    ECOMMERCE_PLATFORMS: '*',
    STYLE_PRESETS: ['name', 'description'],
    SUITE_TEMPLATES: ['name', 'description'],
    SIZE_PRESETS: ['label'],
    // The brief is editable prompt data. Only the preset menu name is UI.
    INDUSTRY_PRESETS: ['name'],
  },
  'lib/bucketGuides.ts': {
    BUCKET_GUIDES: ['title', 'intro', 'name', 'description'],
  },
  'components/MarkdownRenderer.tsx': { translations: '*' },
}

export function collectSourceKeys(source, filename = 'component.tsx') {
  const file = ts.createSourceFile(
    filename,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  )
  const keys = new Map()
  const declarations = new Map()
  const add = (key, node) => {
    if (key && han.test(key))
      keys.set(
        key,
        file.getLineAndCharacterOfPosition(node.getStart(file)).line + 1,
      )
  }
  const visit = (node, action) => {
    action(node)
    ts.forEachChild(node, (child) => visit(child, action))
  }
  visit(file, (node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer
    )
      declarations.set(node.name.text, node.initializer)
  })
  const read = (node, seen = new Set()) => {
    if (!node) return
    if (ts.isStringLiteralLike(node)) add(node.text, node)
    else if (ts.isTemplateExpression(node))
      add(
        node.head.text +
          node.templateSpans
            .map((part, index) => `{{value${index}}}` + part.literal.text)
            .join(''),
        node,
      )
    else if (
      ts.isIdentifier(node) &&
      declarations.has(node.text) &&
      !seen.has(node.text)
    ) {
      read(declarations.get(node.text), new Set([...seen, node.text]))
    } else if (ts.isConditionalExpression(node)) {
      read(node.whenTrue, seen)
      read(node.whenFalse, seen)
    } else if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node))
      read(node.expression, seen)
  }
  visit(file, (node) => {
    if (!ts.isCallExpression(node)) return
    const name = ts.isIdentifier(node.expression)
      ? node.expression.text
      : ts.isPropertyAccessExpression(node.expression)
        ? node.expression.name.text
        : ''
    if (['t', 'tsource', 'translateMessage', 'showToast'].includes(name))
      read(node.arguments[0])
  })
  for (const [name, fields] of Object.entries(
    dataSources[filename.replaceAll('\\', '/')] ?? {},
  )) {
    const initializer = declarations.get(name)
    if (!initializer)
      throw new Error(
        `${filename}: UI data source ${name} disappeared; update the coverage contract`,
      )
    visit(initializer, (node) => {
      if (fields === '*' && ts.isStringLiteralLike(node)) read(node)
      else if (
        ts.isPropertyAssignment(node) &&
        fields.includes(node.name.getText(file))
      )
        read(node.initializer)
    })
  }
  return keys
}

export function validateDictionaries(source, targets) {
  const errors = []
  const tokens = (value) =>
    (value.match(/\{\{[^{}]+\}\}/g) ?? []).sort().join('\n')
  for (const [key, value] of Object.entries(source)) {
    if (!key.trim() || value !== key)
      errors.push(
        `zh-CN: source value must exactly equal its nonempty key: ${key}`,
      )
  }
  for (const [language, dictionary] of Object.entries(targets)) {
    if (!dictionary) {
      errors.push(`${language}: missing locale resource`)
      continue
    }
    for (const key of Object.keys(source)) {
      const value = dictionary[key]
      if (typeof value !== 'string' || !value.trim())
        errors.push(`${language}: missing or empty: ${key}`)
      else {
        if (tokens(key) !== tokens(value))
          errors.push(`${language}: interpolation mismatch: ${key}`)
        if (/ZX[QVE]\s*\d{5}/i.test(value))
          errors.push(`${language}: temporary translation marker: ${key}`)
      }
    }
  }
  return errors
}

function sourceFiles(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(directory, entry.name)
    if (entry.isDirectory())
      return entry.name === 'locales' ? [] : sourceFiles(full)
    return /\.(ts|tsx)$/.test(entry.name) &&
      !/\.(test|spec|d)\./.test(entry.name)
      ? [full]
      : []
  })
}

export function checkLanguage({
  root = appRoot,
  extract = false,
  sourceOnly = false,
} = {}) {
  const sourceRoot = path.join(root, 'src')
  const sourcePath = path.join(sourceRoot, 'locales/zh-CN.json')
  const baseline = JSON.parse(fs.readFileSync(sourcePath, 'utf8'))
  const errors = []
  let added = 0
  let references = 0
  for (const filename of sourceFiles(sourceRoot)) {
    const relative = path.relative(sourceRoot, filename).replaceAll('\\', '/')
    const keys = collectSourceKeys(fs.readFileSync(filename, 'utf8'), relative)
    references += keys.size
    for (const [key, line] of keys) {
      if (Object.hasOwn(baseline, key)) continue
      if (extract) {
        baseline[key] = key
        added++
      } else errors.push(`${relative}:${line}: source key missing: ${key}`)
    }
  }
  if (extract)
    fs.writeFileSync(sourcePath, `${JSON.stringify(baseline, null, 2)}\n`)
  const targets = {}
  if (!extract && !sourceOnly) {
    for (const [language] of SITE_LANGUAGES) {
      if (language === 'zh-CN') continue
      const target = path.join(sourceRoot, 'locales', `${language}.json`)
      try {
        targets[language] = JSON.parse(fs.readFileSync(target, 'utf8'))
      } catch (error) {
        errors.push(
          `${language}: cannot read resource: ${error.code ?? error.message}`,
        )
        targets[language] = null
      }
    }
  }
  errors.push(...validateDictionaries(baseline, targets))
  return {
    errors,
    sourceKeys: Object.keys(baseline).length,
    references,
    targets: Object.keys(targets).length,
    added,
  }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const result = checkLanguage({
    extract: process.argv.includes('--extract'),
    sourceOnly: process.argv.includes('--source-only'),
  })
  console.log(
    `Image language coverage: ${result.sourceKeys} source keys, ${result.references} source references, ${result.targets} target resources, ${result.added} added`,
  )
  if (result.errors.length) {
    console.error(result.errors.slice(0, 60).join('\n'))
    if (result.errors.length > 60)
      console.error(`... ${result.errors.length - 60} more failures`)
    process.exitCode = 1
  }
}
