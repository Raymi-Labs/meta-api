package service

import (
	"bytes"
	"io/fs"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// seo_meta.go 负责给 SPA 的 indexPage 按当前请求路径注入差异化的 SEO meta（title/description/canonical/og）。
// 这是 prerender（阶段 2）落地前的过渡方案：即便没有真预渲染产物，搜索引擎抓 HTML 时也能拿到差异化标签。
// 一旦 prerender 上线，GetPrerenderedHTML 优先返回静态 HTML，本文件的模板替换链路自动降级为兜底。

// routeMeta 描述单个公开路由的 SEO 元数据。
// 字段语义：
//   - Title       完整 <title>
//   - Description <meta name="description"> 与 og:description
//   - Canonical   <link rel="canonical"> 的绝对 URL
//   - OGURL       og:url（通常与 Canonical 相同）
type routeMeta struct {
	Title       string
	Description string
	Canonical   string
	OGURL       string
}

const seoSiteOrigin = "https://cn.meta-api.vip"

var imagePlaygroundMeta = routeMeta{
	Title:       "GPT Image Playground · AI 电商套图与图像编辑工作台 | gpt-image-2 在线生成",
	Description: "GPT Image Playground 是基于 gpt-image-2 的在线 AI 图像生成与编辑工作台，支持电商套图一键生成、行业起步包、个人模板库、批量出图与多平台尺寸预设。",
	Canonical:   seoSiteOrigin + "/image-playground/",
	OGURL:       seoSiteOrigin + "/image-playground/",
}

// routeMetaMap 公开路由 → 元数据映射。
// key 必须是请求的 URL.Path 精确字符串（init 会自动补一份带尾斜杠的别名）。
// 文案策略：品牌词「元衡 API」打头 + 长尾场景词（Claude Code / Codex / Gemini CLI 接入），
// 避开「AI 中转网关」这类头部大词；开源项目 new-api 的署名保留在 description 中。
var routeMetaMap = map[string]routeMeta{
	"/": {
		Title:       "元衡 API：一口接入多模型，智能调度更稳定 | Claude Code / Codex / Gemini CLI 稳定接入",
		Description: "元衡 API 基于开源 new-api 构建：一口接入 OpenAI、Claude、Gemini、DeepSeek、Qwen 等 40+ 模型，智能调度更稳定，支持 Claude Code、Codex CLI、Gemini CLI，一套 API Key 按量透明计费。",
		Canonical:   seoSiteOrigin + "/",
		OGURL:       seoSiteOrigin + "/",
	},
	"/login": {
		Title:       "登录 · 元衡 API 控制台",
		Description: "登录元衡 API 控制台，管理你的 API Key、查看用量与计费、配置模型调用策略。",
		Canonical:   seoSiteOrigin + "/login",
		OGURL:       seoSiteOrigin + "/login",
	},
	"/register": {
		Title:       "注册 · 元衡 API | 获取统一 API Key 调用 40+ 大模型",
		Description: "注册元衡 API 账户，立即获取统一 API Key，一套密钥调用 OpenAI / Claude / Gemini / DeepSeek 等 40+ 模型。",
		Canonical:   seoSiteOrigin + "/register",
		OGURL:       seoSiteOrigin + "/register",
	},
	"/pricing": {
		Title:       "模型价格 · Claude / GPT / Gemini API 中转价格 | 元衡 API",
		Description: "元衡 API 提供 Claude、GPT、Gemini、DeepSeek、Qwen、gpt-image 等模型的中转价格与计费倍率，按 Token / 调用次数透明计费，支持缓存命中优惠。",
		Canonical:   seoSiteOrigin + "/pricing",
		OGURL:       seoSiteOrigin + "/pricing",
	},
	"/download": {
		Title:       "下载 YuanHeng Desktop · 统一管理 AI 终端、客户端与模型",
		Description: "下载 YuanHeng Desktop，统一管理 Codex、Claude Code、ChatGPT 等 AI 工具的终端、客户端、模型和分组，支持应用内自动更新。",
		Canonical:   seoSiteOrigin + "/download",
		OGURL:       seoSiteOrigin + "/download",
	},
	"/about": {
		Title:       "关于 · 元衡 API",
		Description: "了解元衡 API 平台背景、技术架构（基于开源 new-api）、运维状态与服务条款。",
		Canonical:   seoSiteOrigin + "/about",
		OGURL:       seoSiteOrigin + "/about",
	},
	"/image-playground":  imagePlaygroundMeta,
	"/image-playground/": imagePlaygroundMeta,
	// —— 站内文档：长尾词主战场（与 web/src/pages/Docs 的路由一一对应）——
	"/docs": {
		Title:       "使用文档 · 三分钟接入 40+ 大模型 | 元衡 API",
		Description: "元衡 API 平台使用文档：注册、获取 API Key、替换 Base URL 三步接入，覆盖 Claude Code、Codex、Gemini CLI、OpenCode 等客户端配置教程与常见问题排查。",
		Canonical:   seoSiteOrigin + "/docs",
		OGURL:       seoSiteOrigin + "/docs",
	},
	"/docs/clients": {
		Title:       "客户端接入指南 · Claude Code / Codex / Gemini CLI / OpenCode | 元衡 API",
		Description: "各编程客户端接入元衡 API 的配置指南：Claude Code、Codex CLI、Gemini CLI、OpenCode、OpenClaw、CC Switch，含密钥配置与环境变量示例。",
		Canonical:   seoSiteOrigin + "/docs/clients",
		OGURL:       seoSiteOrigin + "/docs/clients",
	},
	"/docs/clients/claude-code": {
		Title:       "Claude Code 中转接入教程 · 稳定使用 Claude 模型 | 元衡 API",
		Description: "Claude Code 接入元衡 API 中转的完整教程：配置 ANTHROPIC_BASE_URL 与 API Key，几分钟即可在国内网络稳定使用 Claude 模型编程。",
		Canonical:   seoSiteOrigin + "/docs/clients/claude-code",
		OGURL:       seoSiteOrigin + "/docs/clients/claude-code",
	},
	"/docs/clients/claude-code-openai": {
		Title:       "Claude Code 调用 OpenAI 模型教程 | 元衡 API",
		Description: "在 Claude Code 中通过元衡 API 调用 OpenAI（GPT）系列模型的配置方法与注意事项。",
		Canonical:   seoSiteOrigin + "/docs/clients/claude-code-openai",
		OGURL:       seoSiteOrigin + "/docs/clients/claude-code-openai",
	},
	"/docs/clients/codex": {
		Title:       "Codex CLI 中转接入教程 · OpenAI Codex 稳定使用 | 元衡 API",
		Description: "OpenAI Codex CLI 接入元衡 API 中转的完整教程：配置 base_url 与 API Key，国内网络稳定使用 Codex 编程模型。",
		Canonical:   seoSiteOrigin + "/docs/clients/codex",
		OGURL:       seoSiteOrigin + "/docs/clients/codex",
	},
	"/docs/clients/gemini": {
		Title:       "Gemini CLI 接入教程 · 稳定调用 Gemini API | 元衡 API",
		Description: "Gemini CLI 接入元衡 API 中转的配置教程：设置 GOOGLE_GEMINI_BASE_URL 与 API Key，稳定调用 Gemini 系列模型。",
		Canonical:   seoSiteOrigin + "/docs/clients/gemini",
		OGURL:       seoSiteOrigin + "/docs/clients/gemini",
	},
	"/docs/clients/opencode": {
		Title:       "OpenCode 接入教程 | 元衡 API",
		Description: "OpenCode 客户端接入元衡 API 的 provider 配置教程与模型选择建议。",
		Canonical:   seoSiteOrigin + "/docs/clients/opencode",
		OGURL:       seoSiteOrigin + "/docs/clients/opencode",
	},
	"/docs/clients/openclaw": {
		Title:       "OpenClaw 接入教程 | 元衡 API",
		Description: "OpenClaw 客户端接入元衡 API 的配置教程，含密钥与模型映射示例。",
		Canonical:   seoSiteOrigin + "/docs/clients/openclaw",
		OGURL:       seoSiteOrigin + "/docs/clients/openclaw",
	},
	"/docs/clients/ccswitch": {
		Title:       "CC Switch 多服务商切换教程 | 元衡 API",
		Description: "使用 CC Switch 在 Claude Code 中一键切换元衡 API 等多个服务商的配置教程。",
		Canonical:   seoSiteOrigin + "/docs/clients/ccswitch",
		OGURL:       seoSiteOrigin + "/docs/clients/ccswitch",
	},
	"/docs/troubleshooting": {
		Title:       "常见问题排查 · API 调用报错处理 | 元衡 API",
		Description: "元衡 API 常见问题排查指南：401/403/429 报错、超时、模型不可用等问题的原因与解决方法。",
		Canonical:   seoSiteOrigin + "/docs/troubleshooting",
		OGURL:       seoSiteOrigin + "/docs/troubleshooting",
	},
}

// init 为所有不带尾斜杠的路由补一份带尾斜杠的别名（如 /docs → /docs/），
// 因为 RenderIndexWithMeta 按 URL.Path 精确匹配，两种写法都应命中同一份 meta。
func init() {
	aliases := make(map[string]routeMeta, len(routeMetaMap))
	for p, m := range routeMetaMap {
		if p != "/" && !strings.HasSuffix(p, "/") {
			aliases[p+"/"] = m
		}
	}
	for p, m := range aliases {
		if _, exists := routeMetaMap[p]; !exists {
			routeMetaMap[p] = m
		}
	}
}

// GetRouteMeta 暴露给测试与未来需要预渲染时使用。
func GetRouteMeta(path string) (routeMeta, bool) {
	m, ok := routeMetaMap[path]
	return m, ok
}

// RenderIndexWithMeta 在不破坏原 indexPage 字节的前提下，按路径注入差异化的 SEO meta。
// 未识别的路径返回 template 原始内容，保证向后兼容（旧路径行为完全不变）。
//
// 注入策略：
//  1. 替换第一个 <title>...</title> 为新 title
//  2. 替换第一个 name="description" content="..." 为新 description
//  3. 替换第一个 rel="canonical" href="..." 为新 canonical
//  4. 替换 property="og:url" content="..." 为新 OGURL
//  5. 替换 property="og:title" content="..." 为新 Title
//  6. 替换 property="og:description" content="..." 为新 Description
//
// 如果模板里没有对应的字段（旧的 image-playground/index.html 没 og:url），则跳过，不报错。
func RenderIndexWithMeta(template []byte, path string) []byte {
	meta, ok := routeMetaMap[path]
	if !ok {
		return template
	}
	out := template
	out = replaceTitle(out, meta.Title)
	out = replaceMetaName(out, "description", meta.Description)
	out = replaceCanonical(out, meta.Canonical)
	out = replaceMetaProperty(out, "og:url", meta.OGURL)
	out = replaceMetaProperty(out, "og:title", meta.Title)
	out = replaceMetaProperty(out, "og:description", meta.Description)
	out = replaceMetaName(out, "twitter:title", meta.Title)
	out = replaceMetaName(out, "twitter:description", meta.Description)
	return out
}

var (
	prerenderedMap     = map[string][]byte{}
	prerenderedMapLock sync.RWMutex
)

// LoadPrerendered 从 embed.FS 加载 SEO 阶段 2 的预渲染产物到内存 map。
// 由 main.go 启动时调用一次。读取路径与 web/scripts/prerender.mjs 输出位置约定一致。
// 任何一个文件不存在都不视为错误（旁路为模板替换链路），但会记录日志便于排查。
func LoadPrerendered(buildFS fs.FS) {
	// (URL path, embed.FS 相对路径) 对，需与 prerender.mjs 中 ROUTES 保持同步。
	// 同一份 HTML 注册两个 path key（带/不带尾斜杠），便于 router 任意一种写法都命中。
	entries := []struct {
		paths    []string
		embedKey string
	}{
		{[]string{"/login", "/login/"}, "web/dist/login/index.html"},
		{[]string{"/register", "/register/"}, "web/dist/register/index.html"},
		{[]string{"/pricing", "/pricing/"}, "web/dist/pricing/index.html"},
		{[]string{"/download", "/download/"}, "web/dist/download/index.html"},
		{[]string{"/about", "/about/"}, "web/dist/about/index.html"},
	}
	loaded := map[string][]byte{}
	for _, e := range entries {
		data, err := fs.ReadFile(buildFS, e.embedKey)
		if err != nil {
			common.SysLog("prerendered html not found, falling back to template injection: " + e.embedKey)
			continue
		}
		for _, p := range e.paths {
			loaded[p] = data
		}
	}
	prerenderedMapLock.Lock()
	prerenderedMap = loaded
	prerenderedMapLock.Unlock()
	common.SysLog("loaded prerendered SEO pages: " + intToStr(len(loaded)))
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// GetPrerenderedHTML 命中时返回预渲染产物（阶段 2），未命中或未加载时返回 nil
// （router 自动降级到 RenderIndexWithMeta 模板替换链路）。
func GetPrerenderedHTML(path string) []byte {
	prerenderedMapLock.RLock()
	defer prerenderedMapLock.RUnlock()
	return prerenderedMap[path]
}

// replaceTitle 替换首个 <title>...</title>。
func replaceTitle(html []byte, newTitle string) []byte {
	start := bytes.Index(html, []byte("<title>"))
	if start < 0 {
		return html
	}
	end := bytes.Index(html[start:], []byte("</title>"))
	if end < 0 {
		return html
	}
	end += start
	buf := make([]byte, 0, len(html)+len(newTitle))
	buf = append(buf, html[:start+len("<title>")]...)
	buf = append(buf, []byte(htmlEscape(newTitle))...)
	buf = append(buf, html[end:]...)
	return buf
}

// replaceMetaName 替换 <meta name="<key>" content="..." />。匹配 key 完全一致。
func replaceMetaName(html []byte, key string, newContent string) []byte {
	return replaceMetaTag(html, `name="`+key+`"`, newContent)
}

// replaceMetaProperty 替换 <meta property="<key>" content="..." />。
func replaceMetaProperty(html []byte, key string, newContent string) []byte {
	return replaceMetaTag(html, `property="`+key+`"`, newContent)
}

// replaceMetaTag 在含 marker（如 name="description"）的 <meta> 标签里替换 content 属性值。
// 同一文档里只替换第一处出现。未找到 marker / content 属性时返回原 html，不报错（向后兼容）。
func replaceMetaTag(html []byte, marker string, newContent string) []byte {
	idx := bytes.Index(html, []byte(marker))
	if idx < 0 {
		return html
	}
	// 在 marker 周围定位 <meta> 标签起止
	tagStart := bytes.LastIndex(html[:idx], []byte("<meta"))
	if tagStart < 0 {
		return html
	}
	tagEnd := bytes.Index(html[idx:], []byte(">"))
	if tagEnd < 0 {
		return html
	}
	tagEnd += idx
	contentIdx := bytes.Index(html[tagStart:tagEnd], []byte(`content="`))
	if contentIdx < 0 {
		return html
	}
	contentIdx += tagStart + len(`content="`)
	contentEnd := bytes.Index(html[contentIdx:tagEnd], []byte(`"`))
	if contentEnd < 0 {
		return html
	}
	contentEnd += contentIdx
	buf := make([]byte, 0, len(html)+len(newContent))
	buf = append(buf, html[:contentIdx]...)
	buf = append(buf, []byte(htmlEscape(newContent))...)
	buf = append(buf, html[contentEnd:]...)
	return buf
}

// replaceCanonical 替换 <link rel="canonical" href="..." />。
func replaceCanonical(html []byte, newHref string) []byte {
	idx := bytes.Index(html, []byte(`rel="canonical"`))
	if idx < 0 {
		return html
	}
	tagStart := bytes.LastIndex(html[:idx], []byte("<link"))
	if tagStart < 0 {
		return html
	}
	tagEnd := bytes.Index(html[idx:], []byte(">"))
	if tagEnd < 0 {
		return html
	}
	tagEnd += idx
	hrefIdx := bytes.Index(html[tagStart:tagEnd], []byte(`href="`))
	if hrefIdx < 0 {
		return html
	}
	hrefIdx += tagStart + len(`href="`)
	hrefEnd := bytes.Index(html[hrefIdx:tagEnd], []byte(`"`))
	if hrefEnd < 0 {
		return html
	}
	hrefEnd += hrefIdx
	buf := make([]byte, 0, len(html)+len(newHref))
	buf = append(buf, html[:hrefIdx]...)
	buf = append(buf, []byte(htmlEscape(newHref))...)
	buf = append(buf, html[hrefEnd:]...)
	return buf
}

// htmlEscape 用最小转义防止注入：仅处理 SEO 字段里可能出现的 " < > &。
// 不使用 html/template 是为了避免引入额外渲染开销，且这些字段都是受控的内置字符串。
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		`&`, `&amp;`,
		`<`, `&lt;`,
		`>`, `&gt;`,
		`"`, `&quot;`,
	)
	return r.Replace(s)
}
