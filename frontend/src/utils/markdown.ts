// markdown.ts: Markdown 渲染引擎
// 使用 remark + rehype 管道，与 llama.cpp 原生 webui 保持一致
//
// 渲染流程：
//   原始 Markdown → preprocessLaTeX（保护 LaTeX/代码块/转义货币$）
//   → remark 解析（GFM + 数学公式 + 换行）
//   → remark-rehype 转换
//   → rehype-katex 渲染数学公式
//   → rehype-mermaid-pre（转换 mermaid 代码块）
//   → rehype-highlight（代码语法高亮）
//   → rehype-code-blocks（添加代码头和复制按钮）
//   → rehype-external-links（外部链接新窗口打开）
//   → rehype-stringify 输出 HTML
//   → DOMPurify 安全过滤

import { remark } from 'remark'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import remarkBreaks from 'remark-breaks'
import remarkRehype from 'remark-rehype'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import { all as lowlightAll } from 'lowlight'
import rehypeStringify from 'rehype-stringify'
import { visit } from 'unist-util-visit'
import DOMPurify from 'dompurify'
import { preprocessLaTeX } from './latex-protection'
import { rehypeMermaidPre, rehypeExternalLinks, hastToString } from './rehypePlugins'

// 关键改动：mermaid 改为 dynamic import，启动时不加载（2.84MB 独立 chunk，按需加载）
// 类型：typeof import('mermaid') 用于类型推断，运行时不会触发实际加载
type MermaidModule = typeof import('mermaid')

// KaTeX CSS
import 'katex/dist/katex.min.css'

// highlight.js 官方主题（亮色模式）
// 暗色模式覆盖在 style.css 中通过 .dark 选择器处理
import 'highlight.js/styles/github.css'

// DOMPurify 兼容处理
// dompurify v3 的 ESM/CJS 导出方式不同：
// - 浏览器中：default 是工厂函数，需要调用 createDOMPurify(window) 或直接 .sanitize
// - Node.js 测试中：可能被 mock
const purify = (() => {
    const d = DOMPurify as any
    // 如果已经有 sanitize 方法，直接用
    if (d && typeof d.sanitize === 'function') return d
    // 如果是工厂函数（浏览器环境），需要 window
    if (typeof d === 'function' && typeof window !== 'undefined') {
        return d(window)
    }
    // 降级：仅在无 window 的环境（如测试）中使用
    // 移除危险的 HTML 标签和事件属性
    return {
        sanitize: (html: string) =>
            html
                .replace(/<script[\s\S]*?<\/script>/gi, '')
                .replace(/<iframe[\s\S]*?<\/iframe>/gi, '')
                .replace(/<object[\s\S]*?<\/object>/gi, '')
                .replace(/<embed[^>]*>/gi, '')
                .replace(/\s+on\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, ''),
    }
})()

// ===== Mermaid 懒加载 =====

// mermaid 模块缓存：首次 dynamic import 后保留引用
let mermaidModule: MermaidModule | null = null
let mermaidInitialized = false
let mermaidCounter = 0

/**
 * 懒加载 mermaid 模块并初始化（仅首次调用时触发实际下载）
 * 后续调用直接返回缓存的模块引用
 */
async function loadMermaid(): Promise<MermaidModule> {
    if (mermaidModule) return mermaidModule
    // dynamic import: 启动时不会加载，2.84MB chunk 按需下载
    const mod = await import('mermaid')
    mermaidModule = mod
    if (!mermaidInitialized) {
        mod.default.initialize({
            startOnLoad: false,
            theme: 'default',
            securityLevel: 'strict',
        })
        mermaidInitialized = true
    }
    return mod
}

/**
 * 渲染指定元素内的所有 mermaid 图表
 * 首次调用时才会触发 mermaid chunk 下载
 *
 * Mermaid 输出已是 securityLevel: 'strict' 模式下的安全 SVG，
 * 但仍通过 sanitizeMermaidSvg 进行二次消毒，防止未来版本或配置变更导致风险。
 */
