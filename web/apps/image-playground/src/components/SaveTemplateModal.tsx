import { t } from '../language'
import { useEffect, useRef, useState } from 'react'

interface SaveTemplateModalProps {
  open: boolean
  defaultName: string
  onCancel: () => void
  onConfirm: (name: string) => void
}

/**
 * SaveTemplateModal 用户输入电商套图模板名称的轻量弹层。
 * - 复用 fixed inset-0 + bg-black/40 蒙层风格
 * - ESC 取消，Enter 提交
 * - 默认值由 caller 派生（通常是 productName + 时间戳）
 */
export default function SaveTemplateModal({ open, defaultName, onCancel, onConfirm }: SaveTemplateModalProps) {
  const [name, setName] = useState(defaultName)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setName(defaultName)
      const t = window.setTimeout(() => inputRef.current?.select(), 30)
      return () => window.clearTimeout(t)
    }
  }, [open, defaultName])

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onCancel()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onCancel])

  if (!open) return null

  const trimmed = name.trim()
  const canConfirm = trimmed.length > 0

  const handleConfirm = () => {
    if (!canConfirm) return
    onConfirm(trimmed)
  }

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/40 px-4">
      <div
        className="w-full max-w-md rounded-2xl bg-white dark:bg-gray-900 p-5 shadow-xl border border-gray-200 dark:border-white/[0.08]"
        role="dialog"
        aria-modal="true"
      >
        <h3 className="text-base font-bold text-gray-950 dark:text-white">{t("保存为模板")}</h3>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {t("将当前 brief 与套图骨架保存为可复用模板（不包含素材与已生成的图片）。")}</p>
        <form
          className="mt-4 space-y-3"
          onSubmit={(event) => {
            event.preventDefault()
            handleConfirm()
          }}
        >
          <label className="block text-xs font-semibold text-gray-500 dark:text-gray-400">
            {t("模板名称")}<input
              ref={inputRef}
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={80}
              autoFocus
              className="mt-1 w-full rounded-xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-950 dark:text-white outline-none focus:ring-2 focus:ring-gray-900/10 dark:focus:ring-white/10"
              placeholder={t("例如：通用美妆主图模板")}
            />
          </label>
          <div className="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-xl border border-gray-200 dark:border-white/[0.08] px-3 py-2 text-sm text-gray-500 hover:text-gray-900 dark:hover:text-white"
            >
              {t("取消")}</button>
            <button
              type="submit"
              disabled={!canConfirm}
              className="rounded-xl bg-gray-900 px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-45 dark:bg-white dark:text-gray-950"
            >
              {t("保存")}</button>
          </div>
        </form>
      </div>
    </div>
  )
}
