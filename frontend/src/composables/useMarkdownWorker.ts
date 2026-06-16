/**
 * 基于 Web Worker 的 Markdown 渲染 composable
 *
 * 用法：
 * ```ts
 * const { rendered, bind, clear } = useMarkdownWorker()
 * bind(() => streamingContent.value)
 * ```
 *
 * 特性：
 * - 内部维护任务 ID 防过期（流式场景下旧任务结果会被新任务覆盖）
 * - 复用 Worker 实例（避免每次渲染都创建 Worker）
 * - onUnmounted 自动终止 Worker
 * - 降级策略：Worker 创建失败时回退到主线程渲染
 */
import { ref, watch, onUnmounted, type Ref } from 'vue'
import { renderMarkdownStreaming } from '../utils/markdown'

// ?worker 后缀让 Vite 把它打包成独立 worker chunk
// @ts-ignore - Vite worker import
import MarkdownWorker from '../workers/markdown.worker?worker'

export function useMarkdownWorker() {
    const rendered = ref('')
    let worker: Worker | null = null
    let workerFailed = false
    let currentId = 0
    let pendingContent = ''
    let renderTimer: ReturnType<typeof setTimeout> | null = null
    let lastRenderTime = 0
    let isRendering = false

    function ensureWorker(): Worker | null {
        if (workerFailed) return null
        if (worker) return worker
        try {
            worker = new MarkdownWorker()
            worker.addEventListener('message', (e: MessageEvent) => {
                const { id, html, error } = e.data
                // 只接受最新任务的结果（防止过期渲染覆盖）
                if (id !== currentId) return
                isRendering = false
                if (error) {
                    console.warn('[markdown-worker] render error:', error)
                }
                rendered.value = html
                lastRenderTime = Date.now()
                // 队列中还有内容则继续
                if (pendingContent) {
                    const next = pendingContent
                    pendingContent = ''
                    scheduleRender(next)
                }
            })
            worker.addEventListener('error', (e) => {
                console.error('[markdown-worker] error:', e)
                workerFailed = true
                isRendering = false
            })
            return worker
        } catch (e) {
            console.warn('[markdown-worker] failed to create, fallback to main thread:', e)
            workerFailed = true
            return null
        }
    }

    async function doRender(content: string) {
        const w = ensureWorker()
        if (w) {
            isRendering = true
            currentId++
            w.postMessage({ id: currentId, content })
        } else {
            // 降级：主线程渲染
            rendered.value = await renderMarkdownStreaming(content)
            lastRenderTime = Date.now()
        }
    }

    /**
     * 动态节流：内容越长，渲染间隔越大
     * 短内容（<2KB）50ms，长内容（>10KB）200ms
     */
    function scheduleRender(content: string) {
        if (isRendering) {
            pendingContent = content
            return
        }
        const len = content.length
        const interval = len > 10000 ? 200 : len > 2000 ? 100 : 50
        const elapsed = Date.now() - lastRenderTime
        const remaining = interval - elapsed
        if (remaining <= 0) {
            if (renderTimer) { clearTimeout(renderTimer); renderTimer = null }
            doRender(content)
        } else if (!renderTimer) {
            renderTimer = setTimeout(() => {
                renderTimer = null
                doRender(pendingContent || content)
            }, remaining)
        }
    }

    function clear() {
        if (renderTimer) { clearTimeout(renderTimer); renderTimer = null }
        rendered.value = ''
        pendingContent = ''
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

    onUnmounted(() => {
        if (renderTimer) clearTimeout(renderTimer)
        if (worker) {
            worker.terminate()
            worker = null
        }
    })

    return {
        rendered: rendered as Ref<string>,
        bind,
        clear,
    }
}
