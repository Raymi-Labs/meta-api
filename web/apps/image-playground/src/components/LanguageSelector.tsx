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

import {
  changeImageLanguage,
  IMAGE_LANGUAGES,
  t,
  useImageLanguage,
} from '../language'

export default function LanguageSelector() {
  const { language, pending, error, storageError } = useImageLanguage()
  const warning = error
    ? t('语言加载失败，请重试')
    : storageError
      ? t('语言已切换，但浏览器无法保存偏好，刷新后可能恢复原语言。')
      : ''
  return (
    <div className="relative flex items-center">
      <select
        aria-label={t('界面语言')}
        aria-busy={Boolean(pending)}
        title={warning || t('界面语言')}
        value={language}
        onChange={(event) => {
          void changeImageLanguage(event.target.value)
        }}
        className={`max-w-24 sm:max-w-28 rounded-lg border bg-transparent px-1 py-2 text-xs text-gray-600 dark:text-gray-300 ${warning ? 'border-red-400' : 'border-gray-200 dark:border-white/10'}`}
      >
        {IMAGE_LANGUAGES.map(([code, label]) => (
          <option
            key={code}
            value={code}
            lang={code}
            className="bg-white text-gray-800 dark:bg-gray-900 dark:text-gray-200"
          >
            {label}
          </option>
        ))}
      </select>
      {pending && (
        <span role="status" className="sr-only">
          {t('正在加载语言…')}
        </span>
      )}
      {warning && (
        <span
          role="alert"
          className="absolute top-full right-0 mt-1 w-48 rounded bg-white p-2 text-xs text-red-600 shadow dark:bg-gray-900"
        >
          {warning}
          {storageError && !error && (
            <button
              type="button"
              disabled={Boolean(pending)}
              onClick={() => void changeImageLanguage(language)}
              className="ml-2 underline disabled:opacity-50"
            >
              {t('重试')}
            </button>
          )}
        </span>
      )}
    </div>
  )
}
