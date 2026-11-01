import type {
  ApiProfile,
  AppSettings,
  EcommerceBrief,
  EcommerceCapabilities,
  EcommerceSuite,
  IndustryPreset,
  ProductAsset,
  ProductAssetKind,
  ResponsesApiResponse,
  SavedSuiteTemplate,
  SuitePlanGroup,
  SuitePlanItem,
} from '../types'
import { buildApiUrl, readClientDevProxyConfig, shouldUseApiProxy } from './devProxy'
import { getApiErrorMessage } from './imageApiShared'

export const DEFAULT_ECOMMERCE_CAPABILITIES: EcommerceCapabilities = {
  defaultImageModel: 'gpt-image-2',
  defaultPlanModel: 'gpt-6-sol',
  supportsEcommerce: true,
}

export const ECOMMERCE_GROUPS: Array<{ id: SuitePlanGroup; label: string; description: string }> = [
  { id: 'main', label: '主图组', description: '搜索页、列表页和商品首图' },
  { id: 'detail', label: '详情图组', description: '卖点、材质、参数、步骤说明' },
  { id: 'promotion', label: '宣传图组', description: '活动海报、社媒封面和投放素材' },
  { id: 'sku', label: '规格图组', description: '颜色、容量、组合装和规格对比' },
]

export const PRODUCT_ASSET_BUCKETS: Array<{
  kind: ProductAssetKind
  label: string
  required?: boolean
  max: number
  hint: string
}> = [
  { kind: 'main', label: '主商品图', required: true, max: 1, hint: '所有套图的主体参考，建议上传正面清晰图' },
  { kind: 'angle', label: '多角度图', max: 8, hint: '补充侧面、背面、细节、包装等结构信息' },
  { kind: 'sku', label: '规格/SKU 图', max: 12, hint: '不同颜色、容量、型号、组合装参考' },
  { kind: 'brand', label: '品牌素材', max: 6, hint: 'Logo、包装、品牌色和视觉资产' },
  { kind: 'style', label: '风格参考图', max: 8, hint: '用于约束光线、构图、质感与场景语言' },
]

export const ECOMMERCE_PLATFORMS = ['淘宝/天猫', '京东', '拼多多', '抖音', '小红书', '亚马逊', '自定义']

export const STYLE_PRESETS = [
  { id: 'minimal-white', name: '极简白底电商风', description: '纯白/浅灰背景，商品居中，干净阴影，适合通用主图' },
  { id: 'tech-premium', name: '高级科技感', description: '冷色调、金属质感、科技光效，适合数码家电' },
  { id: 'xiaohongshu-life', name: '小红书生活方式', description: '柔光、真实生活场景、低饱和、种草感' },
  { id: 'livestream-hit', name: '直播间爆款风', description: '强卖点、强对比、促销氛围，适合直播与活动' },
  { id: 'outdoor-scene', name: '户外场景风', description: '自然光、户外背景、动感构图，适合运动露营车载' },
  { id: 'luxury-texture', name: '奢华质感风', description: '深色背景、精致高光、礼盒/首饰/香水质感' },
  { id: 'japanese-fresh', name: '日系清新风', description: '浅色、留白、清爽、低饱和，适合家居文具小物' },
]

export const SUITE_TEMPLATES = [
  { id: 'basic-suite', name: '基础电商套图', description: '主图、详情、宣传、规格的完整组合' },
  { id: 'marketplace-main', name: '淘宝/京东主图套装', description: '白底主图、场景主图、卖点主图、规格主图' },
  { id: 'xiaohongshu-seeding', name: '小红书种草套图', description: '封面、生活场景、卖点卡片、步骤、竖版海报' },
  { id: 'detail-long-page', name: '详情页长图素材', description: '卖点、材质、参数、步骤、包装清单' },
  { id: 'sku-suite', name: 'SKU 规格图', description: '单 SKU、多色可选、规格对比、全 SKU 合影' },
  { id: 'promotion-suite', name: '促销活动套图', description: '新品上市、折扣、618/双11、直播间背景' },
]

