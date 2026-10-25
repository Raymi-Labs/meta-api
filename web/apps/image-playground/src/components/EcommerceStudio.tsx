import { t, getImageLanguage, translateMessage } from '../language'
import { useEffect, useMemo, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import type { EcommerceBrief, ProductAsset, ProductAssetKind, SuitePlanGroup, SuitePlanItem, TaskRecord } from '../types'
import {
  createInputImageFromFile,
  ensureImageCached,
  ensureImageThumbnailCached,
  subscribeImageThumbnail,
  useStore,
} from '../store'
import {
  ECOMMERCE_GROUPS,
  ECOMMERCE_PLATFORMS,
  INDUSTRY_PRESETS,
  PRODUCT_ASSET_BUCKETS,
  SIZE_PRESETS,
  STYLE_PRESETS,
  SUITE_TEMPLATES,
  getSizePreset,
  getStylePreset,
  getSuiteTemplate,
  isDefaultEcommerceBrief,
} from '../lib/ecommerce'
import { CopyIcon, DownloadIcon, EditIcon, HelpCircleIcon, PhotoIcon, PlusIcon, RefreshIcon, TrashIcon } from './icons'
import SaveTemplateModal from './SaveTemplateModal'
import BucketGuideModal from './BucketGuideModal'

function parseTags(value: string) {
  return value.split(/[，,、\n]/).map((item) => item.trim()).filter(Boolean)
}

function statusLabel(item: SuitePlanItem) {
  if (item.status === 'generating') return t("生成中")
  if (item.status === 'done') return t("已完成")
  if (item.status === 'partial_success') return t("部分成功")
  if (item.status === 'error') return t("失败")
  return t("草稿")
}

function statusClass(item: SuitePlanItem) {
  if (item.status === 'done') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  if (item.status === 'generating') return 'bg-blue-100 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300'
  if (item.status === 'partial_success') return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
  if (item.status === 'error') return 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-white/[0.06] dark:text-gray-300'
}

function ImageThumb({ imageId, className = '' }: { imageId: string; className?: string }) {
  const [src, setSrc] = useState('')

  useEffect(() => {
    let mounted = true
    ensureImageThumbnailCached(imageId).then((thumb) => {
      if (mounted && thumb?.dataUrl) setSrc(thumb.dataUrl)
    })
    const unsubscribe = subscribeImageThumbnail(imageId, (thumb) => setSrc(thumb.dataUrl))
    return () => {
      mounted = false
      unsubscribe()
    }
  }, [imageId])

  if (!src) {
    return (
      <div className={`grid place-items-center bg-gray-100 dark:bg-white/[0.05] text-gray-400 ${className}`}>
        <PhotoIcon className="w-6 h-6" />
      </div>
    )
  }

  return <img src={src} alt="" className={`object-cover ${className}`} draggable={false} />
}

function FieldLabel({ children }: { children: ReactNode }) {
  return <label className="text-xs font-semibold text-gray-500 dark:text-gray-400">{children}</label>
}

function TextInput(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`w-full rounded-xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-gray-900/10 dark:focus:ring-white/10 ${props.className ?? ''}`}
    />
  )
}

function TextArea(props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={`w-full rounded-xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-gray-900/10 dark:focus:ring-white/10 ${props.className ?? ''}`}
    />
  )
}

function SelectInput(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      {...props}
      className={`w-full rounded-xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-gray-900/10 dark:focus:ring-white/10 ${props.className ?? ''}`}
    />
  )
}

function SwitchRow({ label, checked, onChange, hint }: { label: string; checked: boolean; onChange: (value: boolean) => void; hint?: string }) {
  return (
    <label className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 dark:border-white/[0.08] bg-white/70 dark:bg-white/[0.03] px-3 py-2">
      <span>
        <span className="block text-sm font-medium text-gray-800 dark:text-gray-100">{label}</span>
        {hint && <span className="block text-xs text-gray-500 mt-0.5">{hint}</span>}
      </span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="h-4 w-4 accent-gray-900 dark:accent-white"
      />
    </label>
  )
}

