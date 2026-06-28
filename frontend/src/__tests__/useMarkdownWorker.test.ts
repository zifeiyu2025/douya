/**
 * useMarkdownWorker composable 测试
 *
 * 验证 spec: smooth-streaming-render + 流式渲染速度优化 中描述的行为：
 * - setTimeout 合并同帧多次到达（cancelAnimationFrame 已移除）
 * - 渲染时使用最新 pendingContent
 * - 动态间隔分级：
 *   - 流式模式：短<2KB 0ms / 中2-10KB 8ms / 长>10KB 16ms
 *   - 非流式模式：短<2KB 50ms / 中2-10KB 80ms / 长>10KB 120ms
 * - clear() 清理 setTimeout
 * - onScopeDispose 清理资源并 terminate worker
 * - setStreamingMode(false) 触发全量重渲染
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, ref, nextTick } from 'vue'

// ---------------------------------------------------------------------------
// Mock 1: Mock Worker
// Vite 的 ?worker 后缀在 vitest 环境无法解析，需要用 vi.mock 替换为一个假 Worker。
// 假 Worker 把收到的 content 包成 <p> 标签后异步回传，模拟真实 Worker 的消息协议。
// 用 vi.hoisted 提升类定义，确保 vi.mock 工厂函数能引用到它。
// ---------------------------------------------------------------------------
const MockWorkerClass = vi.hoisted(() => {
    return class MockWorker {
        listeners: Record<string, Array<(e: MessageEvent) => void>> = {}
        postMessage(data: { id: number; content: string }) {
            // 模拟异步响应：用 setTimeout(0) 让消息在下一个宏任务中到达
            setTimeout(() => {
                const html = `<p>${data.content}</p>`
                this.dispatchEvent('message', { data: { id: data.id, html } })
            }, 0)
        }
        addEventListener(type: string, listener: (e: MessageEvent) => void) {
            if (!this.listeners[type]) this.listeners[type] = []
            this.listeners[type].push(listener)
        }
        removeEventListener(type: string, listener: (e: MessageEvent) => void) {
            if (!this.listeners[type]) return
            this.listeners[type] = this.listeners[type].filter((l) => l !== listener)
        }
        dispatchEvent(type: string, data: { data: unknown }) {
            ;(this.listeners[type] || []).forEach((l) => l(new MessageEvent(type, data)))
        }
        terminate() {}
    }
})

vi.mock('../workers/markdown.worker?worker', () => ({
    default: MockWorkerClass,
}))

// ---------------------------------------------------------------------------
// Mock 2: Mock ../utils/markdown
// 真实 markdown.ts 依赖 DOMPurify + remark 全家桶，单元测试中无需引入。
// sanitizeHtml 直接透传，renderMarkdownStreaming/renderStreamingLight 简单包成 <p>。
// ---------------------------------------------------------------------------
vi.mock('../utils/markdown', () => ({
    renderMarkdownStreaming: vi.fn(async (content: string) => `<p>${content}</p>`),
    sanitizeHtml: vi.fn((html: string) => html),
    renderStreamingLight: vi.fn(async (content: string) => `<p>${content}</p>`),
}))

import { useMarkdownWorker } from '../composables/useMarkdownWorker'

// 辅助：等待指定毫秒
const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))

describe('useMarkdownWorker', () => {
    let scope: ReturnType<typeof effectScope>
    let currentTime: number

    beforeEach(() => {
        currentTime = 1000000
        vi.spyOn(Date, 'now').mockImplementation(() => currentTime)
        scope = effectScope()
    })

    afterEach(() => {
        scope.stop()
        vi.restoreAllMocks()
    })

    // -------------------------------------------------------------------------
    // Test 1: 多次 scheduleRender 调用通过 setTimeout 合并
    // 验证：scheduleRender 被多次调用时，每次都会 clearTimeout 旧定时器。
    // 注意：Vue 的 watch 会批量合并同帧同步赋值，要让 scheduleRender 被调用多次，
    // 必须在两次赋值之间 await nextTick() 让 watch 分别触发。
    // -------------------------------------------------------------------------
    it('多次 scheduleRender 调用通过 setTimeout 合并', async () => {
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const clearTimeoutSpy = vi.spyOn(globalThis, 'clearTimeout')
        const { bind } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)

        // 第一次赋值 + nextTick → 触发 scheduleRender → 设置 setTimeout
        content.value = 'a'
        await nextTick()
        expect(setTimeoutSpy).toHaveBeenCalled()

        // 第二次赋值 + nextTick → 触发 scheduleRender → clearTimeout 旧定时器，设新定时器
        content.value = 'b'
        await nextTick()
        expect(clearTimeoutSpy).toHaveBeenCalled()
    })

    // -------------------------------------------------------------------------
    // Test 2: 渲染时使用最新 pendingContent
    // 验证：连续赋值 'old'、'new' 后，setTimeout 触发时只渲染 'new'。
    // -------------------------------------------------------------------------
    it('渲染时使用最新 pendingContent', async () => {
        const { rendered, bind } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)

        content.value = 'old'
        await nextTick()
        content.value = 'new'
        await nextTick()

        // 等待 setTimeout(0) 触发 + MockWorker 响应
        await wait(20)

        // rendered 应包含 'new' 而非 'old'（最新 pendingContent 被渲染）
        expect(rendered.value).toContain('new')
        expect(rendered.value).not.toContain('old')
    })

    // -------------------------------------------------------------------------
    // Test 3: 非流式模式动态间隔分级
    // 验证：scheduleRender 根据内容长度选择不同的 setTimeout 延迟。
    // 通过设置 currentTime = 0 让 elapsed = 0，使 setTimeout 以完整 interval 调度。
    // -------------------------------------------------------------------------
    it('非流式模式短内容(<2KB)使用 50ms 间隔', async () => {
        currentTime = 0
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const { bind } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        content.value = 'x'.repeat(100) // 100 字节，属于短内容
        await nextTick()

        // 非流式模式短内容应使用 50ms 间隔
        expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 50)
    })

    it('非流式模式中等内容(2-10KB)使用 80ms 间隔', async () => {
        currentTime = 0
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const { bind } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        content.value = 'x'.repeat(5000) // 5KB，属于中等内容
        await nextTick()

        expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 80)
    })

    it('非流式模式长内容(>10KB)使用 120ms 间隔', async () => {
        currentTime = 0
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const { bind } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        content.value = 'x'.repeat(15000) // 15KB，属于长内容
        await nextTick()

        expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 120)
    })

    // -------------------------------------------------------------------------
    // Test 3b: 流式模式动态间隔分级（更激进）
    // 验证：setStreamingMode(true) 后，间隔从 50/80/120ms 缩短到 0/16/32ms。
    // -------------------------------------------------------------------------
    it('流式模式短内容(<2KB)使用 0ms 间隔', async () => {
        currentTime = 0
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const { bind, setStreamingMode } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        setStreamingMode(true)
        content.value = 'x'.repeat(100)
        await nextTick()

        expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 0)
    })

    it('流式模式中等内容(2-10KB)使用 8ms 间隔', async () => {
        currentTime = 0
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const { bind, setStreamingMode } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        setStreamingMode(true)
        content.value = 'x'.repeat(5000)
        await nextTick()

        expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 8)
    })

    it('流式模式长内容(>10KB)使用 16ms 间隔', async () => {
        currentTime = 0
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        const { bind, setStreamingMode } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        setStreamingMode(true)
        content.value = 'x'.repeat(15000)
        await nextTick()

        expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 16)
    })

    // -------------------------------------------------------------------------
    // Test 4: clear() 清理 setTimeout
    // 验证：clear 调用 clearTimeout，并清空 rendered。
    // -------------------------------------------------------------------------
    it('clear 清理 setTimeout', async () => {
        const clearTimeoutSpy = vi.spyOn(globalThis, 'clearTimeout')
        const { rendered, bind, clear } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        content.value = 'test'
        await nextTick() // 触发 scheduleRender → 设置 setTimeout

        clear()

        expect(clearTimeoutSpy).toHaveBeenCalled()
        expect(rendered.value).toBe('')
    })

    // -------------------------------------------------------------------------
    // Test 5: onScopeDispose 清理
    // 验证：scope.stop() 触发 onScopeDispose 时，clearTimeout 被调用。
    // -------------------------------------------------------------------------
    it('onScopeDispose 触发时清理 setTimeout', async () => {
        const clearTimeoutSpy = vi.spyOn(globalThis, 'clearTimeout')
        const localScope = effectScope()
        const { bind } = localScope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        content.value = 'test'
        await nextTick() // 触发 scheduleRender → 设置 setTimeout

        localScope.stop() // 触发 onScopeDispose

        expect(clearTimeoutSpy).toHaveBeenCalled()
    })

    // -------------------------------------------------------------------------
    // Test 6: setStreamingMode(false) 触发全量重渲染
    // 验证：退出流式模式后，rendered 被全量重渲染（Worker + DOMPurify）。
    // -------------------------------------------------------------------------
    it('setStreamingMode(false) 触发全量重渲染', async () => {
        const { rendered, bind, setStreamingMode } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        setStreamingMode(true)

        content.value = 'hello'
        await nextTick()
        await wait(20)
        expect(rendered.value).toContain('hello')

        // 退出流式模式：应触发 doFinalRender
        setStreamingMode(false)
        await wait(30)

        // rendered 仍包含内容（全量重渲染后）
        expect(rendered.value).toContain('hello')
    })

    // -------------------------------------------------------------------------
    // Test 7: setStreamingMode 重复调用无副作用
    // 验证：重复设置相同值不会触发额外渲染。
    // -------------------------------------------------------------------------
    it('setStreamingMode 重复调用相同值无副作用', async () => {
        const { bind, setStreamingMode } = scope.run(() => useMarkdownWorker())!
        const content = ref('')
        bind(() => content.value)
        setStreamingMode(true)

        content.value = 'test'
        await nextTick()
        await wait(20)

        // 重复设置 true：应无副作用（不触发 doFinalRender）
        const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')
        setTimeoutSpy.mockClear()
        setStreamingMode(true)
        await nextTick()
        // 不应有新的 setTimeout 被调用（因为 isStreamingMode 未变化）
        expect(setTimeoutSpy).not.toHaveBeenCalled()
    })
})
