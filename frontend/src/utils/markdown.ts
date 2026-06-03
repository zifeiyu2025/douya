import MarkdownIt from 'markdown-it'
import mk from '@traptitech/markdown-it-katex'
import hljs from 'highlight.js'
import mermaid from 'mermaid'
import DOMPurify from 'dompurify'

// PERF-009: 代码高亮 LRU 缓存，避免流式渲染时重复高亮同一代码块
const highlightCache = new Map<string, string>()
const HIGHLIGHT_CACHE_MAX = 128

function getCachedHighlight(code: string, lang: string): string | null {
    const key = `${lang}\x00${code}`
    const cached = highlightCache.get(key)
    if (cached !== undefined) {
        highlightCache.delete(key)
        highlightCache.set(key, cached)
        return cached
    }
    return null
}

function setCachedHighlight(code: string, lang: string, result: string): void {
    const key = `${lang}\x00${code}`
    if (highlightCache.size >= HIGHLIGHT_CACHE_MAX) {
        const firstKey = highlightCache.keys().next().value
        if (firstKey !== undefined) highlightCache.delete(firstKey)
    }
    highlightCache.set(key, result)
}

const md = new MarkdownIt({
    html: false,
    linkify: true,
    typographer: true,
    highlight(str: string, lang: string): string {
        if (lang === 'mermaid') {
            return `<div class="mermaid">${str}</div>`
        }
        const escapedCode = md.utils.escapeHtml(str)
        const langLabel = lang && hljs.getLanguage(lang) ? lang : (lang || '')
        const header = `<div class="code-header">${langLabel ? `<span class="code-lang">${langLabel}</span>` : '<span class="code-lang"></span>'}<button class="code-copy-btn" data-code="${escapedCode.replace(/"/g, '&quot;')}">复制</button></div>`
        if (lang && hljs.getLanguage(lang)) {
            const cached = getCachedHighlight(str, lang)
            if (cached) {
                return `<pre class="hljs">${header}<code>${cached}</code></pre>`
            }
            try {
                const highlighted = hljs.highlight(str, { language: lang, ignoreIllegals: true }).value
                setCachedHighlight(str, lang, highlighted)
                return `<pre class="hljs">${header}<code>${highlighted}</code></pre>`
            } catch (_) { /* empty */ }
        }
        return `<pre class="hljs">${header}<code>${escapedCode}</code></pre>`
    },
})

md.use(mk, {
    throwOnError: false,
    errorColor: '#cc0000',
    strict: 'warn',
    maxSize: 500,
    maxExpand: 5000,
    minRuleThickness: 0.05,
    displayMode: false,
    output: 'htmlAndMathml',
    trust: (context: any) => ['\\url', '\\href', '_relative'].includes(context.protocol || context.command),
    macros: {
        '\\R': '\\mathbb{R}',
        '\\N': '\\mathbb{N}',
        '\\Z': '\\mathbb{Z}',
        '\\Q': '\\mathbb{Q}',
        '\\C': '\\mathbb{C}',
    },
})

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

function stripIncompleteMath(text: string): string {
    let i = 0
    const len = text.length
    let inCodeBlock = false
    let inInlineCode = false
    let inMath = false
    let mathStart = -1
    let isDisplayMath = false

    while (i < len) {
        const ch = text[i]
        const next = i + 1 < len ? text[i + 1] : ''

        if (!inInlineCode && ch === '`' && text.substring(i, i + 3) === '```') {
            inCodeBlock = !inCodeBlock
            i += 3
            continue
        }

        if (!inCodeBlock && ch === '`') {
            inInlineCode = !inInlineCode
            i++
            continue
        }

        if (inCodeBlock || inInlineCode) {
            i++
            continue
        }

        if (ch === '\\' && next === '$') {
            i += 2
            continue
        }

        if (ch === '$' && next === '$') {
            if (!inMath) {
                inMath = true
                isDisplayMath = true
                mathStart = i
            } else if (isDisplayMath) {
                inMath = false
            }
            i += 2
            continue
        }

        if (ch === '$') {
            if (!inMath) {
                inMath = true
                isDisplayMath = false
                mathStart = i
            } else if (!isDisplayMath) {
                inMath = false
            }
            i++
            continue
        }

        i++
    }

    if (inMath && mathStart >= 0) {
        return text.substring(0, mathStart)
    }

    return text
}

export function renderMarkdownStreaming(content: string): string {
    const safeContent = stripIncompleteMath(content)
    return sanitizeHtml(renderMarkdownRaw(safeContent))
}

function renderMarkdownRaw(content: string): string {
    try {
        return md.render(content)
    } catch (_) {
        return md.utils.escapeHtml(content)
    }
}

export function renderMarkdown(content: string): string {
    return sanitizeHtml(renderMarkdownRaw(content))
}

export function sanitizeHtml(html: string): string {
    return DOMPurify.sanitize(html, {
        ADD_TAGS: ['math', 'semantics', 'mrow', 'mi', 'mo', 'mn', 'msup', 'msub', 'mfrac', 'msqrt', 'mroot', 'munder', 'mover', 'munderover', 'mtable', 'mtr', 'mtd', 'mtext', 'mspace', 'mpadded', 'mphantom', 'mfenced', 'menclose', 'mstyle', 'merror', 'annotation', 'mglyph', 'mlabeledtr', 'mlongdiv', 'mscarries', 'mscarry', 'msgroup', 'msline', 'msrow', 'mstack', 'maction'],
        ADD_ATTR: ['mathvariant', 'mathsize', 'mathcolor', 'mathbackground', 'displaystyle', 'scriptlevel', 'linethickness', 'lspace', 'rspace', 'stretchy', 'symmetric', 'largeop', 'movablelimits', 'accent', 'accentunder', 'bevelled', 'close', 'open', 'separators', 'notation', 'subscriptshift', 'superscriptshift', 'align', 'columnalign', 'rowalign', 'equalcolumns', 'equalrows', 'columnspacing', 'rowspacing', 'columnlines', 'rowlines', 'frame', 'framespacing', 'groupalign', 'scope', 'encoding', 'data-code', 'class'],
    })
}

export function renderMarkdownInline(content: string): string {
    try {
        return sanitizeHtml(md.renderInline(content))
    } catch (_) {
        return md.utils.escapeHtml(content)
    }
}