function BucketCard({
  kind,
  assets,
  onUpload,
  onRemove,
  onShowGuide,
}: {
  kind: ProductAssetKind
  assets: ProductAsset[]
  onUpload: (kind: ProductAssetKind, files: FileList | null) => void
  onRemove: (assetId: string) => void
  onShowGuide: (kind: ProductAssetKind) => void
}) {
  const bucket = PRODUCT_ASSET_BUCKETS.find((item) => item.kind === kind)!
  const inputRef = useRef<HTMLInputElement>(null)
  const reachedLimit = assets.length >= bucket.max

  return (
    <section className="rounded-2xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 p-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-gray-900 dark:text-white">
            {t(bucket.label)}{bucket.required && <span className="text-red-500 ml-1">*</span>}
          </div>
          <div className="text-xs text-gray-500 mt-1">{t(bucket.hint)}</div>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <button
            type="button"
            onClick={() => onShowGuide(kind)}
            title={t("查看拍摄 / 选图指引")}
            className="inline-flex items-center gap-1 rounded-lg border border-gray-200 dark:border-white/[0.08] px-2 py-1 text-xs font-medium text-gray-500 hover:text-gray-900 dark:hover:text-white"
          >
            <HelpCircleIcon className="w-3.5 h-3.5" />
            {t("指引")}</button>
          <button
            type="button"
            onClick={() => inputRef.current?.click()}
            disabled={reachedLimit}
            className="inline-flex items-center gap-1 rounded-lg bg-gray-900 text-white dark:bg-white dark:text-gray-950 px-2 py-1 text-xs font-medium disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <PlusIcon className="w-3.5 h-3.5" />
            {t("上传")}</button>
        </div>
      </div>
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple={kind !== 'main'}
        className="hidden"
        onChange={(event) => {
          onUpload(kind, event.currentTarget.files)
          event.currentTarget.value = ''
        }}
      />
      <div className="mt-3 grid grid-cols-3 gap-2">
        {assets.map((asset) => (
          <div key={asset.id} className="relative group overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.08]">
            <ImageThumb imageId={asset.imageId} className="aspect-square w-full" />
            <button
              type="button"
              onClick={() => onRemove(asset.id)}
              className="absolute right-1 top-1 rounded-md bg-black/55 p-1 text-white opacity-0 transition-opacity group-hover:opacity-100"
              aria-label={t("删除素材")}
            >
              <TrashIcon className="w-3.5 h-3.5" />
            </button>
          </div>
        ))}
        {assets.length === 0 && (
          <div className="col-span-3 rounded-xl border border-dashed border-gray-200 dark:border-white/[0.08] py-5 text-center text-xs text-gray-400">
            {t("未上传")}</div>
        )}
      </div>
      <div className="mt-2 text-[11px] text-gray-400">{assets.length}/{bucket.max}</div>
    </section>
  )
}