export const SIZE_PRESETS = [
  { id: 'main-1-1', label: '主图 1:1', ratio: '1:1', size: '1024x1024' },
  { id: 'detail-3-4', label: '详情图 3:4', ratio: '3:4', size: '1024x1365' },
  { id: 'social-4-5', label: '宣传图 4:5', ratio: '4:5', size: '1024x1280' },
  { id: 'banner-16-9', label: '横版 16:9', ratio: '16:9', size: '1536x864' },
  { id: 'auto', label: '自动', ratio: 'auto', size: 'auto' },
]

export function createDefaultEcommerceBrief(): EcommerceBrief {
  return {
    productName: '',
    category: '',
    sellingPoints: [],
    targetAudience: '',
    targetPlatforms: ['淘宝/天猫'],
    stylePresetId: 'minimal-white',
    suiteTemplateId: 'basic-suite',
    sizePreset: 'main-1-1',
    extraPrompt: '',
    negativePrompt: '',
    lockProduct: true,
    lockBrandStyle: true,
    allowBeautify: false,
    strictStructure: false,
    counts: {
      main: 3,
      detail: 4,
      promotion: 3,
      sku: 2,
    },
  }
}

// INDUSTRY_PRESETS 行业起步包：用户在 brief 面板顶端选中行业后，整套 brief 被完全覆盖。
// 仅作为新手入门的合理起点，所有字段（包括卖点）都鼓励用户立即按真实商品改写。
// 风格/套图/尺寸 id 均引用现有 STYLE_PRESETS / SUITE_TEMPLATES / SIZE_PRESETS。
export const INDUSTRY_PRESETS: IndustryPreset[] = [
  {
    id: 'beauty',
    name: '美妆个护',
    description: '小红书风格 + 基础套图，主图 1:1，强调质感与生活方式',
    brief: {
      ...createDefaultEcommerceBrief(),
      productName: '示例：玻尿酸口红',
      category: '美妆个护',
      sellingPoints: ['持色12小时', '水润不拔干', '日杂干净显色'],
      targetAudience: '20-35 岁注重妆效的女性',
      targetPlatforms: ['淘宝/天猫', '小红书'],
      stylePresetId: 'xiaohongshu-life',
      suiteTemplateId: 'basic-suite',
      sizePreset: 'main-1-1',
      extraPrompt: '突出唇部特写质感，画面干净通透，避免过度浓妆感',
    },
  },
  {
    id: 'apparel',
    name: '服装鞋帽',
    description: '白底主图 + 平台主图套装，详情 3:4 适配电商详情页',
    brief: {
      ...createDefaultEcommerceBrief(),
      productName: '示例：纯棉宽松T恤',
      category: '服装鞋帽',
      sellingPoints: ['莫代尔棉舒适亲肤', '垂感好不撞色', '显瘦剪裁'],
      targetAudience: '18-35 岁追求舒适基础款的年轻人',
      targetPlatforms: ['淘宝/天猫', '京东'],
      stylePresetId: 'minimal-white',
      suiteTemplateId: 'marketplace-main',
      sizePreset: 'detail-3-4',
      extraPrompt: '展示面料垂感与上身效果，避免过度修图改变版型',
    },
  },
  {
    id: 'digital',
    name: '数码 3C',
    description: '高级科技感 + 详情长图，突出参数与质感',
    brief: {
      ...createDefaultEcommerceBrief(),
      productName: '示例：无线降噪耳机',
      category: '数码 3C',
      sellingPoints: ['主动降噪', '30 小时续航', '蓝牙 5.3 低延迟'],
      targetAudience: '通勤白领、商务人士、影音爱好者',
      targetPlatforms: ['京东', '淘宝/天猫'],
      stylePresetId: 'tech-premium',
      suiteTemplateId: 'detail-long-page',
      sizePreset: 'detail-3-4',
      extraPrompt: '冷色调金属质感，强调参数和卖点信息层级',
    },
  },
  {
    id: 'food',
    name: '食品生鲜',
    description: '极简白底 + 促销套图，社媒 4:5 适合种草',
    brief: {
      ...createDefaultEcommerceBrief(),
      productName: '示例：手工原味曲奇',
      category: '食品生鲜',
      sellingPoints: ['当日烘焙', '真材实料', '不含防腐剂'],
      targetAudience: '注重健康饮食的家庭、办公室零食爱好者',
      targetPlatforms: ['淘宝/天猫', '小红书'],
      stylePresetId: 'minimal-white',
      suiteTemplateId: 'promotion-suite',
      sizePreset: 'social-4-5',
      extraPrompt: '突出食物质感和新鲜感，避免出现非真实成分',
      negativePrompt: '不要夸张医疗/养生功效承诺，不要出现竞品 Logo',
    },
  },
  {
    id: 'home',
    name: '家居生活',
    description: '极简白底 + 基础套图，主图 1:1 通用',
    brief: {
      ...createDefaultEcommerceBrief(),
      productName: '示例：北欧实木餐椅',
      category: '家居生活',
      sellingPoints: ['实木骨架稳固', '人体工学曲线', '易清洁面料'],
      targetAudience: '注重生活品质的家庭、独居青年',
      targetPlatforms: ['淘宝/天猫', '京东'],
      stylePresetId: 'minimal-white',
      suiteTemplateId: 'basic-suite',
      sizePreset: 'main-1-1',
      extraPrompt: '展示尺寸感和真实场景使用效果，避免改变结构',
    },
  },
]

