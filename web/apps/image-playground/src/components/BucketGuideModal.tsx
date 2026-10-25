import { t } from '../language'
import { useEffect } from 'react'
import type { ProductAssetKind } from '../types'
import { getBucketGuide } from '../lib/bucketGuides'

interface BucketGuideModalProps {
  /** 当前要展示指引的槽位类型；为 null 时不渲染 */
  kind: ProductAssetKind | null
  onClose: () => void
}

/**
 * BucketGuideModal — 素材槽位的拍摄/选图教学弹层。
 *
 * - 复用 SaveTemplateModal 的 fixed + bg-black/40 蒙层 + rounded-2xl 主卡风格
 * - 按 kind 从 BUCKET_GUIDES 取数据：非 style 渲染 tip 卡列表，style 渲染渐变意境卡
 * - ESC 关闭，点击蒙层关闭，「知道了」按钮关闭
 * - 暗色模式遵循项目惯例（dark:bg-gray-900 / dark:text-white 等）
 *
 * Phase 2：style 卡的「使用此风格示范」按钮启用后，会通过 store 把对应真实样图
 *          加入 brief.assets[kind='style']，从而作为 gpt-image-2 的视觉参考。
 *          本期按钮 disabled，文案提示"待真实样图准备后启用"。
 */
export default function BucketGuideModal({ kind, onClose }: BucketGuideModalProps) {
  useEffect(() => {
    if (!kind) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [kind, onClose])

  if (!kind) return null

  const guide = getBucketGuide(kind)
  const isStyle = kind === 'style'

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/40 px-4"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="w-full max-w-2xl max-h-[85vh] overflow-y-auto rounded-2xl bg-white dark:bg-gray-900 p-5 shadow-xl border border-gray-200 dark:border-white/[0.08]"
        role="dialog"
        aria-modal="true"
        onClick={(event) => event.stopPropagation()}
      >
        <h3 className="text-base font-bold text-gray-950 dark:text-white">{t(guide.title)}</h3>
        <p className="mt-1.5 text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
          {t(guide.intro)}
        </p>

        {!isStyle && guide.tips && (
          <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
            {guide.tips.map((tip) => (
              <div
                key={tip.title}
                className="rounded-xl border border-gray-200 dark:border-white/[0.08] bg-gray-50 dark:bg-gray-950 px-3 py-3"
              >
                <div className="flex items-start gap-2">
                  <span className="text-xl leading-none" aria-hidden="true">{tip.emoji}</span>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-semibold text-gray-900 dark:text-white">
                      {t(tip.title)}
                    </div>
                    <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
                      {t(tip.description)}
                    </p>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {isStyle && guide.styleSamples && (
          <>
            <div className="mt-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
              {guide.styleSamples.map((sample) => (
                <div
                  key={sample.presetId}
                  className="rounded-xl border border-gray-200 dark:border-white/[0.08] overflow-hidden"
                >
                  <div className={`h-24 w-full ${sample.gradient}`} aria-hidden="true" />
                  <div className="px-3 py-2.5">
                    <div className="text-sm font-semibold text-gray-900 dark:text-white">
                      {t(sample.name)}
                    </div>
                    <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
                      {t(sample.description)}
                    </p>
                    <button
                      type="button"
                      disabled
                      title={t("待真实样图准备后启用")}
                      className="mt-2 inline-flex items-center gap-1 rounded-md border border-gray-200 dark:border-white/[0.08] px-2 py-1 text-[11px] text-gray-400 disabled:cursor-not-allowed"
                    >
                      {t("使用此风格示范（即将支持）")}</button>
                  </div>
                </div>
              ))}
            </div>
            <p className="mt-3 text-[11px] text-gray-400 dark:text-gray-500 leading-relaxed">
              {t("当前为风格意境示意，方便你理解每种风格的视觉差异；右侧「风格模板」下拉同样按这 7 个预设命名。 如果你想用具体图作为参考，请上传真实参考图到本槽位。")}</p>
          </>
        )}

        <div className="mt-5 flex justify-end">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl bg-gray-900 px-4 py-2 text-sm font-semibold text-white dark:bg-white dark:text-gray-950"
          >
            {t("知道了")}</button>
        </div>
      </div>
    </div>
  )
}
