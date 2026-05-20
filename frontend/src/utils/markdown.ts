import MarkdownIt from 'markdown-it'
import mk from '@traptitech/markdown-it-katex'
import hljs from 'highlight.js'
import mermaid from 'mermaid'

const md = new MarkdownIt({
    html: false,
    linkify: true,
    typographer: true,
    highlight(str: string, lang: string): string {
        if (lang === 'mermaid') {
            return `<div class="mermaid">${str}</div>`
        }
        if (lang && hljs.getLanguage(lang)) {
            try {
                return `<pre class="hljs"><code>${hljs.highlight(str, { language: lang, ignoreIllegals: true }).value}</code></pre>`
            } catch (_) { /* empty */ }
        }
        return `<pre class="hljs"><code>${md.utils.escapeHtml(str)}</code></pre>`
    },
})

md.use(mk)

mermaid.initialize({
    startOnLoad: false,
    theme: 'default',
    securityLevel: 'loose',
})

let mermaidCounter = 0

export async function renderMermaidInElement(el: HTMLElement) {
    const mermaidEls = el.querySelectorAll('.mermaid')
    const elements = Array.from(mermaidEls) as HTMLElement[]
    for (const mermaidEl of elements) {
        const id = `mermaid-${++mermaidCounter}`
        try {
            const { svg } = await mermaid.render(id, (mermaidEl as HTMLElement).textContent || '')
            mermaidEl.innerHTML = svg
        } catch (_) { /* empty */ }
    }
}

export function renderMarkdown(content: string): string {
    try {
        return md.render(content)
    } catch (_) {
        return md.utils.escapeHtml(content)
    }
}

export function renderMarkdownInline(content: string): string {
    try {
        return md.renderInline(content)
    } catch (_) {
        return md.utils.escapeHtml(content)
    }
}
