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

import { useSyncExternalStore } from 'react'
// The host and standalone application must agree on aliases and the storage key.
// @ts-expect-error The host exposes this small contract as JavaScript.
import * as hostLanguage from '../../../src/helpers/localeNormalize.js'
const { normalizeLocale, SITE_LANGUAGE_KEY, SITE_LANGUAGES } = hostLanguage

// @ts-expect-error Shared host account marker helper is JavaScript.
import { recordExplicitLanguageChoice } from '../../../src/helpers/languageAccount.js'

export const IMAGE_LANGUAGES = SITE_LANGUAGES as ReadonlyArray<
  readonly [string, string]
>
export const normalizeImageLanguage = normalizeLocale as (
  value: unknown,
) => string
export type ImageDictionary = Record<string, string>
type Snapshot = {
  language: string
  pending: string | null
  error: boolean
  storageError: boolean
}
type StorageLike = Pick<Storage, 'getItem' | 'setItem'>
const localeLoaders = import.meta.glob<{ default: ImageDictionary }>(
  './locales/*.json',
)

export function createImageLanguageStore(
  options: {
    storage?: () => StorageLike | undefined
    browserLanguage?: () => string
    load?: (language: string) => Promise<ImageDictionary>
  } = {},
) {
  const storage = options.storage ?? (() => globalThis.window?.localStorage)
  let transient: string | null = null
  const readLanguage = () => {
    if (transient) return transient
    try {
      const saved =
        storage()?.getItem(SITE_LANGUAGE_KEY) ||
        storage()?.getItem('public-language') ||
        storage()?.getItem('i18nextLng')
      return normalizeImageLanguage(
        saved ||
          options.browserLanguage?.() ||
          globalThis.navigator?.language ||
          'zh-CN',
      )
    } catch {
      return normalizeImageLanguage(options.browserLanguage?.() || 'zh-CN')
    }
  }
  let snapshot: Snapshot = {
    language: readLanguage(),
    pending: null,
    error: false,
    storageError: false,
  }
  let dictionary: ImageDictionary = {}
  let revision = 0
  const listeners = new Set<() => void>()
  const cache = new Map<string, ImageDictionary>()
  const publish = (next: Snapshot) => {
    snapshot = next
    listeners.forEach((listener) => listener())
  }
  const load =
    options.load ??
    (async (language) => {
      if (language === 'zh-CN') return {}
      const loader = localeLoaders[`./locales/${language}.json`]
      if (!loader) throw new Error('Missing image locale')
      return (await loader()).default
    })
  const change = async (value: string, persist = true) => {
    const language = normalizeImageLanguage(value)
    const request = ++revision
    publish({ ...snapshot, pending: language, error: false })
    try {
      const next = cache.get(language) ?? (await load(language))
      cache.set(language, next)
      if (request !== revision) return false
      dictionary = next
      let storageError = snapshot.storageError
      if (persist) {
        transient = language
        try {
          const target = storage()
          if (!target) throw new Error('Storage unavailable')
          target.setItem(SITE_LANGUAGE_KEY, language)
          if (target.getItem(SITE_LANGUAGE_KEY) !== language)
            throw new Error('Storage unavailable')
          transient = null
          storageError = false
          if (!recordExplicitLanguageChoice(language, storage))
            storageError = true
        } catch {
          storageError = true
        }
      }
      publish({ language, pending: null, error: false, storageError })
      return true
    } catch {
      if (request === revision)
        publish({ ...snapshot, pending: null, error: true })
      return false
    }
  }
  const refresh = () => {
    if (snapshot.pending) return
    const language = readLanguage()
    if (
      language !== (snapshot.pending || snapshot.language) ||
      !cache.has(language)
    )
      void change(language, false)
  }
  return {
    getSnapshot: () => snapshot,
    subscribe: (listener: () => void) => {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    getDictionary: () => dictionary,
    change,
    refresh,
    initialize: (target: Window) => {
      void change(readLanguage(), false)
      const onStorage = (event: StorageEvent) => {
        if (event.storageArea) {
          try {
            if (event.storageArea !== storage()) return
          } catch {
            return
          }
        }
        if (
          event.key !== null &&
          ![SITE_LANGUAGE_KEY, 'public-language', 'i18nextLng'].includes(
            event.key,
          )
        )
          return
        if (event.key === 'public-language' || event.key === 'i18nextLng') {
          try {
            if (storage()?.getItem(SITE_LANGUAGE_KEY)) return
          } catch {
            return
          }
        }
        transient = null
        const language = readLanguage()
        if (language !== (snapshot.pending || snapshot.language))
          void change(language, false)
      }
      const onVisible = () => {
        if (target.document.visibilityState === 'visible') refresh()
      }
      target.addEventListener('storage', onStorage)
      target.addEventListener('pageshow', refresh)
      target.addEventListener('focus', refresh)
      target.document.addEventListener('visibilitychange', onVisible)
      return () => {
        target.removeEventListener('storage', onStorage)
        target.removeEventListener('pageshow', refresh)
        target.removeEventListener('focus', refresh)
        target.document.removeEventListener('visibilitychange', onVisible)
      }
    },
  }
}

export const imageLanguageStore = createImageLanguageStore()
export const getImageLanguage = () => imageLanguageStore.getSnapshot().language
export const changeImageLanguage = (language: string) =>
  imageLanguageStore.change(language)
export const useImageLanguage = () =>
  useSyncExternalStore(
    imageLanguageStore.subscribe,
    imageLanguageStore.getSnapshot,
    imageLanguageStore.getSnapshot,
  )

export function t(key: string, values: Record<string, unknown> = {}) {
  const translated = imageLanguageStore.getDictionary()[key]
  const text =
    typeof translated === 'string' && translated.trim() ? translated : key
  return text.replace(/\{\{(\w+)\}\}/g, (token, name: string) =>
    Object.prototype.hasOwnProperty.call(values, name)
      ? String(values[name])
      : token,
  )
}

let lastDictionary: ImageDictionary | null = null
let messagePatterns: Array<{
  key: string
  expression: RegExp
  names: string[]
}> = []
// Only local UI messages enter this function. User prompts and model output do not.
// Unknown server messages pass through; captures such as names and errors remain verbatim.
export function translateMessage(message: string): string {
  const dictionary = imageLanguageStore.getDictionary()
  if (Object.prototype.hasOwnProperty.call(dictionary, message))
    return t(message)
  if (dictionary !== lastDictionary) {
    lastDictionary = dictionary
    messagePatterns = Object.keys(dictionary)
      .filter((key) => key.includes('{{'))
      .map((key) => {
        const names: string[] = []
        const parts = key.split(/(\{\{\w+\}\})/g).map((part) => {
          const match = /^\{\{(\w+)\}\}$/.exec(part)
          if (match) {
            names.push(match[1])
            return '([\\s\\S]*?)'
          }
          return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
        })
        return { key, expression: new RegExp(`^${parts.join('')}$`), names }
      })
      .sort((a, b) => b.key.length - a.key.length)
  }
  for (const pattern of messagePatterns) {
    const match = pattern.expression.exec(message)
    if (match)
      return t(
        pattern.key,
        Object.fromEntries(
          pattern.names.map((name, i) => [name, match[i + 1]]),
        ),
      )
  }
  return message.includes('\n')
    ? message.split('\n').map(translateMessage).join('\n')
    : message
}
