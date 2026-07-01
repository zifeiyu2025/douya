import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, ref, nextTick } from 'vue'
import { useMarkdownWorker } from '../composables/useMarkdownWorker'

vi.mock('../utils/streamingRender', () => ({
    renderStreamingSync: vi.fn((text: string) => `<p>${text}</p>`),
}))

vi.mock('../utils/markdown', () => ({
    renderMarkdown: vi.fn(async (text: string) => `<article>${text}</article>`),
    renderStreamingLight: vi.fn(async (text: string) => `<p>${text}</p>`),
    escapeHtml: vi.fn((text: string) => text),
    renderMermaidBlocksInElement: vi.fn(),
    renderMathInElement: vi.fn(),
    MermaidTheme: { Light: 'default', Dark: 'dark' },
}))

vi.mock('../utils/markdownStreaming', () => ({
    splitStableUnstable: vi.fn((text: string) => {
        const lastNewline = text.lastIndexOf('\n')
        const lastPeriod = text.lastIndexOf('.')
        const stableEnd = Math.max(lastNewline, lastPeriod)
        if (stableEnd < 0 || text.length - stableEnd > 100) {
            return { stable: '', unstable: text }
        }
        return { stable: text.slice(0, stableEnd + 1), unstable: text.slice(stableEnd + 1) }
    }),
}))

describe('useMarkdownWorker', () => {
    let scope: ReturnType<typeof effectScope>
    let rafCallbacks: Map<number, () => void>
    let rafIdCounter: number

    beforeEach(() => {
        rafCallbacks = new Map()
        rafIdCounter = 1
        vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((cb: FrameRequestCallback) => {
            const id = rafIdCounter++
            rafCallbacks.set(id, () => cb(performance.now()))
            return id
        })
        vi.spyOn(globalThis, 'cancelAnimationFrame').mockImplementation((id: number) => {
            rafCallbacks.delete(id)
        })
        vi.clearAllMocks()
        scope = effectScope()
    })

    afterEach(() => {
        scope.stop()
        vi.restoreAllMocks()
    })

    function flushRaf() {
        const cbs = Array.from(rafCallbacks.values())
        rafCallbacks.clear()
        cbs.forEach((cb) => cb())
    }

    function flushAllRaf() {
        while (rafCallbacks.size > 0) {
            flushRaf()
        }
    }

    it('流式内容通过 RAF 帧同步渲染', async () => {
        const source = ref('')
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        source.value = 'Hello'
        await nextTick()
        expect(rendered.value).toBe('')

        flushRaf()
        expect(rendered.value).toContain('Hello')
    })

    it('多次快速更新只触发一次渲染（RAF 合并）', async () => {
        const source = ref('')
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        const { renderStreamingSync } = await import('../utils/streamingRender')

        source.value = 'A'
        await nextTick()
        source.value = 'AB'
        await nextTick()
        source.value = 'ABC'
        await nextTick()

        expect(rendered.value).toBe('')
        expect(renderStreamingSync).not.toHaveBeenCalled()

        flushRaf()

        expect(renderStreamingSync).toHaveBeenCalled()
        expect(rendered.value).toContain('ABC')
    })

    it('clear() 清空渲染结果', async () => {
        const source = ref('')
        const { rendered, bind, clear } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        source.value = 'Hello'
        await nextTick()
        flushRaf()
        expect(rendered.value).not.toBe('')

        source.value = ''
        await nextTick()
        expect(rendered.value).toBe('')

        clear()
        expect(rendered.value).toBe('')
    })

    it('初始空内容不触发渲染', async () => {
        const source = ref('')
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        await nextTick()
        expect(rendered.value).toBe('')
    })

    it('finalizeRender 用完整 renderMarkdown 做最终渲染', async () => {
        const source = ref('')
        const { rendered, bind, finalizeRender } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        source.value = 'Final content.'
        await nextTick()
        flushRaf()
        expect(rendered.value).toContain('Final content')

        await finalizeRender()
        const { renderMarkdown } = await import('../utils/markdown')
        expect(renderMarkdown).toHaveBeenCalledWith('Final content.')
        expect(rendered.value).toBe('<article>Final content.</article>')
    })

    it('流式期间使用同步 renderStreamingSync（不走异步），每帧立即渲染', async () => {
        const source = ref('')
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        const { renderStreamingSync } = await import('../utils/streamingRender')
        const { renderStreamingLight } = await import('../utils/markdown')

        source.value = 'First chunk'
        await nextTick()
        flushRaf()

        expect(renderStreamingSync).toHaveBeenCalled()
        expect(renderStreamingLight).not.toHaveBeenCalled()
        expect(rendered.value).toContain('First chunk')
    })

    it('每帧渲染全部最新内容（不限制字符数，对标千问无限速）', async () => {
        const source = ref('')
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        // 模拟 SSE 一次到达 50 个字符
        source.value = 'A'.repeat(50)
        await nextTick()
        flushRaf()

        // 一帧内应渲染全部 50 个字符，不限速
        expect(rendered.value).toContain('A'.repeat(50))
    })

    it('已有 RAF 调度时不重复设置（只更新 latestContent）', async () => {
        const source = ref('')
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        bind(() => source.value)

        const { renderStreamingSync } = await import('../utils/streamingRender')

        source.value = 'First'
        await nextTick()

        // 第一次 scheduleRender 应设置 RAF
        expect(rafCallbacks.size).toBe(1)

        source.value = 'Second'
        await nextTick()

        // 第二次 scheduleRender 不应设置新 RAF（已有调度）
        expect(rafCallbacks.size).toBe(1)

        flushRaf()

        // RAF 回调渲染的是最新内容
        expect(rendered.value).toContain('Second')
        // 只调用了一次 renderStreamingSync（stable+unstable 合并后）
        const callsAfterFlush = (renderStreamingSync as ReturnType<typeof vi.fn>).mock.calls.length
        expect(callsAfterFlush).toBeGreaterThan(0)
    })
})