export function getIndustryPreset(id: string): IndustryPreset | undefined {
  return INDUSTRY_PRESETS.find((preset) => preset.id === id)
}

// 判断 brief 是否仍处于"未填写过"状态。用于决定切换行业起步包/套用模板前是否需要二次确认。
export function isDefaultEcommerceBrief(brief: EcommerceBrief): boolean {
  const def = createDefaultEcommerceBrief()
  if (brief.productName.trim() !== '') return false
  if (brief.category.trim() !== '') return false
  if (brief.sellingPoints.length > 0) return false
  if (brief.targetAudience.trim() !== '') return false
  if (brief.extraPrompt.trim() !== '') return false
  if ((brief.negativePrompt ?? '').trim() !== '') return false
  if (brief.stylePresetId !== def.stylePresetId) return false
  if (brief.suiteTemplateId !== def.suiteTemplateId) return false
  if (brief.sizePreset !== def.sizePreset) return false
  if (brief.targetPlatforms.length !== def.targetPlatforms.length) return false
  if (brief.targetPlatforms.some((p, i) => p !== def.targetPlatforms[i])) return false
  return true
}

// cloneSuitePlanItemSkeleton 把已生成的 plan item 剥成模板骨架：
// 去掉 outputTaskIds/status/error/count/enabled，仅保留结构/提示词，避免把生成结果与运行状态写进模板。
export function cloneSuitePlanItemSkeleton(item: SuitePlanItem): SavedSuiteTemplate['plan'][number] {
  return {
    id: item.id,
    group: item.group,
    title: item.title,
    purpose: item.purpose,
    ratio: item.ratio,
    prompt: item.prompt,
    promptPurpose: item.promptPurpose,
    finalPromptDraft: item.finalPromptDraft,
  }
}

// hydrateSuitePlanItemFromSkeleton 套用模板时把骨架还原为完整 SuitePlanItem，状态重置为草稿、不带任何生成结果。
export function hydrateSuitePlanItemFromSkeleton(skeleton: SavedSuiteTemplate['plan'][number]): SuitePlanItem {
  return {
    id: skeleton.id,
    group: skeleton.group,
    title: skeleton.title,
    purpose: skeleton.purpose,
    ratio: skeleton.ratio,
    prompt: skeleton.prompt,
    promptPurpose: skeleton.promptPurpose,
    finalPromptDraft: skeleton.finalPromptDraft,
    count: 1,
    enabled: true,
    status: 'draft',
    outputTaskIds: [],
    error: null,
  }
}

