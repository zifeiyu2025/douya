/// <reference lib="webworker" />
/**
 * Markdown 渲染 Worker（对标千问官网流式渲染）
 *
 * 负责：把 Markdown 文本渲染成 HTML
 * 输入：{ id, content }
 * 输出：{ id, html } 或 { id, error }
 *
 * 优化：内部维护 stable 缓存，stable 命中时只渲染 unstable 增量，减少全量渲染开销
 * 收益：流式 token 高频触发渲染时，主线程不阻塞（processMarkdown 在 Worker 线程执行）
 * sanitize 用 lightSanitize（Worker 内无 DOM，DOMPurify 不可用）
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
import { rehypeCodeBlocks } from '../utils/markdown'
import { splitStableUnstable } from '../utils/markdownStreaming'

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
        // 代码块添加头部（语言标签 + 复制按钮）
        .use(rehypeCodeBlocks)
        .use(rehypeStringify, { allowDangerousHtml: true })
}

const processor = createProcessor()

/** 渲染单段内容（preprocess + process + lightSanitize） */
async function renderSegment(text: string): Promise<string> {
    const normalized = preprocessLaTeX(text)
    const result = await processor.process(normalized)
    return lightSanitize(String(result))
}

// stable 缓存：stable 命中时复用 lastStableHtml，避免重复渲染已完成段落
let lastStable = ''
let lastStableHtml = ''

interface RenderRequest {
    id: number
    content: string
}

self.addEventListener('message', async (e: MessageEvent<RenderRequest>) => {
    const { id, content } = e.data
    try {
        const { stable, unstable } = splitStableUnstable(content)
        let stableHtml: string
        if (stable === lastStable && lastStableHtml) {
            // stable 命中缓存：直接复用
            stableHtml = lastStableHtml
        } else if (stable) {
            // stable 不命中：渲染并缓存
            stableHtml = await renderSegment(stable)
            lastStable = stable
            lastStableHtml = stableHtml
        } else {
            stableHtml = ''
            lastStable = ''
            lastStableHtml = ''
        }
        const unstableHtml = unstable ? await renderSegment(unstable) : ''
        ;(self as any).postMessage({ id, html: stableHtml + unstableHtml })
    } catch (err) {
        ;(self as any).postMessage({
            id,
            html: '',
            error: err instanceof Error ? err.message : String(err),
        })
    }
})

export {}
