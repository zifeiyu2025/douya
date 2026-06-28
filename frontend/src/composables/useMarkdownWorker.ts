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
import { renderMarkdownStreaming, sanitizeHtml, renderStreamingLight } from '../utils/markdown'
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
    let renderTimer: ReturnType<typeof setTimeout> | null = null  // setTimeout 调度用
    let lastRenderTime = 0
    let isRendering = false
    // 稳定块缓存：stable 命中时只渲染 unstable
    let lastStable = ''
    let lastStableHtml = ''
    // 双模式渲染：流式期间用轻量同步渲染（跳过 Worker + DOMPurify），结束后全量重渲染
    let isStreamingMode = false
    // 最后一次渲染的完整内容（退出流式模式时用于触发全量重渲染）
    let lastFullContent = ''
    // pending Promise 的 reject 函数列表：worker error/terminate 时拒绝所有挂起的 Promise，避免永久挂起
    const pendingRejects: Array<(error: Error) => void> = []
    // doRenderLight 完成后立即处理 pendingContent 的定时器
    let pendingTimer: ReturnType<typeof setTimeout> | null = null

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
     * 流式期间的轻量渲染（主线程同步，跳过 Worker 往返和 DOMPurify）
     * - 只渲染 unstable 增量段，stable 复用缓存
     * - 用 lightSanitize（正则消毒）替代 DOMPurify
     * - 单次渲染 < 10ms，避免 Worker 异步往返延迟
     * - 退出流式模式时会触发 doFinalRender 全量重渲染 + DOMPurify 消毒
     */
    async function doRenderLight(content: string) {
        isRendering = true
        try {
            const { stable, unstable } = splitStableUnstable(content)
            let stableHtml: string
            if (stable === lastStable && lastStableHtml) {
                // stable 命中缓存：直接复用
                stableHtml = lastStableHtml
            } else if (stable) {
                // stable 不命中：轻量渲染后用 sanitizeHtml（DOMPurify）完整消毒并缓存
                // renderStreamingLight 内部已用 lightSanitize 做第一层正则消毒，此处做第二层 DOMPurify 消毒
                stableHtml = sanitizeHtml(await renderStreamingLight(stable))
                lastStable = stable
                lastStableHtml = stableHtml
            } else {
                stableHtml = ''
                lastStable = ''
                lastStableHtml = ''
            }
            // 渲染 unstable（可能为空），用 sanitizeHtml（DOMPurify）完整消毒
            // 关闭流式期间的 XSS 暴露窗口：unstable 增量段也经过 DOMPurify，而非仅 lightSanitize
            const unstableHtml = unstable ? sanitizeHtml(await renderStreamingLight(unstable)) : ''
            rendered.value = stableHtml + unstableHtml
            lastRenderTime = Date.now()
            lastFullContent = content
        } catch (err) {
            // 降级：主线程全量渲染
            console.warn('[markdown-worker] light render failed, fallback:', err)
            rendered.value = await renderMarkdownStreaming(content)
            lastRenderTime = Date.now()
            lastFullContent = content
        } finally {
            isRendering = false
            // 队列中还有内容则立即处理（setTimeout(0) 让出主线程，不走 scheduleRender 节流）
            if (pendingContent) {
                const next = pendingContent
                pendingContent = ''
                if (pendingTimer) clearTimeout(pendingTimer)
                pendingTimer = setTimeout(() => {
                    pendingTimer = null
                    doRenderLight(next)
                }, 0)
            }
        }
    }

    /**
     * 流式结束后的全量重渲染（Worker + DOMPurify）
     * - 用 Worker 渲染完整内容，DOMPurify 二次消毒
     * - 更新 stable 缓存为 Worker 结果（保证最终结果与历史消息渲染一致）
     */
    async function doFinalRender(content: string) {
        isRendering = true
        try {
            const w = ensureWorker()
            let html: string
            if (w) {
                try {
                    html = await renderWithWorker(w, content)
                    // Worker 中使用 lightSanitize，主线程再用 DOMPurify 二次消毒
                    html = sanitizeHtml(html)
                } catch (err) {
                    // Worker 渲染失败：降级到主线程
                    console.warn('[markdown-worker] final render worker failed, fallback:', err)
                    html = await renderMarkdownStreaming(content)
                }
            } else {
                // Worker 不可用：主线程全量渲染
                html = await renderMarkdownStreaming(content)
            }
            rendered.value = html
            lastRenderTime = Date.now()
            lastFullContent = content
            // 更新 stable 缓存为最终结果，避免下次流式启动时缓存不一致
            // 缓存经过 sanitizeHtml 消毒的结果，确保 doRenderLight 命中缓存时也是安全的
            const { stable } = splitStableUnstable(content)
            if (stable) {
                lastStable = stable
                lastStableHtml = sanitizeHtml(await renderStreamingLight(stable))
            } else {
                lastStable = ''
                lastStableHtml = ''
            }
        } finally {
            isRendering = false
        }
    }

    /**
     * 进入/退出流式模式
     * - 进入：isStreamingMode = true，scheduleRender 改用 doRenderLight + 激进间隔
     * - 退出：触发一次 doFinalRender 全量重渲染（Worker + DOMPurify），保证最终结果安全
     */
    function setStreamingMode(streaming: boolean) {
        if (isStreamingMode === streaming) return
        isStreamingMode = streaming
        if (!streaming) {
            // 退出流式模式：用最后一次内容触发全量重渲染
            const content = pendingContent || lastFullContent
            pendingContent = ''
            if (content && !isRendering) {
                doFinalRender(content)
            }
        }
    }

    /**
     * 动态节流：内容越长，渲染间隔越大
     * 流式模式：短<2KB 0ms（立即）/ 中2-10KB 8ms / 长>10KB 16ms（无 Worker 往返，可更激进）
     * 非流式模式：短<2KB 50ms / 中2-10KB 80ms / 长>10KB 120ms（Worker 往返需要更多时间）
     * 用单层 setTimeout 合并同帧多次到达（setTimeout 本身就能合并：每次 clearTimeout 旧定时器）
     */
    function scheduleRender(content: string) {
        if (isRendering) {
            pendingContent = content
            return
        }
        // 用 setTimeout 合并同帧多次到达（替代 RAF + setTimeout 双重节流）
        if (renderTimer) { clearTimeout(renderTimer); renderTimer = null }
        pendingContent = content
        const len = pendingContent.length
        const interval = isStreamingMode
            ? (len > 10000 ? 16 : len > 2000 ? 8 : 0)
            : (len > 10000 ? 120 : len > 2000 ? 80 : 50)
        const elapsed = Date.now() - lastRenderTime
        const remaining = Math.max(0, interval - elapsed)
        renderTimer = setTimeout(() => {
            renderTimer = null
            const next = pendingContent
            pendingContent = ''
            isStreamingMode ? doRenderLight(next) : doRender(next)
        }, remaining)
    }

    function clear() {
        if (renderTimer) { clearTimeout(renderTimer); renderTimer = null }
        if (pendingTimer) { clearTimeout(pendingTimer); pendingTimer = null }
        rendered.value = ''
        pendingContent = ''
        lastStable = ''
        lastStableHtml = ''
        lastFullContent = ''
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
        if (pendingTimer) clearTimeout(pendingTimer)
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
        setStreamingMode,
    }
}