type SkeletonItem = Pick<SuitePlanItem, 'id' | 'group' | 'title' | 'purpose' | 'ratio' | 'promptPurpose'>

const TEMPLATE_ITEMS: Record<string, SkeletonItem[]> = {
  'basic-suite': [
    { id: 'main-white-bg', group: 'main', title: '白底主图', purpose: '商品列表和搜索首图', ratio: '1:1', promptPurpose: '商品居中，占画面 70% 左右，纯白或浅灰背景，干净阴影，无多余装饰' },
    { id: 'main-scene', group: 'main', title: '场景主图', purpose: '突出商品使用场景', ratio: '1:1', promptPurpose: '保持商品主体一致，放入真实使用场景，增强购买联想' },
    { id: 'main-selling-points', group: 'main', title: '卖点主图', purpose: '主图中简洁传达核心卖点', ratio: '1:1', promptPurpose: '围绕 2-3 个核心卖点做干净版式，文案短，商品仍是主体' },
    { id: 'detail-core-points', group: 'detail', title: '核心卖点详情图', purpose: '详情页第一屏卖点说明', ratio: '3:4', promptPurpose: '展示商品特写、简洁标题和辅助说明图形' },
    { id: 'detail-material', group: 'detail', title: '材质工艺图', purpose: '说明材质、质感或工艺', ratio: '3:4', promptPurpose: '用局部特写、放大细节和简洁注释体现材质质感' },
    { id: 'detail-params', group: 'detail', title: '参数说明图', purpose: '展示尺寸、容量、规格参数', ratio: '3:4', promptPurpose: '用规整信息卡和产品图表达参数，文字区域清晰克制' },
    { id: 'detail-steps', group: 'detail', title: '使用步骤图', purpose: '说明使用方式', ratio: '3:4', promptPurpose: '用 3-4 步流程展示使用动作，画面简洁易懂' },
    { id: 'promo-poster', group: 'promotion', title: '活动宣传海报', purpose: '活动页、广告投放和促销传播', ratio: '4:5', promptPurpose: '保留促销标题空间，增强氛围但不改变商品' },
    { id: 'promo-social-cover', group: 'promotion', title: '社媒封面图', purpose: '小红书、抖音封面', ratio: '4:5', promptPurpose: '更生活方式的构图，有留白和封面标题区域' },
    { id: 'promo-banner', group: 'promotion', title: '横版 Banner', purpose: '店铺首页和横幅投放', ratio: '16:9', promptPurpose: '横向构图，商品和文案区域左右平衡，背景有品牌氛围' },
    { id: 'sku-colors', group: 'sku', title: '多色可选图', purpose: '展示不同颜色/SKU', ratio: '1:1', promptPurpose: '多个 SKU 排列整齐，颜色和包装与参考图一致' },
    { id: 'sku-compare', group: 'sku', title: '规格对比图', purpose: '比较不同规格容量', ratio: '1:1', promptPurpose: '清楚展示不同规格差异，适合商品详情和规格选择' },
  ],
  'marketplace-main': [
    { id: 'market-white', group: 'main', title: '平台白底主图', purpose: '淘宝/京东/亚马逊主图', ratio: '1:1', promptPurpose: '严格白底，商品居中，少装饰，无夸张文字' },
    { id: 'market-scene', group: 'main', title: '平台场景主图', purpose: '提升点击率的场景首图', ratio: '1:1', promptPurpose: '将商品放入可信使用场景，风格统一且主体清晰' },
    { id: 'market-points', group: 'main', title: '平台卖点主图', purpose: '主图卖点表达', ratio: '1:1', promptPurpose: '短卖点、强层级、少文字，避免遮挡商品' },
    { id: 'market-sku', group: 'sku', title: '规格主图', purpose: '商品规格页展示', ratio: '1:1', promptPurpose: '展示核心规格差异和整齐排列的 SKU' },
  ],
  'xiaohongshu-seeding': [
    { id: 'xhs-cover', group: 'promotion', title: '小红书封面图', purpose: '小红书笔记封面', ratio: '4:5', promptPurpose: '真实生活方式场景，柔光，封面标题区域清楚' },
    { id: 'xhs-life', group: 'promotion', title: '生活场景图', purpose: '种草内容配图', ratio: '4:5', promptPurpose: '自然使用场景，弱营销感，商品外观保持一致' },
    { id: 'xhs-card', group: 'detail', title: '卖点卡片图', purpose: '小红书卖点说明', ratio: '4:5', promptPurpose: '卡片式信息设计，3 个短卖点，文字区域不拥挤' },
    { id: 'xhs-steps', group: 'detail', title: '使用步骤图', purpose: '教程和体验说明', ratio: '4:5', promptPurpose: '按步骤展示使用动作，生活感强' },
    { id: 'xhs-main', group: 'main', title: '种草主图', purpose: '封面之外的商品主视觉', ratio: '4:5', promptPurpose: '商品主体清晰，氛围干净，适合移动端信息流' },
  ],
  'detail-long-page': [
    { id: 'detail-core-points', group: 'detail', title: '核心卖点详情图', purpose: '详情页第一屏', ratio: '3:4', promptPurpose: '高信息层级展示核心卖点' },
    { id: 'detail-material', group: 'detail', title: '材质说明图', purpose: '材质和工艺说明', ratio: '3:4', promptPurpose: '细节特写与简洁注释结合' },
    { id: 'detail-params', group: 'detail', title: '参数说明图', purpose: '规格参数展示', ratio: '3:4', promptPurpose: '参数表/信息卡表达，排版整齐' },
    { id: 'detail-steps', group: 'detail', title: '使用步骤图', purpose: '教程说明', ratio: '3:4', promptPurpose: '3-4 步流程说明' },
    { id: 'detail-package', group: 'detail', title: '包装清单图', purpose: '展示包装和配件', ratio: '3:4', promptPurpose: '清晰展示商品、包装和附件，避免生成不存在配件' },
  ],
  'sku-suite': [
    { id: 'sku-single', group: 'sku', title: '单 SKU 主图', purpose: '单规格展示', ratio: '1:1', promptPurpose: '单个 SKU 清晰居中展示' },
    { id: 'sku-colors', group: 'sku', title: '多色可选图', purpose: '颜色选择', ratio: '1:1', promptPurpose: '多色整齐排列，颜色准确' },
    { id: 'sku-compare', group: 'sku', title: '规格对比图', purpose: '规格对比', ratio: '1:1', promptPurpose: '不同容量/型号对比清晰' },
    { id: 'sku-family', group: 'sku', title: '全规格合影图', purpose: '套装和全规格展示', ratio: '1:1', promptPurpose: '全规格同框，主次有序' },
  ],
  'promotion-suite': [
    { id: 'promo-launch', group: 'promotion', title: '新品上市图', purpose: '新品推广', ratio: '4:5', promptPurpose: '新品发布氛围，商品主体强，保留标题区' },
    { id: 'promo-sale', group: 'promotion', title: '限时折扣图', purpose: '促销活动', ratio: '4:5', promptPurpose: '促销氛围强但版式克制' },
    { id: 'promo-618', group: 'promotion', title: '大促活动图', purpose: '618/双11 大促', ratio: '4:5', promptPurpose: '活动感、强视觉冲击，避免遮挡商品' },
    { id: 'promo-live', group: 'promotion', title: '直播间背景图', purpose: '直播背景', ratio: '16:9', promptPurpose: '直播背景构图，商品和卖点区域清晰' },
  ],
}

