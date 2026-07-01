/**
 * Markdown 渲染 composable（同步流式 + 最终全量渲染）
 *
 * 用法：
 * ```ts
 * const { rendered, bind, clear, finalizeRender } = useMarkdownWorker()
 * bind(() => streamingContent.value)
 * // 流式结束时调用：
 * finalizeRender()
 * ```
 *
 * 核心设计（解决"蹦字"问题）：
 * 1. 流式期间用纯同步渲染（renderStreamingSync），无 async/await，每帧立即出 DOM
 *    - 之前用 await processMarkdown(remark异步)，isRendering=true 时多帧内容被缓冲
 *    - 现在同步执行，无缓冲，每帧内容都在同一帧内渲染到 DOM
 * 2. 同步渲染只做基础格式（段落、换行、行内代码/粗体/斜体），性能极高
 * 3. 流式结束 finalizeRender() 用完整 renderMarkdown(remark+DOMPurify) 做最终渲染
 *    - 此时文本已完整，复杂格式（代码块、表格、列表、公式）正确解析
 *    - 最终替换为完整 HTML，视觉上无缝（内容一致，只是格式更丰富）
 */
import { ref, watch, onScopeDispose, type Ref } from 'vue'
import { renderStreamingSync } from '../utils/streamingRender'
import { renderMarkdown, escapeHtml } from '../utils/markdown'
import { splitStableUnstable } from '../utils/markdownStreaming'

export function useMarkdownWorker() {
    const rendered = ref('')
    let latestContent = ''
    let rafId: number | null = null
    let lastStable = ''
    let lastStableHtml = ''
    let isFinalized = false

    function doRenderSync(content: string) {
        try {
            const { stable, unstable } = splitStableUnstable(content)
            let stableHtml: string
            if (stable === lastStable && lastStableHtml) {
                stableHtml = lastStableHtml
            } else if (stable) {
                stableHtml = renderStreamingSync(stable)
                lastStable = stable
                lastStableHtml = stableHtml
            } else {
                stableHtml = ''
                lastStable = ''
                lastStableHtml = ''
            }
            const unstableHtml = unstable ? renderStreamingSync(unstable) : ''
            rendered.value = stableHtml + unstableHtml
        } catch (err) {
            console.warn('[markdown-worker] sync render failed, fallback:', err)
            rendered.value = escapeHtml(content)
        }
    }

    async function doFinalRender(content: string) {
        try {
            const html = await renderMarkdown(content)
            rendered.value = html
            lastStable = ''
            lastStableHtml = ''
        } catch (err) {
            console.warn('[markdown-worker] final render failed:', err)
        }
        isFinalized = false
    }

    function finalizeRender() {
        if (rafId !== null) { cancelAnimationFrame(rafId); rafId = null }
        isFinalized = true
        lastStable = ''
        lastStableHtml = ''
        return doFinalRender(latestContent)
    }

    function scheduleRender(content: string) {
        latestContent = content
        if (isFinalized) return
        // 已有 RAF 调度时不重复设置——只更新 latestContent
        // RAF 回调会读取最新的 latestContent，天然合并同帧多次更新
        if (rafId !== null) return
        rafId = requestAnimationFrame(() => {
            rafId = null
            doRenderSync(latestContent)
        })
    }

    function clear() {
        if (rafId !== null) { cancelAnimationFrame(rafId); rafId = null }
        isFinalized = false
        rendered.value = ''
        latestContent = ''
        lastStable = ''
        lastStableHtml = ''
    }

    function bind(getContent: () => string) {
        watch(getContent, (newContent) => {
            if (!newContent) {
                clear()
                return
            }
            scheduleRender(newContent)
        }, { immediate: true })
    }

    onScopeDispose(() => {
        if (rafId !== null) cancelAnimationFrame(rafId)
    })

    return {
        rendered: rendered as Ref<string>,
        bind,
        clear,
        finalizeRender,
    }
}
