/**
 * morphdom DOM Diff 流式渲染 composable（对标千问丝滑流式方案）
 *
 * 与 useMarkdownWorker 的核心区别：
 * - useMarkdownWorker：生成 HTML 字符串 → v-html 全量替换 innerHTML（O(N) DOM 操作）
 * - useMorphRender：生成 HTML 字符串 → morphdom DOM Diff 只更新增量节点（O(Δ) DOM 操作）
 *
 * 优势：
 * 1. 未变化节点保留同一引用（不销毁不重建），CSS 淡入动画可触发
 * 2. 新增节点通过 onBeforeNodeAdded 添加 stream-node-enter 类，触发淡入动画
 * 3. 长文本性能不衰减（DOM 操作量与增量成正比，而非与总量成正比）
 *
 * 用法：
 * ```ts
 * const { containerRef, bind, finalizeRender } = useMorphRender()
 * // 模板中：<div ref="streamingContainer" class="markdown-body streaming" />
 * bind(() => streamingContent.value)
 * finalizeRender()  // 流式结束时调用
 * ```
 */
import { ref, watch, onScopeDispose, type Ref } from 'vue'
import morphdom from 'morphdom'
import { renderStreamingSync } from '../utils/streamingRender'
import { renderMarkdown, escapeHtml } from '../utils/markdown'

export function useMorphRender() {
    const containerRef = ref<HTMLElement | null>(null)
    let latestContent = ''
    let rafId: number | null = null
    let isFinalized = false
    let isFirstRender = true
    // 控制是否给新增节点添加淡入类
    // finalizeRender 时为 false（最终渲染应平滑过渡，不触发淡入）
    let shouldAddEnterClass = true

    /**
     * 将 HTML 字符串应用到容器
     * - 容器为空（首次渲染或 v-if 重新挂载）：直接 innerHTML（避免全部淡入）
     * - 后续渲染：morphdom DOM Diff，新增节点添加淡入类
     *
     * 注意：容器由 v-if="streamingContent" 控制，streamingContent 清空再赋值时
     * Vue 会重新创建 DOM 元素，containerRef.value 指向新节点（childNodes.length=0），
     * 此时必须走 innerHTML 分支，否则 morphdom diff 空容器会丢失内容。
     */
    function applyHtml(html: string) {
        const el = containerRef.value
        if (!el) return

        if (isFirstRender || el.childNodes.length === 0) {
            // 容器为空：直接 innerHTML
            el.innerHTML = html
            isFirstRender = false
        } else {
            // 后续渲染：morphdom diff，只更新变化的节点
            const template = document.createElement('div')
            template.innerHTML = html
            morphdom(el, template, {
                childrenOnly: true,
                onBeforeNodeAdded: (node: Node) => {
                    if (shouldAddEnterClass && node.nodeType === 1) {
                        ;(node as HTMLElement).classList.add('stream-node-enter')
                    }
                    return node
                },
            })
        }
    }

    /**
     * 同步渲染：全量 renderStreamingSync 生成 HTML，应用到容器
     *
     * 为什么不用 stable/unstable 缓存？
     * 之前用 splitStableUnstable 拆分后分别渲染再拼接，但
     *   renderStreamingSync(stable) + renderStreamingSync(unstable)
     *   ≠ renderStreamingSync(stable + unstable)
     * 因为拆分边界会破坏段落完整性，例如：
     *   - "A\nB" 拆成 stable="A\n" + unstable="B" → <p>A</p><p>B</p>
     *   - "A\nB\n" 全是 stable → <p>A<br>B</p>
     * 两次 HTML 结构不同，morphdom diff 时会删除+重建节点，导致闪烁。
     * 全量渲染保证相同内容总是生成相同 HTML，morphdom 只做增量追加，无闪烁。
     * renderStreamingSync 是纯同步，5000 字约 1ms，性能损失可忽略。
     */
    function doRenderSync(content: string) {
        try {
            const html = renderStreamingSync(content)
            applyHtml(html)
        } catch (err) {
            console.warn('[morph-render] sync render failed, fallback:', err)
            applyHtml(escapeHtml(content))
        }
    }

    /**
     * 最终渲染：流式结束时用完整 renderMarkdown（remark + DOMPurify）渲染
     *
     * 流式期间已渲染行内格式（<strong>、<code>、<em>），结束时 renderMarkdown 会补充
     * 代码块、表格、列表等复杂结构。morphdom diff 时行内格式节点大多能保留，只有
     * 复杂结构需要新建，配合 CSS 淡入动画，视觉上平滑过渡。
     */
    async function doFinalRender(content: string) {
        shouldAddEnterClass = false
        try {
            const html = await renderMarkdown(content)
            applyHtml(html)
        } catch (err) {
            console.warn('[morph-render] final render failed:', err)
        } finally {
            shouldAddEnterClass = true
            isFinalized = false
        }
    }

    function finalizeRender() {
        if (rafId !== null) {
            cancelAnimationFrame(rafId)
            rafId = null
        }
        isFinalized = true
        return doFinalRender(latestContent)
    }

    /**
     * 调度渲染：RAF 合帧
     * 同一帧内多次更新只调度一次 RAF，回调读取最新 latestContent
     */
    function scheduleRender(content: string) {
        latestContent = content
        if (isFinalized) return
        if (rafId !== null) return
        rafId = requestAnimationFrame(() => {
            rafId = null
            doRenderSync(latestContent)
        })
    }

    /**
     * 清空容器，重置状态
     * 清空后再次渲染视为首次渲染（不加淡入类）
     */
    function clear() {
        if (rafId !== null) {
            cancelAnimationFrame(rafId)
            rafId = null
        }
        isFinalized = false
        isFirstRender = true
        const el = containerRef.value
        if (el) el.innerHTML = ''
        latestContent = ''
    }

    /**
     * 绑定响应式内容源
     * 内容变化时自动调度渲染；空内容时清空容器
     *
     * 容器延迟挂载修复：
     * 容器由 v-if="streamingContent" 控制，首次对话时流程是：
     *   streamingContent="" → watch 触发 clear()（容器不存在，跳过）
     *   首 token 到达 → streamingContent="第" → watch 触发 scheduleRender
     *   → Vue patch 创建容器 DOM → containerRef 赋值
     *   → RAF 回调 applyHtml（此时 containerRef 应已有效）
     * 但如果 watch pre-flush 在 DOM patch 之前触发 RAF，RAF 回调执行时
     * containerRef 可能为 null，applyHtml 直接 return，内容丢失。
     * 这里额外监听 containerRef，当 ref 从 null 变为非 null 时，
     * 如果有 pending 内容，重新触发渲染。
     */
    function bind(getContent: () => string) {
        watch(
            getContent,
            (newContent) => {
                if (!newContent) {
                    clear()
                    return
                }
                scheduleRender(newContent)
            },
            { immediate: true },
        )
        // 容器挂载后补偿渲染：解决 v-if 延迟挂载导致的首次内容丢失
        watch(containerRef, (newEl) => {
            if (newEl && latestContent && !isFinalized && rafId === null) {
                scheduleRender(latestContent)
            }
        })
    }

    onScopeDispose(() => {
        if (rafId !== null) cancelAnimationFrame(rafId)
    })

    return {
        containerRef: containerRef as Ref<HTMLElement | null>,
        bind,
        clear,
        finalizeRender,
    }
}