export function createEcommerceSuite(now = Date.now()): EcommerceSuite {
  return {
    id: createId('suite'),
    name: '新电商套图',
    brief: createDefaultEcommerceBrief(),
    assets: [],
    plan: [],
    createdAt: now,
    updatedAt: now,
  }
}

export function createProductAsset(kind: ProductAssetKind, imageId: string, name?: string): ProductAsset {
  return {
    id: createId('asset'),
    kind,
    imageId,
    name,
    createdAt: Date.now(),
  }
}

export function createSuiteSkeleton(brief: EcommerceBrief): SuitePlanItem[] {
  const selected = TEMPLATE_ITEMS[brief.suiteTemplateId] ?? TEMPLATE_ITEMS['basic-suite']
  const groupSeen: Record<SuitePlanGroup, number> = { main: 0, detail: 0, promotion: 0, sku: 0 }
  return selected
    .filter((item) => {
      const limit = Math.max(0, Math.trunc(brief.counts[item.group] ?? 0))
      if (groupSeen[item.group] >= limit) return false
      groupSeen[item.group] += 1
      return true
    })
    .map((item, index) => createPlanItemFromSkeleton(item, brief, index))
}

export function buildEcommerceGlobalPrompt(brief: EcommerceBrief, assets: ProductAsset[]) {
  const style = getStylePreset(brief.stylePresetId)
  const assetSummary = summarizeAssets(assets)
  return [
    '你正在为电商商品生成一组风格统一的产品宣传图。',
    '请严格参考用户上传的商品图片，保持商品主体外观、结构、颜色、比例、Logo、包装一致。',
    `商品名称：${brief.productName || '未填写'}`,
    `商品类目：${brief.category || '未填写'}`,
    `核心卖点：${brief.sellingPoints.join('、') || '未填写'}`,
    `目标人群：${brief.targetAudience || '未填写'}`,
    `目标平台：${brief.targetPlatforms.join('、') || '未填写'}`,
    `整体风格：${style.name}。${style.description}`,
    `素材摘要：${assetSummary}`,
    brief.lockProduct ? '主体锁定：必须保持商品主体一致，不要改变外观、结构、颜色、Logo、包装。' : '',
    brief.lockBrandStyle ? '品牌锁定：保持品牌色、Logo、包装视觉一致。' : '',
    brief.allowBeautify ? '允许轻微优化光影、材质和质感，但不得改变结构。' : '不要美化到改变真实商品结构。',
    brief.strictStructure ? '严格结构一致：形状、部件、比例和颜色必须尽量与参考图一致。' : '',
    brief.extraPrompt ? `补充宣传需求：${brief.extraPrompt}` : '',
    `负面约束：${brief.negativePrompt || '不要改变商品结构，不要生成不存在的配件，不要出现竞品品牌，不要夸张医疗/功效承诺。'}`,
  ].filter(Boolean).join('\n')
}

