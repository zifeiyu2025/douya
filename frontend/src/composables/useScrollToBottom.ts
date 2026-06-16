/**
 * 滚动到底部 composable
 * 用于消息列表的智能滚动行为：
 *   - 新消息：总是滚动
 *   - 内容更新：仅在用户已经在底部时滚动
 */
import { ref, watch, nextTick } from 'vue'

export function useScrollToBottom(threshold = 100) {
    const containerRef = ref<HTMLElement | null>(null)

    function isNearBottom(): boolean {
        const el = containerRef.value
        if (!el) return true
        return el.scrollHeight - el.scrollTop - el.clientHeight < threshold
    }

    function scrollToBottom() {
        const el = containerRef.value
        if (el) {
            el.scrollTop = el.scrollHeight
        }
    }

    function watchMessagesLength(getLength: () => number) {
        let prevLen = 0
        watch(getLength, (newLen) => {
            if (newLen > prevLen) {
                // 新增消息
                nextTick(scrollToBottom)
            } else if (isNearBottom()) {
                nextTick(scrollToBottom)
            }
            prevLen = newLen
        })
    }

    function watchContentChange(getContent: () => unknown) {
        watch(getContent, () => {
            if (isNearBottom()) {
                nextTick(scrollToBottom)
            }
        })
    }

    return {
        containerRef,
        isNearBottom,
        scrollToBottom,
        watchMessagesLength,
        watchContentChange,
    }
}
