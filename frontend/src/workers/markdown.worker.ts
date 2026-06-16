/// <reference lib="webworker" />
/**
 * Markdown 渲染 Worker
 *
 * 负责：把 Markdown 文本渲染成 HTML
 * 输入：{ id, content }
 * 输出：{ id, html } 或 { id, error }
 *
 * 收益：流式 token 高频触发渲染时，主线程不阻塞。
 * sanitize 用 lightSanitize（Worker 内无 DOM，DOMPurify 不可用）。
 */
import { remark } from 'remark'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'
import remarkBreaks from 'remark-breaks'
import remarkRehype from 'remark-rehype'
import rehypeKatex from 'rehype-katex'
import rehypeHighlight from 'rehype-highlight'
import rehypeStringify from 'rehype-stringify'
import { all as lowlightAll } from 'lowlight'
import { visit } from 'unist-util-visit'
import { preprocessLaTeX } from '../utils/latex-protection'
import { lightSanitize } from '../utils/lightSanitize'

// 工厂函数：每个 worker 创建一个 processor（worker 复用，复用 processor 更省内存）
function createProcessor() {
    return remark()
        .use(remarkGfm)
        .use(remarkMath)
        .use(remarkBreaks)
        .use(remarkRehype)
        .use(rehypeKatex)
        // mermaid 代码块转换为 <div class="mermaid">
        .use(() => (tree: any) => {
            visit(tree, 'element', (node: any) => {
                if (node.tagName !== 'pre') return
                const codeChild = node.children?.find(
                    (c: any) => c.type === 'element' && c.tagName === 'code'
                )
                if (!codeChild) return
                const classes = codeChild.properties?.className
                if (!Array.isArray(classes)) return
                const hasMermaid = classes.some(
                    (c: any) => typeof c === 'string' && c === 'language-mermaid'
                )
                if (!hasMermaid) return
                // 提取代码文本
                const extractText = (n: any): string => {
                    if (n.type === 'text') return n.value || ''
                    if (Array.isArray(n.children)) return n.children.map(extractText).join('')
                    return ''
                }
                const rawCode = extractText(codeChild)
                node.tagName = 'div'
                node.properties = { className: ['mermaid'] }
                node.children = [{ type: 'text', value: rawCode }]
            })
        })
        .use(rehypeHighlight, { languages: lowlightAll })
        // 外部链接新窗口
        .use(() => (tree: any) => {
            visit(tree, 'element', (node: any) => {
                if (node.tagName !== 'a') return
                const href = node.properties?.href
                if (typeof href === 'string' && (href.startsWith('http://') || href.startsWith('https://'))) {
                    node.properties.target = '_blank'
                    node.properties.rel = 'noopener noreferrer'
                }
            })
        })
        .use(rehypeStringify, { allowDangerousHtml: true })
}

const processor = createProcessor()

interface RenderRequest {
    id: number
    content: string
}

self.addEventListener('message', async (e: MessageEvent<RenderRequest>) => {
    const { id, content } = e.data
    try {
        const normalized = preprocessLaTeX(content)
        const result = await processor.process(normalized)
        const html = lightSanitize(String(result))
        ;(self as any).postMessage({ id, html })
    } catch (err) {
        ;(self as any).postMessage({
            id,
            html: '',
            error: err instanceof Error ? err.message : String(err),
        })
    }
})

export {}
