/**
 * Markdown 渲染 composable
 * 提供节流(throttle)的流式 Markdown 渲染
 * 避免高频 token 更新时频繁触发重渲染
 */
import { ref, watch, onUnmounted } from 'vue'
import { renderMarkdownStreaming } from '../utils/markdown'

export function useMarkdownRender() {
    const rendered = ref('')
    let renderTimer: ReturnType<typeof setTimeout> | null = null
    let lastRenderTime = 0
    let pendingContent = ''
    let isRendering = false

    async function doRender(content: string) {
        isRendering = true
        rendered.value = await renderMarkdownStreaming(content)
        lastRenderTime = Date.now()
        isRendering = false
    }

    /**
     * 根据内容长度动态调整节流间隔：
     *  - 短内容（<2KB）快速渲染
     *  - 长内容（>10KB）降低频率
     */
    function scheduleRender(content: string) {
        pendingContent = content
        if (isRendering) return

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
                doRender(pendingContent)
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
        if (renderTimer) {
            clearTimeout(renderTimer)
            renderTimer = null
        }
    })

    return {
        rendered,
        bind,
        clear,
    }
}
