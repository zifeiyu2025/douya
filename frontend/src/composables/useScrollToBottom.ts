/**
 * 滚动到底部 composable
 * 用于消息列表的智能滚动行为：
 *   - 用户在底部时：内容变化平滑滚动跟随（smooth + RAF 批处理 + 100ms 节流）
 *   - 用户向上滚动超过 10px：停止自动滚动
 *   - 程序化滚动不误判为用户滚动（isProgrammaticScroll 防循环）
 *   - MutationObserver 监听 DOM 变化触发滚动
 */
import { ref, watch, onScopeDispose } from 'vue'

export function useScrollToBottom(threshold = 10) {
    const containerRef = ref<HTMLElement | null>(null)
    // 程序化滚动标志：防止 scrollToBottom 触发的 scroll 事件被误判为用户滚动
    let isProgrammaticScroll = false
    let userScrollCallbacks: Array<() => void> = []
    // RAF 批处理：一帧内多次内容变化只滚动一次
    let rafId: number | null = null
    // 100ms 节流：防止过于频繁的滚动
    let lastScrollTime = 0
    let scrollTimer: ReturnType<typeof setTimeout> | null = null
    let observer: MutationObserver | null = null
    let scrollListenerBound = false

    function isNearBottom(): boolean {
        const el = containerRef.value
        if (!el) return true
        return el.scrollHeight - el.scrollTop - el.clientHeight < threshold
    }

    function scrollToBottom(behavior: 'smooth' | 'auto' = 'smooth') {
        const el = containerRef.value
        if (!el) return
        // 标记程序化滚动，scroll 事件中据此跳过用户滚动判定
        isProgrammaticScroll = true
        el.scrollTo({ top: el.scrollHeight, behavior })
        // 下一帧重置标志（scroll 事件已在此帧同步触发）
        requestAnimationFrame(() => {
            isProgrammaticScroll = false
        })
    }

    function onUserScroll(cb: () => void) {
        userScrollCallbacks.push(cb)
    }

    function bindScrollListener() {
        const el = containerRef.value
        if (!el || scrollListenerBound) return
        el.addEventListener('scroll', () => {
            if (!isProgrammaticScroll) {
                userScrollCallbacks.forEach((cb) => cb())
            }
        })
        scrollListenerBound = true
    }

    /**
     * 调度滚动：RAF 批处理 + 100ms 节流
     * 一帧内多次调用只执行一次；两次滚动间隔至少 100ms
     */
    function scheduleScroll() {
        if (!isNearBottom()) return
        // RAF 批处理：同一帧内已调度则跳过
        if (rafId !== null) return
        rafId = requestAnimationFrame(() => {
            rafId = null
            const now = Date.now()
            const elapsed = now - lastScrollTime
            if (elapsed >= 100) {
                // 间隔足够，立即滚动
                lastScrollTime = now
                scrollToBottom('smooth')
            } else if (scrollTimer === null) {
                // 间隔不足，延迟到 100ms 后
                scrollTimer = setTimeout(() => {
                    scrollTimer = null
                    lastScrollTime = Date.now()
                    if (isNearBottom()) scrollToBottom('smooth')
                }, 100 - elapsed)
            }
        })
    }

    function watchContentChange(getContent: () => unknown) {
        watch(getContent, () => {
            scheduleScroll()
        })
    }

    function watchMessagesLength(getLength: () => number) {
        let prevLen = 0
        watch(getLength, (newLen) => {
            if (newLen > prevLen) {
                // 新增消息：强制滚动到底部（绕过节流）
                requestAnimationFrame(() => scrollToBottom('smooth'))
            } else if (isNearBottom()) {
                scheduleScroll()
            }
            prevLen = newLen
        })
    }

    /** 启动 MutationObserver 监听容器子节点变化 */
    function startObserver() {
        const el = containerRef.value
        if (!el || observer) return
        observer = new MutationObserver(() => {
            scheduleScroll()
        })
        observer.observe(el, { childList: true, subtree: true, characterData: true })
    }

    function stopObserver() {
        observer?.disconnect()
        observer = null
    }

    // containerRef 变化时同步绑定 scroll listener（flush: 'sync' 确保立即绑定）
    watch(
        containerRef,
        (el) => {
            if (el) bindScrollListener()
        },
        { flush: 'sync' }
    )

    onScopeDispose(() => {
        stopObserver()
        if (rafId !== null) cancelAnimationFrame(rafId)
        if (scrollTimer) clearTimeout(scrollTimer)
    })

    return {
        containerRef,
        isNearBottom,
        scrollToBottom,
        watchContentChange,
        watchMessagesLength,
        onUserScroll,
        startObserver,
        stopObserver,
    }
}
