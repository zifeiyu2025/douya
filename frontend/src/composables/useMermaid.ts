/**
 * Mermaid 懒加载 composable
 * - 用 IntersectionObserver 观察消息中的 .mermaid 元素
 * - 当图表进入视口（rootMargin 200px 提前触发）才调用 renderMermaidInElement
 * - 自动清理观察器
 *
 * 收益：Mermaid 2.84MB chunk 仅在用户滚动到含 mermaid 的消息时才下载
 */
import { onMounted, onUnmounted, nextTick, type Ref } from 'vue'
import { renderMermaidInElement } from '../utils/markdown'

export function useMermaid(rootRef: Ref<HTMLElement | undefined>) {
    let observer: IntersectionObserver | null = null

    function observeAll() {
        if (!observer || !rootRef.value) return
        const els = rootRef.value.querySelectorAll('.mermaid:not([data-mermaid-rendered])')
        els.forEach((el) => observer!.observe(el))
    }

    onMounted(() => {
        // IO 不支持，fallback 直接渲染
        if (typeof IntersectionObserver === 'undefined') {
            nextTick(() => {
                if (rootRef.value) renderMermaidInElement(rootRef.value)
            })
            return
        }

        observer = new IntersectionObserver(
            (entries) => {
                for (const entry of entries) {
                    if (entry.isIntersecting) {
                        const target = entry.target as HTMLElement
                        observer?.unobserve(target)
                        // 渲染该元素（renderMermaidInElement 内部会查找 .mermaid 节点）
                        renderMermaidInElement(target)
                    }
                }
            },
            { rootMargin: '200px' }
        )

        nextTick(() => {
            observeAll()
            // 复制按钮由 setupCodeCopyDelegation 事件委托处理
        })
    })

    /**
     * 渲染完成后（或内容变化后）重新观察新出现的 .mermaid 元素
     * 在 MessageItem 渲染流式消息时调用
     */
    function refreshObservation() {
        nextTick(() => observeAll())
    }

    onUnmounted(() => {
        if (observer) {
            observer.disconnect()
            observer = null
        }
    })

    return {
        refreshObservation,
    }
}