function PlanCard({
  item,
  tasks,
  selected,
  onSelect,
  onPatch,
  onRetry,
  onCopyPrompt,
  onSetStyle,
  onDownload,
  onEdit,
  onOpenTask,
}: {
  item: SuitePlanItem
  tasks: TaskRecord[]
  selected: boolean
  onSelect: () => void
  onPatch: (patch: Partial<SuitePlanItem>) => void
  onRetry: () => void
  onCopyPrompt: () => void
  onSetStyle: (imageId: string) => void
  onDownload: (imageId: string) => void
  onEdit: (imageId: string) => void
  onOpenTask: (taskId: string) => void
}) {
  const outputImageIds = tasks.flatMap((task) => task.outputImages)

  return (
    <article className={`rounded-2xl border bg-white dark:bg-gray-950 p-4 transition-colors ${selected ? 'border-gray-900 dark:border-white' : 'border-gray-200 dark:border-white/[0.08]'}`}>
      <div className="flex items-start justify-between gap-3">
        <button type="button" onClick={onSelect} className="min-w-0 text-left">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold text-gray-900 dark:text-white">{item.title}</span>
            <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${statusClass(item)}`}>{statusLabel(item)}</span>
          </div>
          <p className="mt-1 text-xs text-gray-500 line-clamp-2">{item.purpose}</p>
        </button>
        <label className="inline-flex items-center gap-1 text-xs text-gray-500">
          <input type="checkbox" checked={item.enabled} onChange={(event) => onPatch({ enabled: event.target.checked })} />
          {t("启用")}</label>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-gray-500">
        <span className="rounded-full bg-gray-100 dark:bg-white/[0.06] px-2 py-1">{t("比例")} {item.ratio}</span>
        <label className="inline-flex items-center gap-1 rounded-full bg-gray-100 dark:bg-white/[0.06] px-2 py-1">
          {t("数量")}<input
            type="number"
            min={1}
            max={6}
            value={item.count}
            onChange={(event) => onPatch({ count: Math.max(1, Math.min(6, Number(event.target.value) || 1)) })}
            className="w-10 bg-transparent text-center outline-none"
          />
        </label>
      </div>
      {item.error && <div className="mt-3 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-300">{translateMessage(item.error)}</div>}
      {outputImageIds.length > 0 ? (
        <div className="mt-3 grid grid-cols-2 sm:grid-cols-3 gap-2">
          {outputImageIds.map((imageId) => (
            <div key={imageId} className="group relative overflow-hidden rounded-xl border border-gray-200 dark:border-white/[0.08]">
              <button type="button" onClick={() => onOpenTask(tasks.find((task) => task.outputImages.includes(imageId))?.id || '')} className="block w-full">
                <ImageThumb imageId={imageId} className="aspect-square w-full" />
              </button>
              <div className="absolute inset-x-1 bottom-1 flex justify-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                <IconButton label={t("风格参考")} onClick={() => onSetStyle(imageId)}><PhotoIcon className="w-3.5 h-3.5" /></IconButton>
                <IconButton label={t("下载")} onClick={() => onDownload(imageId)}><DownloadIcon className="w-3.5 h-3.5" /></IconButton>
                <IconButton label={t("遮罩编辑")} onClick={() => onEdit(imageId)}><EditIcon className="w-3.5 h-3.5" /></IconButton>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-3 rounded-xl border border-dashed border-gray-200 dark:border-white/[0.08] px-3 py-5 text-center text-xs text-gray-400">
          {t("生成后图片会显示在这里")}</div>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        <SmallButton onClick={onRetry} disabled={!item.enabled || item.status === 'generating'} icon={<RefreshIcon className="w-3.5 h-3.5" />}>{t("同类型重生成")}</SmallButton>
        <SmallButton onClick={onCopyPrompt} icon={<CopyIcon className="w-3.5 h-3.5" />}>{t("复制提示词")}</SmallButton>
        <SmallButton disabled title={t("即将支持")}>{t("设为主图")}</SmallButton>
        <SmallButton disabled title={t("即将支持")}>{t("替换文案")}</SmallButton>
      </div>
      {selected && (
        <details className="mt-3 rounded-xl bg-gray-50 dark:bg-white/[0.04] p-3 text-xs text-gray-600 dark:text-gray-300">
          <summary className="cursor-pointer font-medium">{t("查看最终 prompt 草案")}</summary>
          <pre className="mt-2 whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed">{item.prompt || item.finalPromptDraft}</pre>
        </details>
      )}
    </article>
  )
}

function IconButton({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      title={label}
      onClick={(event) => {
        event.stopPropagation()
        onClick()
      }}
      className="rounded-lg bg-black/65 p-1.5 text-white backdrop-blur hover:bg-black/80"
    >
      {children}
    </button>
  )
}

function SmallButton({ children, icon, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { icon?: ReactNode }) {
  return (
    <button
      type="button"
      {...props}
      className={`inline-flex items-center gap-1 rounded-lg border border-gray-200 dark:border-white/[0.08] px-2.5 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/[0.04] disabled:cursor-not-allowed disabled:opacity-40 ${props.className ?? ''}`}
    >
      {icon}
      {children}
    </button>
  )
}

export default function EcommerceStudio() {
  const suites = useStore((s) => s.ecommerceSuites)
  const activeSuiteId = useStore((s) => s.activeEcommerceSuiteId)
  const selectedPlanItemId = useStore((s) => s.selectedEcommercePlanItemId)
  const planStatus = useStore((s) => s.ecommercePlanDraftStatus)
  const generationQueue = useStore((s) => s.generationQueue)
  const tasks = useStore((s) => s.tasks)
  const createSuite = useStore((s) => s.createEcommerceSuite)
  const updateBrief = useStore((s) => s.updateEcommerceBrief)
  const addAsset = useStore((s) => s.addEcommerceAsset)
  const removeAsset = useStore((s) => s.removeEcommerceAsset)
  const generatePlan = useStore((s) => s.generateEcommercePlan)
  const generateSuite = useStore((s) => s.generateEcommerceSuite)
  const retryPlanItem = useStore((s) => s.retryEcommercePlanItem)
  const updatePlanItem = useStore((s) => s.updateEcommercePlanItem)
  const setSelectedPlanItemId = useStore((s) => s.setSelectedEcommercePlanItemId)
  const addStyleReference = useStore((s) => s.addEcommerceStyleReference)
  const setMaskEditorImageId = useStore((s) => s.setMaskEditorImageId)
  const setDetailTaskId = useStore((s) => s.setDetailTaskId)
  const showToast = useStore((s) => s.showToast)
  const capabilities = useStore((s) => s.ecommerceCapabilities)
  const userTemplates = useStore((s) => s.userSuiteTemplates)
  const applyIndustryPreset = useStore((s) => s.applyIndustryPreset)
  const saveCurrentSuiteAsTemplate = useStore((s) => s.saveCurrentSuiteAsTemplate)
  const applyUserTemplate = useStore((s) => s.applyUserTemplate)
  const deleteUserTemplate = useStore((s) => s.deleteUserTemplate)
  const renameUserTemplate = useStore((s) => s.renameUserTemplate)
  const [saveTemplateModalOpen, setSaveTemplateModalOpen] = useState(false)
  const [guideKind, setGuideKind] = useState<ProductAssetKind | null>(null)
  const suite = suites.find((item) => item.id === activeSuiteId)

  useEffect(() => {
    if (!suite) createSuite()
  }, [suite, createSuite])

  const tasksByItem = useMemo(() => {
    const map = new Map<string, TaskRecord[]>()
    for (const task of tasks) {
      if (task.sourceMode !== 'ecommerce' || !task.suiteItemId || task.suiteId !== suite?.id) continue
      const list = map.get(task.suiteItemId) ?? []
      list.push(task)
      map.set(task.suiteItemId, list)
    }
    return map
  }, [tasks, suite?.id])

  if (!suite) {
    return <main className="safe-area-x max-w-7xl mx-auto py-10 text-sm text-gray-500">{t("正在创建电商套图工程...")}</main>
  }

  const brief = suite.brief
  const style = getStylePreset(brief.stylePresetId)
  const template = getSuiteTemplate(brief.suiteTemplateId)
  const sizePreset = getSizePreset(brief.sizePreset)
  const isGenerating = generationQueue.length > 0

  const sortedUserTemplates = useMemo(
    () => [...userTemplates].sort((a, b) => b.updatedAt - a.updatedAt),
    [userTemplates],
  )

  const briefIsDefault = isDefaultEcommerceBrief(brief)

  // 确认覆盖：当前 brief 已有非默认内容时弹窗二次确认，避免误覆盖用户填写。
  // 接受一个 reason 文案区分"行业起步包"和"用户模板"两种触发源。
  const confirmOverwriteIfNeeded = (reason: string): boolean => {
    if (briefIsDefault) return true
    return window.confirm(t("{{value0}}将完全覆盖当前 brief（不影响已上传素材）。继续？", { value0: (reason) }))
  }

  const handleSelectIndustryPreset = (presetId: string) => {
    if (!presetId) return
    if (!confirmOverwriteIfNeeded(t("选择行业起步包"))) return
    if (!applyIndustryPreset(presetId)) {
      showToast('未找到该行业起步包', 'error')
    }
  }

  const handleApplyUserTemplate = (templateId: string, templateName: string) => {
    if (!confirmOverwriteIfNeeded(t("套用模板「{{value0}}」", { value0: (templateName) }))) return
    if (!applyUserTemplate(templateId)) {
      showToast('模板已不存在', 'error')
    }
  }

  const handleSaveTemplate = (name: string) => {
    saveCurrentSuiteAsTemplate(name)
    setSaveTemplateModalOpen(false)
  }

  const handleRenameUserTemplate = (templateId: string, currentName: string) => {
    const next = window.prompt(t("重命名模板"), currentName)
    if (next === null) return
    const trimmed = next.trim()
    if (!trimmed) {
      showToast('模板名称不能为空', 'error')
      return
    }
    renameUserTemplate(templateId, trimmed)
  }

  const handleDeleteUserTemplate = (templateId: string, name: string) => {
    if (!window.confirm(t("确定删除模板「{{value0}}」？", { value0: (name) }))) return
    deleteUserTemplate(templateId)
  }

  const defaultTemplateName = `${brief.productName.trim() || '未命名套图'} · ${new Date().toLocaleString(getImageLanguage(), { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })}`

  const uploadAssets = async (kind: ProductAssetKind, files: FileList | null) => {
    if (!files?.length) return
    for (const file of Array.from(files)) {
      const image = await createInputImageFromFile(file)
      if (image) addAsset(kind, image.id, file.name)
    }
  }

  const copyPrompt = async (item: SuitePlanItem) => {
    await navigator.clipboard.writeText(item.prompt || item.finalPromptDraft)
    showToast('已复制提示词', 'success')
  }

  const downloadImage = async (imageId: string) => {
    const dataUrl = await ensureImageCached(imageId)
    if (!dataUrl) {
      showToast('图片已丢失', 'error')
      return
    }
    const a = document.createElement('a')
    a.href = dataUrl
    a.download = `ecommerce-${imageId.slice(0, 8)}.png`
    a.click()
  }

  const setStyle = (imageId: string) => {
    addStyleReference(imageId, '生成结果')
    showToast('已加入风格参考图', 'success')
  }

  return (
    <main className="safe-area-x max-w-[1600px] mx-auto pb-10">
      <div className="mb-4 rounded-3xl border border-gray-200 dark:border-white/[0.08] bg-gradient-to-r from-amber-50 via-white to-sky-50 dark:from-amber-500/10 dark:via-gray-950 dark:to-sky-500/10 p-5">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.24em] text-gray-500">Ecommerce Suite Studio</p>
            <h2 className="mt-1 text-2xl font-black tracking-tight text-gray-950 dark:text-white">{t("电商套图工作台")}</h2>
            <p className="mt-1 text-sm text-gray-600 dark:text-gray-300">{t("商品素材 → 套图方案 → 批量生成 → 回流画廊")}</p>
          </div>
          <div className="flex flex-wrap gap-2 text-xs text-gray-600 dark:text-gray-300">
            <span className="rounded-full bg-white/80 dark:bg-white/[0.06] px-3 py-1.5">{t("规划模型")} {capabilities.defaultPlanModel}</span>
            <span className="rounded-full bg-white/80 dark:bg-white/[0.06] px-3 py-1.5">{t("出图模型")} {capabilities.defaultImageModel}</span>
            <span className="rounded-full bg-white/80 dark:bg-white/[0.06] px-3 py-1.5">{t("并发 2")}</span>
          </div>
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[300px_minmax(0,1fr)_340px]">
        <aside className="space-y-3">
          {PRODUCT_ASSET_BUCKETS.map((bucket) => (
            <BucketCard
              key={bucket.kind}
              kind={bucket.kind}
              assets={suite.assets.filter((asset) => asset.kind === bucket.kind)}
              onUpload={uploadAssets}
              onRemove={removeAsset}
              onShowGuide={setGuideKind}
            />
          ))}
          <section className="space-y-2 rounded-2xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 p-3">
            <SwitchRow label={t("锁定商品主体")} checked={brief.lockProduct} onChange={(lockProduct) => updateBrief({ lockProduct })} />
            <SwitchRow label={t("锁定品牌风格")} checked={brief.lockBrandStyle} onChange={(lockBrandStyle) => updateBrief({ lockBrandStyle })} />
            <SwitchRow label={t("允许轻微美化")} checked={brief.allowBeautify} onChange={(allowBeautify) => updateBrief({ allowBeautify })} />
            <SwitchRow label={t("严格结构一致")} checked={brief.strictStructure} onChange={(strictStructure) => updateBrief({ strictStructure })} />
          </section>
        </aside>

        <section className="min-w-0 space-y-4">
          {ECOMMERCE_GROUPS.map((group) => {
            const items = suite.plan.filter((item) => item.group === group.id)
            return (
              <section key={group.id} className="rounded-3xl border border-gray-200 dark:border-white/[0.08] bg-gray-50/80 dark:bg-white/[0.02] p-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <div>
                    <h3 className="font-bold text-gray-950 dark:text-white">{t(group.label)}</h3>
                    <p className="text-xs text-gray-500">{t(group.description)}</p>
                  </div>
                  <span className="rounded-full bg-white dark:bg-gray-950 px-2.5 py-1 text-xs text-gray-500">{t('共 {{count}} 项', { count: items.length })}</span>
                </div>
                {items.length > 0 ? (
                  <div className="grid gap-3 2xl:grid-cols-2">
                    {items.map((item) => (
                      <PlanCard
                        key={item.id}
                        item={item}
                        tasks={tasksByItem.get(item.id) ?? []}
                        selected={selectedPlanItemId === item.id}
                        onSelect={() => setSelectedPlanItemId(item.id)}
                        onPatch={(patch) => updatePlanItem(item.id, patch)}
                        onRetry={() => void retryPlanItem(item.id)}
                        onCopyPrompt={() => void copyPrompt(item)}
                        onSetStyle={setStyle}
                        onDownload={(imageId) => void downloadImage(imageId)}
                        onEdit={setMaskEditorImageId}
                        onOpenTask={(taskId) => taskId && setDetailTaskId(taskId)}
                      />
                    ))}
                  </div>
                ) : (
                  <div className="rounded-2xl border border-dashed border-gray-200 dark:border-white/[0.08] bg-white/60 dark:bg-gray-950/50 px-4 py-10 text-center text-sm text-gray-500">
                    {t("生成套图方案后，这里会按用途展示计划和结果。")}</div>
                )}
              </section>
            )
          })}
        </section>

        <aside className="space-y-3">
          <section className="rounded-2xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 p-4">
            <div className="flex items-center justify-between gap-3">
              <h3 className="font-bold text-gray-950 dark:text-white">{t("快速起步")}</h3>
              <span className="text-xs text-gray-400">{t("一键填充配置")}</span>
            </div>
            <div className="mt-3 space-y-3">
              <div>
                <FieldLabel>{t("行业起步包")}</FieldLabel>
                <SelectInput
                  value=""
                  onChange={(event) => {
                    handleSelectIndustryPreset(event.target.value)
                    event.target.value = ''
                  }}
                >
                  <option value="">{t("选择一个行业…")}</option>
                  {INDUSTRY_PRESETS.map((preset) => (
                    <option key={preset.id} value={preset.id}>{t(preset.name)}</option>
                  ))}
                </SelectInput>
                <p className="mt-1 text-xs text-gray-500">{t("选中后会完全覆盖当前 brief，但不会动已上传的素材。")}</p>
              </div>
              <div>
                <div className="flex items-center justify-between">
                  <FieldLabel>{t('我的模板（{{count}}）', { count: sortedUserTemplates.length })}</FieldLabel>
                  <button
                    type="button"
                    onClick={() => setSaveTemplateModalOpen(true)}
                    className="text-xs font-medium text-gray-500 hover:text-gray-900 dark:hover:text-white"
                  >
                    {t("保存当前为模板")}</button>
                </div>
                {sortedUserTemplates.length === 0 ? (
                  <p className="mt-2 text-xs text-gray-400">{t("还没有保存的模板。配置好 brief 与套图骨架后，点上方按钮保存。")}</p>
                ) : (
                  <ul className="mt-2 space-y-1.5 max-h-44 overflow-y-auto pr-1">
                    {sortedUserTemplates.map((tpl) => (
                      <li
                        key={tpl.id}
                        className="group flex items-center justify-between gap-2 rounded-xl border border-gray-200 dark:border-white/[0.08] px-2.5 py-1.5"
                      >
                        <button
                          type="button"
                          onClick={() => handleApplyUserTemplate(tpl.id, tpl.name)}
                          className="flex-1 min-w-0 text-left text-xs text-gray-700 dark:text-gray-200 hover:text-gray-950 dark:hover:text-white"
                          title={t("套用模板：{{value0}}", { value0: (tpl.name) })}
                        >
                          <span className="truncate block">{tpl.name}</span>
                          <span className="block text-[10px] text-gray-400 mt-0.5">
                            {new Date(tpl.updatedAt).toLocaleString(getImageLanguage(), { hour12: false })}  · {t('套图骨架：{{count}} 项', { count: tpl.plan.length })}</span>
                        </button>
                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                          <button
                            type="button"
                            onClick={() => handleRenameUserTemplate(tpl.id, tpl.name)}
                            className="rounded-md p-1 text-gray-400 hover:text-gray-900 dark:hover:text-white"
                            title={t("重命名")}
                          >
                            <EditIcon className="w-3.5 h-3.5" />
                          </button>
                          <button
                            type="button"
                            onClick={() => handleDeleteUserTemplate(tpl.id, tpl.name)}
                            className="rounded-md p-1 text-gray-400 hover:text-red-500"
                            title={t("删除")}
                          >
                            <TrashIcon className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          </section>

          <section className="rounded-2xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 p-4">
            <div className="flex items-center justify-between gap-3">
              <h3 className="font-bold text-gray-950 dark:text-white">{t("套图配置")}</h3>
              <button type="button" onClick={createSuite} className="text-xs font-medium text-gray-500 hover:text-gray-900 dark:hover:text-white">{t("新建")}</button>
            </div>
            <div className="mt-4 space-y-3">
              <div>
                <FieldLabel>{t("商品名称 *")}</FieldLabel>
                <TextInput value={brief.productName} onChange={(event) => updateBrief({ productName: event.target.value })} placeholder={t("例如：便携榨汁杯")} />
              </div>
              <div>
                <FieldLabel>{t("商品类目 *")}</FieldLabel>
                <TextInput value={brief.category} onChange={(event) => updateBrief({ category: event.target.value })} placeholder={t("例如：小家电 / 数码 / 美妆")} />
              </div>
              <div>
                <FieldLabel>{t("核心卖点 *")}</FieldLabel>
                <TextInput value={brief.sellingPoints.join('、')} onChange={(event) => updateBrief({ sellingPoints: parseTags(event.target.value) })} placeholder={t("便携、低噪音、易清洗")} />
              </div>
              <div>
                <FieldLabel>{t("目标人群")}</FieldLabel>
                <TextInput value={brief.targetAudience} onChange={(event) => updateBrief({ targetAudience: event.target.value })} placeholder={t("例如：通勤白领、宝妈、户外人群")} />
              </div>
              <div>
                <FieldLabel>{t("目标平台")}</FieldLabel>
                <div className="mt-1 flex flex-wrap gap-2">
                  {ECOMMERCE_PLATFORMS.map((platform) => {
                    const checked = brief.targetPlatforms.includes(platform)
                    return (
                      <button
                        key={platform}
                        type="button"
                        onClick={() => updateBrief({
                          targetPlatforms: checked
                            ? brief.targetPlatforms.filter((item) => item !== platform)
                            : [...brief.targetPlatforms, platform],
                        })}
                        className={`rounded-full border px-2.5 py-1 text-xs ${checked ? 'border-gray-900 bg-gray-900 text-white dark:border-white dark:bg-white dark:text-gray-950' : 'border-gray-200 dark:border-white/[0.08] text-gray-500'}`}
                      >
                        {t(platform)}
                      </button>
                    )
                  })}
                </div>
              </div>
              <div>
                <FieldLabel>{t("风格模板")}</FieldLabel>
                <SelectInput value={brief.stylePresetId} onChange={(event) => updateBrief({ stylePresetId: event.target.value })}>
                  {STYLE_PRESETS.map((item) => <option key={item.id} value={item.id}>{t(item.name)}</option>)}
                </SelectInput>
                <p className="mt-1 text-xs text-gray-500">{t(style.description)}</p>
              </div>
              <div>
                <FieldLabel>{t("套图模板")}</FieldLabel>
                <SelectInput value={brief.suiteTemplateId} onChange={(event) => updateBrief({ suiteTemplateId: event.target.value })}>
                  {SUITE_TEMPLATES.map((item) => <option key={item.id} value={item.id}>{t(item.name)}</option>)}
                </SelectInput>
                <p className="mt-1 text-xs text-gray-500">{t(template.description)}</p>
              </div>
              <div>
                <FieldLabel>{t("尺寸预设")}</FieldLabel>
                <SelectInput value={brief.sizePreset} onChange={(event) => updateBrief({ sizePreset: event.target.value })}>
                  {SIZE_PRESETS.map((item) => <option key={item.id} value={item.id}>{t(item.label)}</option>)}
                </SelectInput>
                <p className="mt-1 text-xs text-gray-500">{sizePreset.ratio} · {sizePreset.size}</p>
              </div>
              <div>
                <FieldLabel>{t("每组生成项数量")}</FieldLabel>
                <div className="mt-1 grid grid-cols-2 gap-2">
                  {ECOMMERCE_GROUPS.map((group) => (
                    <label key={group.id} className="rounded-xl border border-gray-200 dark:border-white/[0.08] px-3 py-2 text-xs text-gray-500">
                      {t(group.label)}
                      <input
                        type="number"
                        min={0}
                        max={12}
                        value={brief.counts[group.id]}
                        onChange={(event) => updateBrief({ counts: { ...brief.counts, [group.id]: Math.max(0, Math.min(12, Number(event.target.value) || 0)) } as EcommerceBrief['counts'] })}
                        className="mt-1 w-full bg-transparent text-sm font-semibold text-gray-900 dark:text-white outline-none"
                      />
                    </label>
                  ))}
                </div>
              </div>
              <div>
                <FieldLabel>{t("禁忌内容")}</FieldLabel>
                <TextArea rows={2} value={brief.negativePrompt ?? ''} onChange={(event) => updateBrief({ negativePrompt: event.target.value })} placeholder={t("不要生成竞品品牌，不夸张功效...")} />
              </div>
            </div>
          </section>

          <section className="rounded-2xl border border-gray-200 dark:border-white/[0.08] bg-white dark:bg-gray-950 p-4">
            <FieldLabel>{t("补充宣传需求")}</FieldLabel>
            <TextArea rows={4} value={brief.extraPrompt} onChange={(event) => updateBrief({ extraPrompt: event.target.value })} placeholder={t("例如：更适合小红书封面、减少文字、突出夏季清爽感...")} />
            <div className="mt-3 grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => void generatePlan()}
                disabled={planStatus === 'planning' || isGenerating}
                className="rounded-xl bg-gray-900 px-4 py-2.5 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-45 dark:bg-white dark:text-gray-950"
              >
                {planStatus === 'planning' ? t("规划中...") : t("生成套图方案")}
              </button>
              <button
                type="button"
                onClick={() => void generateSuite()}
                disabled={suite.plan.length === 0 || isGenerating}
                className="rounded-xl bg-emerald-600 px-4 py-2.5 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-45"
              >
                {isGenerating ? t("生成中 {{value0}}", { value0: (generationQueue.length) }) : t("开始生成")}
              </button>
            </div>
            <div className="mt-2 grid grid-cols-2 gap-2">
              <button type="button" disabled className="rounded-xl border border-gray-200 dark:border-white/[0.08] px-3 py-2 text-xs text-gray-400 disabled:cursor-not-allowed" title={t("即将支持")}>{t("全部下载 ZIP")}</button>
              <button
                type="button"
                onClick={() => setSaveTemplateModalOpen(true)}
                className="rounded-xl border border-gray-200 dark:border-white/[0.08] px-3 py-2 text-xs text-gray-600 dark:text-gray-300 hover:border-gray-900 hover:text-gray-900 dark:hover:border-white dark:hover:text-white"
              >
                {t("保存为模板")}</button>
            </div>
          </section>
        </aside>
      </div>
      <SaveTemplateModal
        open={saveTemplateModalOpen}
        defaultName={defaultTemplateName}
        onCancel={() => setSaveTemplateModalOpen(false)}
        onConfirm={handleSaveTemplate}
      />
      <BucketGuideModal kind={guideKind} onClose={() => setGuideKind(null)} />
    </main>
  )
}
