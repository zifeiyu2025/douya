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
import { preprocessLaTeX } from '../utils/latex-protection'
import { lightSanitize } from '../utils/lightSanitize'
import { rehypeMermaidPre, rehypeExternalLinks } from '../utils/rehypePlugins'

// 工厂函数：每个 worker 创建一个 processor（worker 复用，复用 processor 更省内存）
function createProcessor() {
    return remark()
        .use(remarkGfm)
        .use(remarkMath)
        .use(remarkBreaks)
        .use(remarkRehype)
        .use(rehypeKatex)
        // mermaid 代码块转换为 <div class="mermaid">
        .use(rehypeMermaidPre)
        .use(rehypeHighlight, { languages: lowlightAll })
        // 外部链接新窗口
        .use(rehypeExternalLinks)
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
