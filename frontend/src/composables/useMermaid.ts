/**
 * Mermaid 懒加载 composable
 * - 用模块级共享 IntersectionObserver 观察消息中的 .mermaid 元素
 * - 所有 MessageItem 实例复用同一个 IO，避免长会话下创建大量观察器
 * - 当图表进入视口（rootMargin 200px 提前触发）才调用 renderMermaidInElement
 * - 渲染完成后 unobserve（Mermaid 渲染一次即可，无需重复）
 * - 组件卸载时清理本实例注册的 target，避免内存泄漏
 *
 * 收益：Mermaid 2.84MB chunk 仅在用户滚动到含 mermaid 的消息时才下载
 */
import { onMounted, onUnmounted, nextTick, type Ref } from 'vue'
import { renderMermaidInElement } from '../utils/markdown'

// 模块级单例 IntersectionObserver，所有 MessageItem 共享
// 避免长会话下每个 MessageItem 各创建一个 IO（IO 本身有开销，数量过多可能触发浏览器警告）
let sharedIO: IntersectionObserver | null = null

// target → 渲染函数 映射
// 使用 WeakMap：当 .mermaid 元素被 DOM 移除后，对应条目可被 GC 自动回收，避免内存泄漏
const targetToRenderFn = new WeakMap<Element, () => void>()

/**
 * 获取共享 IntersectionObserver（惰性创建）
 * 回调内根据 entry.target 找到对应的渲染函数并执行，渲染完成后 unobserve
 */
function getSharedIO(): IntersectionObserver {
    if (!sharedIO) {
        // 用局部 io 变量在闭包内引用，避免回调中访问可能被重赋值的 sharedIO
        const io = new IntersectionObserver(
            (entries) => {
                for (const entry of entries) {
                    if (entry.isIntersecting) {
                        const renderFn = targetToRenderFn.get(entry.target)
                        if (renderFn) {
                            // 调用对应的渲染函数（renderMermaidInElement 内部会查找 .mermaid 节点）
                            renderFn()
                            // 渲染完成后从映射表中移除并取消观察（Mermaid 渲染一次即可，无需重复）
                            targetToRenderFn.delete(entry.target)
                            io.unobserve(entry.target)
                        }
                    }
                }
            },
            { rootMargin: '200px' }
        )
        sharedIO = io
    }
    return sharedIO
}

export function useMermaid(rootRef: Ref<HTMLElement | undefined>) {
    // 本实例注册到共享 IO 的 target 集合，用于卸载时清理
    // 注意：不能调用 disconnect()，因为 IO 是共享的，disconnect 会影响其他 MessageItem
    const registeredTargets: Set<Element> = new Set()

    function observeAll() {
        if (!rootRef.value) return
        // IO 不支持时由 onMounted 的 fallback 路径处理，此处直接返回（保持与原实现一致）
        if (typeof IntersectionObserver === 'undefined') return
        const io = getSharedIO()
        const els = rootRef.value.querySelectorAll('.mermaid:not([data-mermaid-rendered])')
        els.forEach((el) => {
            // 避免重复注册（同一元素可能因 refreshObservation 多次进入）
            if (targetToRenderFn.has(el)) return
            const target = el as HTMLElement
            // 为每个 target 绑定渲染函数（renderMermaidInElement 内部会查找 .mermaid 节点）
            targetToRenderFn.set(target, () => renderMermaidInElement(target))
            registeredTargets.add(target)
            io.observe(target)
        })
    }

    onMounted(() => {
        // IO 不支持，fallback 直接渲染整个根元素
        if (typeof IntersectionObserver === 'undefined') {
            nextTick(() => {
                if (rootRef.value) renderMermaidInElement(rootRef.value)
            })
            return
        }

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
        // 清理本实例注册的所有 target，避免内存泄漏
        // 注意：不调用 disconnect()，因为 IO 是共享的，disconnect 会影响其他 MessageItem
        const io = sharedIO
        if (io) {
            for (const target of registeredTargets) {
                io.unobserve(target)
                targetToRenderFn.delete(target)
            }
        }
        registeredTargets.clear()
    })

    return {
        refreshObservation,
    }
}
