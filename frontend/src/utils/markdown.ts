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
import mermaid from 'mermaid'
import { preprocessLaTeX } from './latex-protection'

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

// ===== Mermaid 初始化 =====

mermaid.initialize({
    startOnLoad: false,
    theme: 'default',
    securityLevel: 'strict',
})

let mermaidCounter = 0

export async function renderMermaidInElement(el: HTMLElement) {
    const mermaidEls = el.querySelectorAll('.mermaid')
    const elements = Array.from(mermaidEls) as HTMLElement[]
    for (const mermaidEl of elements) {
        const id = `mermaid-${++mermaidCounter}`
        try {
            const { svg } = await mermaid.render(id, (mermaidEl as HTMLElement).textContent || '')
            mermaidEl.innerHTML = sanitizeHtml(svg)
        } catch (_) { /* empty */ }
    }
}

// ===== Hast 辅助函数 =====

/** 从 hast 节点提取纯文本内容 */
function hastToString(node: any): string {
    if (node.type === 'text') return node.value || ''
    if (Array.isArray(node.children)) {
        return node.children.map(hastToString).join('')
    }
    return ''
}

// ===== 自定义 rehype 插件 =====

/**
 * rehypeMermaidPre: 在 rehype-highlight 之前运行
 * 将 <pre><code class="language-mermaid"> 转换为 <div class="mermaid">
 * 避免 rehype-highlight 尝试高亮 mermaid 语法
 */
function rehypeMermaidPre() {
    return (tree: any) => {
        visit(tree, 'element', (node: any) => {
            if (node.tagName !== 'pre') return

            const codeChild = node.children?.find(
                (child: any) => child.type === 'element' && child.tagName === 'code'
            )
            if (!codeChild) return

            const classes = codeChild.properties?.className
            if (!Array.isArray(classes)) return

            const hasMermaid = classes.some(
                (c: any) => typeof c === 'string' && c === 'language-mermaid'
            )
            if (!hasMermaid) return

            // 提取原始代码文本
            const rawCode = hastToString(codeChild)

            // 转换为 <div class="mermaid">
            node.tagName = 'div'
            node.properties = { className: ['mermaid'] }
            node.children = [{ type: 'text', value: rawCode }]
        })
    }
}

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

/**
 * rehypeExternalLinks: 外部链接在新窗口打开
 */
function rehypeExternalLinks() {
    return (tree: any) => {
        visit(tree, 'element', (node: any) => {
            if (node.tagName !== 'a') return
            const href = node.properties?.href
            if (typeof href === 'string' && (href.startsWith('http://') || href.startsWith('https://'))) {
                node.properties.target = '_blank'
                node.properties.rel = 'noopener noreferrer'
            }
        })
    }
}

// ===== Processor 工厂 =====

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

// ===== 核心渲染函数 =====

/** 处理 Markdown 为 HTML（内部函数） */
async function processMarkdown(content: string): Promise<string> {
    const normalized = preprocessLaTeX(content)
    const processor = createProcessor()
    const result = await processor.process(normalized)
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