export async function renderMermaidInElement(el: HTMLElement) {
    const mermaidEls = el.querySelectorAll('.mermaid:not([data-mermaid-rendered])')
    if (mermaidEls.length === 0) return

    // 首次触发 mermaid 模块加载（dynamic import 异步）
    const mermaid = await loadMermaid()
    const elements = Array.from(mermaidEls) as HTMLElement[]
    for (const mermaidEl of elements) {
        const id = `mermaid-${++mermaidCounter}`
        mermaidEl.setAttribute('data-mermaid-rendered', '1')
        try {
            const { svg } = await mermaid.default.render(id, mermaidEl.textContent || '')
            // 即使 mermaid 已设置 securityLevel: strict，仍用 DOMPurify 二次消毒
            mermaidEl.innerHTML = sanitizeMermaidSvg(svg)
        } catch (_) { /* empty */ }
    }
}

// ===== 自定义 rehype 插件 =====

/**
 * rehypeCodeBlocks: 在 rehype-highlight 之后运行
 * 为代码块添加头部（语言标签 + 复制按钮）
 */
function rehypeCodeBlocks() {
    return (tree: any) => {
        visit(tree, 'element', (node: any) => {
            if (node.tagName !== 'pre') return
            // 跳过 mermaid 块（已被 rehypeMermaidPre 转换为 div）
            if (node.properties?.className?.includes('mermaid')) return

            const codeChild = node.children?.find(
                (child: any) => child.type === 'element' && child.tagName === 'code'
            )
            if (!codeChild) return

            // 提取语言
            const classes: string[] = codeChild.properties?.className || []
            const langClass = classes.find(
                (c: any) => typeof c === 'string' && c.startsWith('language-')
            )
            const lang = langClass ? langClass.replace('language-', '') : ''

            // 提取原始代码（用于复制按钮）
            const rawCode = hastToString(codeChild)

            // 创建代码头部
            // 注意：不使用 data-code 属性存储代码（属性值中的 HTML 特殊字符可能导致问题）
            // 复制按钮通过 codeCopy.ts 从相邻的 <code> 元素提取文本
            const header = {
                type: 'element',
                tagName: 'div',
                properties: { className: ['code-header'] },
                children: [
                    {
                        type: 'element',
                        tagName: 'span',
                        properties: { className: ['code-lang'] },
                        children: [{ type: 'text', value: lang || '' }],
                    },
                    {
                        type: 'element',
                        tagName: 'button',
                        properties: {
                            className: ['code-copy-btn'],
                        },
                        children: [{ type: 'text', value: '复制' }],
                    },
                ],
            }

            // 在 code 元素前插入头部
            node.children = [header, codeChild]

            // 给 pre 添加 hljs 类
            if (!node.properties) node.properties = {}
            if (!node.properties.className) node.properties.className = []
            if (!node.properties.className.includes('hljs')) {
                node.properties.className.push('hljs')
            }
        })
    }
}

// ===== Processor 工厂 =====

/** 共享 processor 实例：避免每次渲染都重建 remark 管道（plugin 链、AST 缓存、microtask 注册等） */
let sharedProcessor: ReturnType<typeof createProcessor> | null = null

/** 创建 remark + rehype 处理管道（与 llama.cpp webui 一致） */
function createProcessor() {
    return remark()
        .use(remarkGfm)           // GitHub Flavored Markdown
        .use(remarkMath)          // 解析 $inline$ 和 $$block$$ 数学公式
        .use(remarkBreaks)        // 换行转 <br>
        .use(remarkRehype)        // Markdown AST → Hast
        .use(rehypeKatex)         // 数学公式 → KaTeX HTML
        .use(rehypeMermaidPre)    // mermaid 代码块 → <div class="mermaid">
        .use(rehypeHighlight, {   // 代码语法高亮
            languages: lowlightAll,
        })
        .use(rehypeCodeBlocks)    // 代码块添加头部和复制按钮
        .use(rehypeExternalLinks) // 外部链接新窗口
        .use(rehypeStringify, { allowDangerousHtml: true })  // 输出 HTML
}

/** 获取共享 processor（首次调用时创建） */
function getProcessor() {
    if (!sharedProcessor) {
        sharedProcessor = createProcessor()
    }
    return sharedProcessor
}

// ===== 核心渲染函数 =====

/** 处理 Markdown 为 HTML（内部函数） */
async function processMarkdown(content: string): Promise<string> {
    const normalized = preprocessLaTeX(content)
    const result = await getProcessor().process(normalized)
    return String(result)
}

