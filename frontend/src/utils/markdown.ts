// markdown.ts: Markdown 渲染引擎（轻量版）
// 使用 marked + highlight.js + DOMPurify，对齐 llama.cpp 原生 webui 渲染方式
//
// 渲染流程：
//   原始 Markdown → marked 解析（GFM + 换行）
//   → 自定义 renderer（代码高亮 + 代码头 + 外部链接新窗口）
//   → DOMPurify 安全过滤

import { marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js'
import { isSafeUrl } from './lightSanitize'
import { logWarn } from './logger'

// highlight.js 官方主题：亮色模式默认加载，深色模式按需动态加载
import 'highlight.js/styles/github.css'

// ===== 工具函数（先定义，供后续 purify 兜底使用） =====

/** HTML 转义（导出供组件 catch 回退分支使用，避免直接赋值原始未消毒内容到 v-html） */
export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

// ===== DOMPurify 兼容处理 =====
// dompurify v3 的 ESM/CJS 导出方式不同：
// - 浏览器中：default 是工厂函数，需要调用 createDOMPurify(window) 或直接 .sanitize
// - Node.js 测试中：可能被 mock
const purify = (() => {
  const d = DOMPurify as any
  if (d && typeof d.sanitize === 'function') return d
  if (typeof d === 'function' && typeof window !== 'undefined') {
    return d(window)
  }
  // 安全降级：无 window 环境（如测试）中用 escapeHtml 转义并保留换行
  logWarn('[security] DOMPurify unavailable, fallback to escapeHtml')
  return {
    sanitize: (html: string) => escapeHtml(html).replace(/\n/g, '<br>')
  }
})()

// ===== 深色代码主题动态加载与切换 =====
//
// 实现原理：
// - 亮色主题 github.css 在启动时静态导入（默认生效，无法卸载）
// - 深色主题 github-dark.css 在首次切换到深色模式时动态导入
// - 动态导入后捕获其 <style> 元素引用，通过 disabled 属性控制启用/禁用
let darkCodeThemeEl: HTMLStyleElement | null = null
let darkCodeThemePromise: Promise<void> | null = null

/**
 * 动态加载 github-dark.css 并捕获其 <style> 元素引用
 *
 * 定位策略：github-dark.css 的特征色是 #ff7b72（关键字色），
 * 通过查询 textContent 包含该色的 <style> 标签精准定位
 */
export async function loadDarkCodeTheme(): Promise<void> {
  if (darkCodeThemeEl) return
  if (darkCodeThemePromise) return darkCodeThemePromise
  darkCodeThemePromise = import('highlight.js/styles/github-dark.css').then(() => {
    // 测试环境（happy-dom）下 document.head 可能未初始化，跳过 DOM 操作
    if (typeof document === 'undefined' || !document.head) return
    // 通过特征色 #ff7b72 精准定位 github-dark.css 注入的 <style>
    const allStyles = Array.from(document.head.querySelectorAll('style'))
    darkCodeThemeEl = allStyles.find(s =>
      s.textContent?.includes('#ff7b72')
    ) as HTMLStyleElement | null
    // 默认禁用，等 applyCodeTheme 根据当前主题决定是否启用
    if (darkCodeThemeEl) {
      darkCodeThemeEl.disabled = true
    }
  })
  return darkCodeThemePromise
}

/**
 * 根据主题状态启用/禁用深色代码高亮
 * @param isDark 是否为深色模式
 */
export function applyCodeTheme(isDark: boolean): void {
  if (isDark) {
    if (darkCodeThemeEl) {
      darkCodeThemeEl.disabled = false
    } else {
      // 首次切换到深色：加载并自动启用
      loadDarkCodeTheme().then(() => {
        if (darkCodeThemeEl) darkCodeThemeEl.disabled = false
      })
    }
  } else {
    // 亮色：禁用深色主题，让静态导入的 github.css 生效
    if (darkCodeThemeEl) {
      darkCodeThemeEl.disabled = true
    }
  }
}

// ===== 配置 marked =====
//
// marked 配置说明：
// - gfm: true       启用 GitHub Flavored Markdown（表格、删除线、任务列表等）
// - breaks: true    单个换行符转 <br>
// - renderer.code   重写代码块渲染：语法高亮 + 代码头（语言标签 + 复制按钮）
// - renderer.link   重写链接渲染：外部链接新窗口打开

const renderer = new marked.Renderer()

/**
 * 重写代码块渲染
 *
 * 生成的 HTML 结构（复用现有 CSS 和 codeCopy.ts）：
 * <pre class="hljs">
 *   <div class="code-header">
 *     <span class="code-lang">javascript</span>
 *     <button class="code-copy-btn" type="button">
 *       <svg class="cp-icon cp-copy-ic">…复制图标…</svg>
 *       <svg class="cp-icon cp-done-ic">…对勾图标…</svg>
 *       <span class="cp-label">复制</span>
 *     </button>
 *   </div>
 *   <code class="hljs language-javascript">...高亮后的代码...</code>
 * </pre>
 *
 * 复制按钮采用「复制 / 对勾」双态图标：
 * - cp-copy-ic：默认显示的「复制」图标
 * - cp-done-ic：复制成功后显示的「对勾」图标（默认隐藏，由 .copied 切换显示）
 * - cp-label：按钮文字（codeCopy.ts 会替换其内容为「已复制 / 复制」）
 * 图标使用 lucide 风格线性图标，stroke=currentColor 以便随按钮文字颜色变化。
 *
 * marked v18 renderer.code 签名：code({ text, lang, escaped }: Tokens.Code)
 * - text: 代码内容（字符串）
 * - lang: 语言标识（如 "javascript"，可能为 undefined）
 * - escaped: 是否已转义
 */
renderer.code = function ({ text, lang }: Tokens.Code): string {
  const language = lang && hljs.getLanguage(lang) ? lang : ''
  // 调用 highlight.js 进行语法高亮
  const highlighted = language ? hljs.highlight(text, { language }).value : escapeHtml(text)
  const langLabel = lang || ''
  const escapedLang = escapeHtml(langLabel)
  return `<pre class="hljs"><div class="code-header"><span class="code-lang">${escapedLang}</span><button class="code-copy-btn" type="button" aria-label="复制代码"><svg class="cp-icon cp-copy-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg><svg class="cp-icon cp-done-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M20 6 9 17l-5-5"></path></svg><span class="cp-label">复制</span></button></div><code class="hljs language-${escapedLang}">${highlighted}</code></pre>`
}

/**
 * 重写链接渲染：外部链接新窗口打开，避免离开当前聊天界面。
 *
 * marked v18 renderer.link 签名：link({ href, title, tokens }: Tokens.Link)
 * - href: 链接地址
 * - title: 标题（可能为 null/undefined）
 * - tokens: 链接文本的子 token 数组，需用 this.parser.parseInline 渲染为 HTML
 */
renderer.link = function ({ href, title, tokens }: Tokens.Link): string {
  const text = this.parser.parseInline(tokens)
  const titleAttr = title ? ` title="${escapeHtml(title)}"` : ''
  // 安全实践：校验协议白名单（http(s) / mailto / tel / #），
  // 不安全链接（如 javascript: / vbscript: / data:text/html）降级为 #
  const safeHref = isSafeUrl(href) ? href : '#'
  return `<a href="${escapeHtml(safeHref)}"${titleAttr} target="_blank" rel="noopener noreferrer">${text}</a>`
}

// 初始化 marked 配置（全局一次）
marked.use({
  gfm: true,
  breaks: true,
  renderer
})

// ===== 核心渲染函数 =====

/**
 * 渲染完整 Markdown（异步）
 * 用于：历史消息、思考内容等已完成的文本
 *
 * 流程：marked 解析为 HTML，再经 DOMPurify 安全过滤后返回。
 */
export async function renderMarkdown(content: string): Promise<string> {
  if (!content) return ''
  try {
    // marked.parse 默认同步，返回 string
    const html = marked.parse(content, { async: false }) as string
    return sanitizeHtml(html)
  } catch (_) {
    // 降级：转义 HTML 并返回
    return sanitizeHtml(escapeHtml(content))
  }
}

// ===== 安全过滤 =====

/**
 * DOMPurify 安全过滤
 *
 * 安全加固：显式硬化白名单与协议限制，作为纵深防御。
 * - 不设置 ALLOWED_TAGS/ALLOWED_ATTR：用 DOMPurify 默认白名单，避免破坏 markdown 渲染
 * - ALLOWED_URI_REGEXP：URI 协议白名单（http(s) / mailto / ftp / tel / data:image / #）
 * - FORBID_ATTR：禁用 style 与常见危险事件属性
 */
export function sanitizeHtml(html: string): string {
  return purify.sanitize(html, {
    // 安全实践：显式 URI 协议白名单 + 禁用危险事件属性
    ALLOWED_URI_REGEXP: /^(?:https?|mailto|ftp|tel|data:image\/(?:png|jpeg|gif|webp|bmp)|#)/i,
    FORBID_ATTR: ['style', 'onerror', 'onload', 'onclick', 'onmouseover']
  })
}
