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
 * - onScopeDispose 自动终止 Worker
 * - 降级策略：Worker 创建失败时回退到主线程渲染
 * - 稳定/不稳定块拆分：stable 命中缓存时只渲染 unstable，减少 Worker 负载
 */
import { ref, watch, onScopeDispose, type Ref } from 'vue'
import { renderMarkdownStreaming, sanitizeHtml } from '../utils/markdown'
import { splitStableUnstable } from '../utils/markdownStreaming'

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
    // 稳定块缓存：stable 命中时只渲染 unstable
    let lastStable = ''
    let lastStableHtml = ''
    // pending Promise 的 reject 函数列表：worker error/terminate 时拒绝所有挂起的 Promise，避免永久挂起
    const pendingRejects: Array<(error: Error) => void> = []

    function ensureWorker(): Worker | null {
        if (workerFailed) return null
        if (worker) return worker
        try {
            worker = new MarkdownWorker()
            worker.addEventListener('error', (e) => {
                console.error('[markdown-worker] error:', e)
                workerFailed = true
                isRendering = false
                // reject 所有 pending Promise，避免永久挂起
                const err = new Error(`Markdown worker error: ${e.message}`)
                while (pendingRejects.length > 0) {
                    const reject = pendingRejects.shift()!
                    reject(err)
                }
            })
            return worker
        } catch (e) {
            console.warn('[markdown-worker] failed to create, fallback to main thread:', e)
            workerFailed = true
            return null
        }
    }

    /**
     * 用 Worker 渲染单次内容（Promise 包装）
     * 返回渲染后的 HTML，自动处理过期任务
     */
    function renderWithWorker(w: Worker, content: string): Promise<string> {
        return new Promise<string>((resolve, reject) => {
            currentId++
            const id = currentId
            // 将 reject 存入列表，用于 worker error/terminate 时拒绝
            pendingRejects.push(reject)
            const handler = (e: MessageEvent) => {
                if (e.data.id !== id) return
                w.removeEventListener('message', handler)
                // 渲染完成，从列表中移除该 reject
                const idx = pendingRejects.indexOf(reject)
                if (idx >= 0) pendingRejects.splice(idx, 1)
                if (e.data.error) {
                    console.warn('[markdown-worker] render error:', e.data.error)
                }
                resolve(e.data.html || '')
            }
            w.addEventListener('message', handler)
            w.postMessage({ id, content })
        })
    }

    async function doRender(content: string) {
        const w = ensureWorker()
        if (!w) {
            // 降级：主线程渲染
            rendered.value = await renderMarkdownStreaming(content)
            lastRenderTime = Date.now()
            isRendering = false
            // 队列中还有内容则继续
            if (pendingContent) {
                const next = pendingContent
                pendingContent = ''
                scheduleRender(next)
            }
            return
        }

        isRendering = true
        const { stable, unstable } = splitStableUnstable(content)

        let stableHtml: string
        if (stable === lastStable && lastStableHtml) {
            // stable 命中缓存：直接复用
            stableHtml = lastStableHtml
        } else if (stable) {
            // stable 不命中：渲染 stable 并缓存
            try {
                stableHtml = await renderWithWorker(w, stable)
            } catch (err) {
                // 降级：主线程渲染
                console.warn('[markdown-worker] stable render failed, fallback:', err)
                stableHtml = await renderMarkdownStreaming(stable)
            }
            lastStable = stable
            lastStableHtml = stableHtml
        } else {
            // 无 stable
            stableHtml = ''
            lastStable = ''
            lastStableHtml = ''
        }

        // 渲染 unstable（可能为空）
        let unstableHtml = ''
        if (unstable) {
            try {
                unstableHtml = await renderWithWorker(w, unstable)
            } catch (err) {
                // 降级：主线程渲染
                console.warn('[markdown-worker] unstable render failed, fallback:', err)
                unstableHtml = await renderMarkdownStreaming(unstable)
            }
        }
        // Worker 中使用的是轻量级 lightSanitize，主线程再用 DOMPurify 进行二次消毒
        rendered.value = sanitizeHtml(stableHtml + unstableHtml)
        lastRenderTime = Date.now()
        isRendering = false

        // 队列中还有内容则继续
        if (pendingContent) {
            const next = pendingContent
            pendingContent = ''
            scheduleRender(next)
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
        if (renderTimer) clearTimeout(renderTimer)
        // 先 reject 所有 pending Promise，避免 terminate 后 Promise 永久挂起
        const err = new Error('Markdown worker disposed')
        while (pendingRejects.length > 0) {
            const reject = pendingRejects.shift()!
            reject(err)
        }
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