/**
 * 渲染完整 Markdown（异步）
 * 用于：历史消息、思考内容等已完成的文本
 */
export async function renderMarkdown(content: string): Promise<string> {
    try {
        return sanitizeHtml(await processMarkdown(content))
    } catch (_) {
        // 降级：转义 HTML 并返回
        return sanitizeHtml(escapeHtml(content))
    }
}

/**
 * 渲染流式 Markdown（异步）
 * 用于：模型正在生成的文本
 * 与 renderMarkdown 使用相同的管道，只是语义上区分
 */
export async function renderMarkdownStreaming(content: string): Promise<string> {
    try {
        return sanitizeHtml(await processMarkdown(content))
    } catch (_) {
        return sanitizeHtml(escapeHtml(content))
    }
}

// ===== 工具函数 =====

/** HTML 转义 */
function escapeHtml(str: string): string {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;')
}

/** DOMPurify 安全过滤 */
export function sanitizeHtml(html: string): string {
    return purify.sanitize(html, {
        ADD_TAGS: ['math', 'semantics', 'mrow', 'mi', 'mo', 'mn', 'msup', 'msub', 'mfrac', 'msqrt', 'mroot', 'munder', 'mover', 'munderover', 'mtable', 'mtr', 'mtd', 'mtext', 'mspace', 'mpadded', 'mphantom', 'mfenced', 'menclose', 'mstyle', 'merror', 'annotation', 'mglyph', 'mlabeledtr', 'mlongdiv', 'mscarries', 'mscarry', 'msgroup', 'msline', 'msrow', 'mstack', 'maction'],
        ADD_ATTR: ['mathvariant', 'mathsize', 'mathcolor', 'mathbackground', 'displaystyle', 'scriptlevel', 'linethickness', 'lspace', 'rspace', 'stretchy', 'symmetric', 'largeop', 'movablelimits', 'accent', 'accentunder', 'bevelled', 'close', 'open', 'separators', 'notation', 'subscriptshift', 'superscriptshift', 'align', 'columnalign', 'rowalign', 'equalcolumns', 'equalrows', 'columnspacing', 'rowspacing', 'columnlines', 'rowlines', 'frame', 'framespacing', 'groupalign', 'scope', 'encoding', 'class', 'target', 'rel'],
    })
}

/**
 * 专门用于消毒 Mermaid 生成的 SVG
 * 允许 SVG 绘图所需标签和属性，但禁止 script/foreignObject/use 等可能引入脚本的内容
 */
export function sanitizeMermaidSvg(html: string): string {
    return purify.sanitize(html, {
        ADD_TAGS: [
            'svg', 'g', 'defs', 'marker', 'pattern', 'clipPath', 'mask',
            'linearGradient', 'radialGradient', 'stop',
            'rect', 'circle', 'ellipse', 'line', 'polyline', 'polygon', 'path',
            'text', 'tspan', 'textPath',
            'image', 'title', 'desc',
        ],
        ADD_ATTR: [
            'viewBox', 'width', 'height', 'xmlns', 'xmlns:xlink', 'version',
            'x', 'y', 'x1', 'y1', 'x2', 'y2', 'cx', 'cy', 'r', 'rx', 'ry',
            'd', 'points', 'fill', 'stroke', 'stroke-width', 'stroke-linecap', 'stroke-linejoin',
            'stroke-dasharray', 'stroke-dashoffset', 'opacity', 'transform',
            'font-family', 'font-size', 'font-weight', 'text-anchor', 'dominant-baseline',
            'marker-start', 'marker-end', 'marker-mid', 'markerWidth', 'markerHeight',
            'refX', 'refY', 'orient', 'gradientUnits', 'gradientTransform',
            'offset', 'stop-color', 'stop-opacity',
            'clip-path', 'mask', 'filter',
            'class', 'id', 'style',
        ],
        // 明确禁止 foreignObject、script、use 等危险 SVG 子元素
        FORBID_TAGS: ['script', 'foreignObject', 'use', 'audio', 'video', 'iframe', 'embed', 'object'],
        FORBID_ATTR: ['onerror', 'onload', 'onclick', 'onmouseover', 'href', 'xlink:href'],
    })
}