export function buildEcommerceItemPrompt(brief: EcommerceBrief, item: Pick<SuitePlanItem, 'title' | 'purpose' | 'ratio' | 'promptPurpose'>, assets: ProductAsset[]) {
  return [
    buildEcommerceGlobalPrompt(brief, assets),
    '',
    `当前图片用途：${item.title}`,
    `用途说明：${item.purpose}`,
    `画面比例：${item.ratio}`,
    `专属要求：${item.promptPurpose}`,
    '',
    '输出要求：生成一张可直接用于电商运营审稿的高质量商品宣传图，画面完整、主体清晰、风格统一。若需要文字，保持简短、清晰、不过密。',
  ].join('\n')
}

export async function callEcommercePlanApi(opts: {
  settings: AppSettings
  profile: ApiProfile
  capabilities: EcommerceCapabilities
  brief: EcommerceBrief
  assets: ProductAsset[]
  skeleton: SuitePlanItem[]
  signal?: AbortSignal
}): Promise<SuitePlanItem[]> {
  const { settings, profile, capabilities, brief, assets, skeleton, signal } = opts
  const planProfile: ApiProfile = {
    ...profile,
    model: capabilities.defaultPlanModel || DEFAULT_ECOMMERCE_CAPABILITIES.defaultPlanModel,
    apiMode: 'responses',
  }
  const proxyConfig = readClientDevProxyConfig()
  const useApiProxy = shouldUseApiProxy(planProfile.apiProxy, proxyConfig)
  const controller = new AbortController()
  const timeoutId = window.setTimeout(() => controller.abort(), planProfile.timeout * 1000)
  const abortFromCaller = () => controller.abort()
  if (signal?.aborted) controller.abort()
  signal?.addEventListener('abort', abortFromCaller, { once: true })

  try {
    const response = await fetch(buildApiUrl(planProfile.baseUrl, 'responses', proxyConfig, useApiProxy), {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${planProfile.apiKey}`,
        'Content-Type': 'application/json',
      },
      cache: 'no-store',
      body: JSON.stringify({
        model: planProfile.model || settings.model,
        instructions: ECOMMERCE_PLAN_INSTRUCTIONS,
        input: [{
          role: 'user',
          content: [
            {
              type: 'input_text',
              text: JSON.stringify({
                brief,
                assetsSummary: summarizeAssets(assets),
                templateSkeleton: skeleton.map(({ id, group, title, purpose, ratio, promptPurpose }) => ({
                  id,
                  group,
                  title,
                  purpose,
                  ratio,
                  promptPurpose,
                })),
                stylePreset: getStylePreset(brief.stylePresetId),
                schema: 'Return JSON: {"items":[{"id","group","title","purpose","ratio","promptPurpose","finalPromptDraft"}]}',
              }),
            },
          ],
        }],
        text: {
          format: {
            type: 'json_schema',
            name: 'ecommerce_suite_plan',
            strict: true,
            schema: PLAN_SCHEMA,
          },
        },
      }),
      signal: controller.signal,
    })

    if (!response.ok) throw new Error(await getApiErrorMessage(response))
    const payload = await response.json() as ResponsesApiResponse
    const text = extractResponseText(payload)
    return mergePlanCompletion(skeleton, text, brief, assets)
  } finally {
    window.clearTimeout(timeoutId)
    signal?.removeEventListener('abort', abortFromCaller)
  }
}

export function fallbackEcommercePlan(brief: EcommerceBrief, assets: ProductAsset[], skeleton = createSuiteSkeleton(brief)): SuitePlanItem[] {
  return skeleton.map((item) => ({
    ...item,
    prompt: item.finalPromptDraft || buildEcommerceItemPrompt(brief, item, assets),
    finalPromptDraft: item.finalPromptDraft || buildEcommerceItemPrompt(brief, item, assets),
  }))
}

export function selectEcommerceInputImageIds(suite: EcommerceSuite, item: SuitePlanItem): string[] {
  const ids: string[] = []
  const add = (asset: ProductAsset | undefined) => {
    if (asset && !ids.includes(asset.imageId)) ids.push(asset.imageId)
  }
  const addMany = (kind: ProductAssetKind, limit: number) => {
    suite.assets.filter((asset) => asset.kind === kind).slice(0, limit).forEach(add)
  }

  add(suite.assets.find((asset) => asset.kind === 'main'))
  addMany('angle', item.group === 'detail' ? 6 : 3)
  addMany('brand', 3)
  if (item.group === 'sku') addMany('sku', 8)
  if (item.group === 'promotion' || item.group === 'main') addMany('style', 3)
  return ids
}

export function getStylePreset(id: string) {
  return STYLE_PRESETS.find((item) => item.id === id) ?? STYLE_PRESETS[0]
}

export function getSizePreset(id: string) {
  return SIZE_PRESETS.find((item) => item.id === id) ?? SIZE_PRESETS[0]
}

export function getSuiteTemplate(id: string) {
  return SUITE_TEMPLATES.find((item) => item.id === id) ?? SUITE_TEMPLATES[0]
}

export function getBucketLimit(kind: ProductAssetKind) {
  return PRODUCT_ASSET_BUCKETS.find((bucket) => bucket.kind === kind)?.max ?? 8
}

function createId(prefix: string) {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
}

function createPlanItemFromSkeleton(item: SkeletonItem, brief: EcommerceBrief, index: number): SuitePlanItem {
  const sizePreset = getSizePreset(brief.sizePreset)
  const ratio = item.ratio === 'auto' ? sizePreset.ratio : item.ratio || sizePreset.ratio
  const id = `${item.id}-${index + 1}`
  return {
    id,
    group: item.group,
    title: item.title,
    purpose: item.purpose,
    ratio,
    promptPurpose: item.promptPurpose,
    prompt: '',
    finalPromptDraft: '',
    count: 1,
    enabled: true,
    status: 'draft',
    outputTaskIds: [],
    error: null,
  }
}

function summarizeAssets(assets: ProductAsset[]) {
  if (assets.length === 0) return '暂无上传素材'
  const counts = PRODUCT_ASSET_BUCKETS
    .map((bucket) => `${bucket.label} ${assets.filter((asset) => asset.kind === bucket.kind).length} 张`)
    .join('，')
  return `${counts}。素材仅按分类和数量描述，实际图片会作为参考图传入生成任务。`
}

function extractResponseText(payload: ResponsesApiResponse) {
  const chunks: string[] = []
  for (const item of payload.output ?? []) {
    if (item.type !== 'message') continue
    for (const part of item.content ?? []) {
      if ((part.type === 'output_text' || part.type === 'text') && typeof part.text === 'string') {
        chunks.push(part.text)
      }
    }
  }
  return chunks.join('\n').trim()
}

function parsePlanJson(text: string): unknown {
  const trimmed = text.trim()
  if (!trimmed) return null
  try {
    return JSON.parse(trimmed)
  } catch {
    const match = trimmed.match(/\{[\s\S]*\}/)
    if (!match) return null
    try {
      return JSON.parse(match[0])
    } catch {
      return null
    }
  }
}

function mergePlanCompletion(skeleton: SuitePlanItem[], text: string, brief: EcommerceBrief, assets: ProductAsset[]): SuitePlanItem[] {
  const parsed = parsePlanJson(text)
  const rawItems = parsed && typeof parsed === 'object' && Array.isArray((parsed as { items?: unknown }).items)
    ? (parsed as { items: unknown[] }).items
    : []
  const byId = new Map<string, Record<string, unknown>>()
  for (const raw of rawItems) {
    if (!raw || typeof raw !== 'object') continue
    const item = raw as Record<string, unknown>
    if (typeof item.id === 'string') byId.set(item.id, item)
  }

  return skeleton.map((item) => {
    const completed = byId.get(item.id)
    const next: SuitePlanItem = {
      ...item,
      title: readString(completed?.title, item.title),
      purpose: readString(completed?.purpose, item.purpose),
      ratio: readString(completed?.ratio, item.ratio),
      promptPurpose: readString(completed?.promptPurpose, item.promptPurpose),
      finalPromptDraft: readString(completed?.finalPromptDraft, ''),
      status: 'draft',
      outputTaskIds: [],
      error: null,
    }
    const prompt = next.finalPromptDraft || buildEcommerceItemPrompt(brief, next, assets)
    return { ...next, prompt, finalPromptDraft: prompt }
  })
}

function readString(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

const ECOMMERCE_PLAN_INSTRUCTIONS = [
  '你是电商产品视觉策划助手，只负责把套图骨架补全为可执行的图片生成计划。',
  '必须保留输入 templateSkeleton 中每一项的 id、group，不要新增、删除或改变分组。',
  '可以优化 title、purpose、ratio、promptPurpose，并为每项生成 finalPromptDraft。',
  'finalPromptDraft 必须包含商品一致性、风格模板、目标平台、当前用途和禁忌约束。',
  '不同用途的 finalPromptDraft 必须有明显差异。',
  '只输出符合 schema 的 JSON，不输出 Markdown、解释或代码块。',
].join('\n')

const PLAN_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  properties: {
    items: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          id: { type: 'string' },
          group: { type: 'string', enum: ['main', 'detail', 'promotion', 'sku'] },
          title: { type: 'string' },
          purpose: { type: 'string' },
          ratio: { type: 'string' },
          promptPurpose: { type: 'string' },
          finalPromptDraft: { type: 'string' },
        },
        required: ['id', 'group', 'title', 'purpose', 'ratio', 'promptPurpose', 'finalPromptDraft'],
      },
    },
  },
  required: ['items'],
}
