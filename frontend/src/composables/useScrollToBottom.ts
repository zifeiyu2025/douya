/**
 * 滚动到底部 composable
 * 主流方案：即时滚动 + 用户滚动检测 + 回到底部按钮
 *   - 流式期间用 behavior: 'auto'（即时滚动），非流式用 smooth
 *   - 用户向上滚动超过阈值时停止自动滚动，显示"回到底部"按钮
 *   - 用户滚回底部时自动恢复自动滚动
 *   - RAF 批处理 + 100ms 节流防止过于频繁的滚动
 */
import { ref, watch, onScopeDispose } from 'vue'

export function useScrollToBottom(threshold = 150) {
    const containerRef = ref<HTMLElement | null>(null)
    // 响应式：是否启用自动滚动（用户向上滚动后变为 false）
    const isAutoScrollEnabled = ref(true)
    // 程序化滚动标志：防止 scrollToBottom 触发的 scroll 事件被误判为用户滚动
    let isProgrammaticScroll = false
    // RAF 批处理：一帧内多次内容变化只滚动一次
    let rafId: number | null = null
    // 100ms 节流：防止过于频繁的滚动
    let lastScrollTime = 0
    let scrollTimer: ReturnType<typeof setTimeout> | null = null
    let observer: MutationObserver | null = null
    // 命名 scroll handler 引用，便于后续 removeEventListener 移除
    let scrollHandler: (() => void) | null = null
    // 当前已绑定 scroll 监听器的元素，用于 containerRef 变化或销毁时正确解绑
    let boundElement: HTMLElement | null = null
    // 增量滚动：记录上次滚动高度，只滚动差值部分
    let lastScrollHeight = 0
    // 是否处于流式模式（内容持续增长）
    let isStreamingMode = false

    function isNearBottom(): boolean {
        const el = containerRef.value
        if (!el) return true
        return el.scrollHeight - el.scrollTop - el.clientHeight < threshold
    }

    function scrollToBottom(behavior: 'smooth' | 'auto' = 'auto') {
        const el = containerRef.value
        if (!el) return
        isProgrammaticScroll = true
        el.scrollTo({ top: el.scrollHeight, behavior })
        // 即时滚动：scroll 事件在同步代码中触发，可在微任务中重置
        // smooth 滚动：动画持续期间需要保持标志，用 setTimeout 延迟重置
        if (behavior === 'smooth') {
            setTimeout(() => {
                isProgrammaticScroll = false
            }, 500)
        } else {
            // auto 滚动是同步的，下一帧重置即可
            requestAnimationFrame(() => {
                isProgrammaticScroll = false
            })
        }
    }

    /**
     * 流式期间增量滚动：只滚动新增内容的高度差
     * 比 scrollTo(scrollHeight) 更平滑，避免整页跳跃
     */
    function scrollByDelta() {
        const el = containerRef.value
        if (!el) return
        const newHeight = el.scrollHeight
        const delta = newHeight - lastScrollHeight
        lastScrollHeight = newHeight
        if (delta > 0) {
            isProgrammaticScroll = true
            el.scrollBy({ top: delta, behavior: 'auto' })
            requestAnimationFrame(() => {
                isProgrammaticScroll = false
            })
        }
    }

    function bindScrollListener() {
        const el = containerRef.value
        if (!el) return
        // 元素变化时，先从旧元素移除监听器，防止泄漏
        if (boundElement && boundElement !== el && scrollHandler) {
            boundElement.removeEventListener('scroll', scrollHandler)
            boundElement = null
            scrollHandler = null
        }
        // 已绑定到同一元素，跳过
        if (boundElement === el && scrollHandler) return
        // 使用命名函数，便于后续 removeEventListener
        scrollHandler = () => {
            if (isProgrammaticScroll) return
            // 用户手动滚动：检查是否在底部附近
            if (isNearBottom()) {
                // 用户滚回底部，恢复自动滚动
                isAutoScrollEnabled.value = true
            } else {
                // 用户向上滚动超过阈值，停止自动滚动
                isAutoScrollEnabled.value = false
            }
        }
        el.addEventListener('scroll', scrollHandler, { passive: true })
        boundElement = el
    }

    /**
     * 调度滚动：RAF 批处理 + 100ms 节流
     * 流式模式下使用增量滚动（scrollByDelta），非流式使用绝对滚动
     */
    function scheduleScroll() {
        if (!isAutoScrollEnabled.value) return
        // RAF 批处理：同一帧内已调度则跳过
        if (rafId !== null) return
        rafId = requestAnimationFrame(() => {
            rafId = null
            const now = Date.now()
            const elapsed = now - lastScrollTime
            if (elapsed >= 100) {
                lastScrollTime = now
                if (isStreamingMode) {
                    scrollByDelta()
                } else {
                    scrollToBottom('auto')
                }
            } else if (scrollTimer === null) {
                scrollTimer = setTimeout(() => {
                    scrollTimer = null
                    lastScrollTime = Date.now()
                    if (isAutoScrollEnabled.value) {
                        if (isStreamingMode) {
                            scrollByDelta()
                        } else {
                            scrollToBottom('auto')
                        }
                    }
                }, 100 - elapsed)
            }
        })
    }

    function watchContentChange(getContent: () => unknown) {
        watch(getContent, () => {
            scheduleScroll()
        })
    }

    /**
     * 设置流式模式：流式期间使用增量滚动，更平滑
     * 开始流式时传入 true，结束时传入 false
     */
    function setStreamingMode(streaming: boolean) {
        isStreamingMode = streaming
        if (streaming) {
            // 进入流式模式时记录当前高度作为基准
            const el = containerRef.value
            if (el) lastScrollHeight = el.scrollHeight
        }
    }

    function watchMessagesLength(getLength: () => number) {
        let prevLen = 0
        watch(getLength, (newLen) => {
            if (newLen > prevLen) {
                // 新增消息：强制滚动到底部（绕过节流）
                isAutoScrollEnabled.value = true
                requestAnimationFrame(() => scrollToBottom('smooth'))
            } else if (isAutoScrollEnabled.value) {
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

    // containerRef 变化时同步绑定 scroll listener
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
        // 移除 scroll 监听器，防止 DOM 元素上残留监听器导致内存泄漏
        if (scrollHandler && boundElement) {
            boundElement.removeEventListener('scroll', scrollHandler)
            scrollHandler = null
            boundElement = null
        }
    })

    return {
        containerRef,
        isAutoScrollEnabled,
        isNearBottom,
        scrollToBottom,
        watchContentChange,
        watchMessagesLength,
        setStreamingMode,
        startObserver,
        stopObserver,
    }
}
