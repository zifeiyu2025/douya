/**
 * useMorphRender 测试
 *
 * 核心验证点（对标千问 morphdom DOM Diff 方案）：
 * 1. 首次渲染：HTML 写入容器
 * 2. 增量更新：未变化节点保留同一引用（morphdom 核心 value）
 * 3. 新增节点添加 stream-node-enter 淡入动画类
 * 4. RAF 合帧：多次快速更新只触发一次渲染
 * 5. clear() 清空容器
 * 6. finalizeRender 用完整 renderMarkdown 做最终渲染
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, ref, nextTick } from 'vue'
import { useMorphRender } from '../composables/useMorphRender'

// mock renderStreamingSync：按 \n\n 分段为 <p>，\n 转 <br>，模拟真实流式渲染
vi.mock('../utils/streamingRender', () => ({
    renderStreamingSync: vi.fn((text: string) => {
        if (!text) return ''
        const paragraphs = text.split(/\n{2,}/)
        return paragraphs
            .filter(p => p.trim())
            .map(p => {
                const lines = p.trim().split('\n')
                return `<p>${lines.join('<br>')}</p>`
            })
            .join('')
    }),
}))

vi.mock('../utils/markdown', () => ({
    renderMarkdown: vi.fn(async (text: string) => `<article>${text}</article>`),
    escapeHtml: vi.fn((text: string) => text),
    renderMermaidBlocksInElement: vi.fn(),
    renderMathInElement: vi.fn(),
    MermaidTheme: { Light: 'default', Dark: 'dark' },
}))

describe('useMorphRender', () => {
    let scope: ReturnType<typeof effectScope>
    let rafCallbacks: Map<number, () => void>
    let rafIdCounter: number
    let container: HTMLElement

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
        container = document.createElement('div')
        document.body.appendChild(container)
    })

    afterEach(() => {
        scope.stop()
        vi.restoreAllMocks()
        if (container.parentNode) {
            container.parentNode.removeChild(container)
        }
    })

    function flushRaf() {
        const cbs = Array.from(rafCallbacks.values())
        rafCallbacks.clear()
        cbs.forEach((cb) => cb())
    }

    it('首次渲染：HTML 写入容器，生成正确的 DOM 结构', async () => {
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('Hello')
        bind(() => source.value)

        await nextTick()
        flushRaf()

        const p = container.querySelector('p')
        expect(p).not.toBeNull()
        expect(p?.textContent).toBe('Hello')
    })

    it('增量更新：未变化节点保留同一引用（morphdom DOM Diff 核心）', async () => {
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('A')
        bind(() => source.value)

        await nextTick()
        flushRaf()

        const firstP = container.querySelector('p')
        expect(firstP?.textContent).toBe('A')

        // 从 <p>A</p> 变成 <p>A</p><p>B</p>（追加新段落）
        source.value = 'A\n\nB'
        await nextTick()
        flushRaf()

        const paragraphs = container.querySelectorAll('p')
        expect(paragraphs.length).toBe(2)
        expect(paragraphs[0]).toBe(firstP) // 同一引用，未销毁
        expect(paragraphs[1].textContent).toBe('B')
    })

    it('新增节点添加 stream-node-enter 淡入动画类', async () => {
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('A')
        bind(() => source.value)

        await nextTick()
        flushRaf()

        // 首次渲染：直接 innerHTML，不加淡入类（避免首次全部淡入）
        const firstP = container.querySelector('p')
        expect(firstP?.classList.contains('stream-node-enter')).toBe(false)

        // 追加新段落
        source.value = 'A\n\nB'
        await nextTick()
        flushRaf()

        const paragraphs = container.querySelectorAll('p')
        // 第一个 p 保留，不加淡入类
        expect(paragraphs[0].classList.contains('stream-node-enter')).toBe(false)
        // 第二个 p 是新增的，加淡入类
        expect(paragraphs[1].classList.contains('stream-node-enter')).toBe(true)
    })

    it('RAF 合帧：多次快速更新只调度一次 RAF', async () => {
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('')
        bind(() => source.value)

        source.value = 'A'
        await nextTick()
        source.value = 'AB'
        await nextTick()
        source.value = 'ABC'
        await nextTick()

        // 三次更新只应有一次 RAF 调度（合帧）
        expect(rafCallbacks.size).toBe(1)

        flushRaf()

        // 最终内容是最后一次更新
        expect(container.querySelector('p')?.textContent).toBe('ABC')
    })

    it('clear() 清空容器内容', async () => {
        const { containerRef, bind, clear } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('Hello')
        bind(() => source.value)

        await nextTick()
        flushRaf()
        expect(container.innerHTML).not.toBe('')

        clear()
        expect(container.innerHTML).toBe('')
        expect(container.childNodes.length).toBe(0)
    })

    it('clear 后再次渲染视为首次渲染（不加淡入类）', async () => {
        const { containerRef, bind, clear } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('A')
        bind(() => source.value)

        await nextTick()
        flushRaf()
        clear()

        // 重新渲染
        source.value = 'B'
        await nextTick()
        flushRaf()

        const p = container.querySelector('p')
        expect(p?.textContent).toBe('B')
        expect(p?.classList.contains('stream-node-enter')).toBe(false)
    })

    it('finalizeRender 用完整 renderMarkdown 做最终渲染', async () => {
        const { containerRef, bind, finalizeRender } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('Final content.')
        bind(() => source.value)

        await nextTick()
        flushRaf()
        expect(container.innerHTML).toContain('Final content')

        await finalizeRender()
        const { renderMarkdown } = await import('../utils/markdown')
        expect(renderMarkdown).toHaveBeenCalledWith('Final content.')
        expect(container.innerHTML).toBe('<article>Final content.</article>')
    })

    it('空内容触发 clear 而非渲染', async () => {
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('Hello')
        bind(() => source.value)

        await nextTick()
        flushRaf()
        expect(container.innerHTML).not.toBe('')

        source.value = ''
        await nextTick()

        expect(container.innerHTML).toBe('')
    })

    it('容器未绑定时安全跳过渲染', async () => {
        const { bind } = scope.run(() => useMorphRender())!
        const source = ref('Hello')
        // containerRef.value 未设置
        bind(() => source.value)

        await nextTick()
        // 不应抛错
        flushRaf()
    })

    it('容器延迟挂载：watch 触发时容器为 null，挂载后补偿渲染', async () => {
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        const source = ref('')
        bind(() => source.value)

        // 模拟首次对话：streamingContent 变化，但容器 DOM 还没挂载
        source.value = '首 token'
        await nextTick()
        // 此时 containerRef.value 仍为 null（v-if 还没渲染容器）
        flushRaf()
        // applyHtml 因 el 为 null 跳过，内容不渲染

        // 容器挂载（v-if=true → DOM 创建 → ref 赋值）
        containerRef.value = container
        await nextTick()
        // containerRef watch 触发，补偿渲染
        flushRaf()

        // 内容应正确渲染
        expect(container.querySelector('p')?.textContent).toBe('首 token')
    })

    it('回归测试：换行边界变化不导致 HTML 结构重组闪烁', async () => {
        // 场景：流式输出 "A\nB" → "A\nB\n" 时
        // 旧方案（stable/unstable 拆分）会生成不同 HTML 结构：
        //   "A\nB"  → stable="A\n" + unstable="B" → <p>A</p><p>B</p>
        //   "A\nB\n" → stable="A\nB\n" + unstable="" → <p>A<br>B</p>
        // morphdom diff 时删除 <p>B</p> + 修改 <p>A</p>，导致闪烁
        // 新方案（全量渲染）保证相同内容生成相同 HTML：
        //   "A\nB"  → <p>A<br>B</p>
        //   "A\nB\n" → <p>A<br>B</p>（trim 后相同）
        // morphdom diff 无变化，无闪烁
        const { containerRef, bind } = scope.run(() => useMorphRender())!
        containerRef.value = container
        const source = ref('')
        bind(() => source.value)

        // 第一帧：渲染 "A\nB"
        source.value = 'A\nB'
        await nextTick()
        flushRaf()

        const firstP = container.querySelector('p')
        expect(firstP).not.toBeNull()
        const htmlAfterFirst = container.innerHTML

        // 第二帧：渲染 "A\nB\n"（边界变化）
        source.value = 'A\nB\n'
        await nextTick()
        flushRaf()

        // HTML 结构应保持不变（全量渲染保证一致性）
        expect(container.innerHTML).toBe(htmlAfterFirst)

        // 第一个 <p> 节点应保留同一引用（未被销毁重建）
        const secondP = container.querySelector('p')
        expect(secondP).toBe(firstP)
    })
})
