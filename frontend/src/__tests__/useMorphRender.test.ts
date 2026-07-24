/**
 * useMorphRender 测试（轻量版，v-html 方案）
 *
 * 核心验证点：
 * 1. 首次渲染：HTML 写入容器
 * 2. 内容更新：HTML 更新为新内容
 * 3. RAF 合帧：多次快速更新只触发一次渲染
 * 4. clear() 清空容器
 * 5. finalizeRender 用完整 renderMarkdown 做最终渲染
 * 6. 空内容触发 clear
 * 7. 容器未绑定时安全跳过
 * 8. 容器延迟挂载补偿渲染
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, ref, nextTick } from 'vue'
import { useMorphRender } from '../composables/useMorphRender'

// mock marked：简单地将文本按 \n\n 分段为 <p>
// useMorphRender.ts 从 marked 导入 lexer 和 Parser（非 marked.lexer / marked.Parser），
// mock 需提供这两个独立导出，否则 vitest 报 "No lexer export defined"
vi.mock('marked', () => ({
  marked: {
    parse: vi.fn((text: string) => {
      if (!text) return ''
      const paragraphs = text.split(/\n{2,}/)
      return paragraphs
        .filter(p => p.trim())
        .map(p => `<p>${p}</p>`)
        .join('')
    }),
    use: vi.fn(),
    Renderer: vi.fn(() => ({}))
  },
  // lexer：将文本按 \n\n 分段为 token 数组（模拟 marked.Lexer.lex）
  lexer: vi.fn((text: string) => {
    if (!text) return []
    return text
      .split(/\n{2,}/)
      .filter(p => p.trim())
      .map(p => ({ type: 'paragraph', raw: p }))
  }),
  // Parser.parse：将 token 数组渲染为 HTML（模拟 marked.Parser.parse）
  Parser: {
    parse: vi.fn((tokens: { raw: string }[]) => tokens.map(t => `<p>${t.raw}</p>`).join(''))
  }
}))

// mock lightSanitize：直接返回原 HTML（测试环境无 XSS 风险）
vi.mock('../utils/lightSanitize', () => ({
  lightSanitize: vi.fn((html: string) => html),
  isSafeUrl: vi.fn((_url: string) => true)
}))

vi.mock('../utils/markdown', () => ({
  renderMarkdown: vi.fn(async (text: string) => `<article>${text}</article>`),
  escapeHtml: vi.fn((text: string) => text),
  // useMorphRender.renderToken 调用 sanitizeHtml 消毒每个 token 的 HTML，
  // 测试环境无 XSS 风险，直接返回原 HTML
  sanitizeHtml: vi.fn((html: string) => html)
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
    cbs.forEach(cb => cb())
  }

  it('首次渲染：HTML 写入容器', async () => {
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

  it('内容更新：HTML 更新为新内容', async () => {
    const { containerRef, bind } = scope.run(() => useMorphRender())!
    containerRef.value = container
    const source = ref('A')
    bind(() => source.value)

    await nextTick()
    flushRaf()

    expect(container.querySelector('p')?.textContent).toBe('A')

    // 更新内容
    source.value = 'A\n\nB'
    await nextTick()
    flushRaf()

    const paragraphs = container.querySelectorAll('p')
    expect(paragraphs.length).toBe(2)
    expect(paragraphs[0].textContent).toBe('A')
    expect(paragraphs[1].textContent).toBe('B')
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

  // 回归测试：修复段落从 N 增长到 N+1 时 DOM 顺序错乱
  // 旧 bug：第一行渲染后，第三行先出现在 unstable-block，第二行被新建在末尾，
  // 导致顺序变成 A、C、B
  it('段落连续增长时保持 DOM 顺序（1段→2段→3段→4段）', async () => {
    const { containerRef, bind } = scope.run(() => useMorphRender())!
    containerRef.value = container
    const source = ref('')
    bind(() => source.value)

    // 帧1：1段
    source.value = 'A'
    await nextTick()
    flushRaf()
    expect(container.textContent).toBe('A')

    // 帧2：2段
    source.value = 'A\n\nB'
    await nextTick()
    flushRaf()
    // 验证：第一行 A 在前，第二行 B 在后
    const ps2 = container.querySelectorAll('p')
    expect(ps2.length).toBe(2)
    expect(ps2[0].textContent).toBe('A')
    expect(ps2[1].textContent).toBe('B')

    // 帧3：3段（bug 触发点：原本会变成 A、C、B）
    source.value = 'A\n\nB\n\nC'
    await nextTick()
    flushRaf()
    const ps3 = container.querySelectorAll('p')
    expect(ps3.length).toBe(3)
    // 关键断言：DOM 顺序必须与逻辑顺序一致
    expect(Array.from(ps3).map(p => p.textContent)).toEqual(['A', 'B', 'C'])

    // 帧4：4段（再次增长，验证连续提升的稳定性）
    source.value = 'A\n\nB\n\nC\n\nD'
    await nextTick()
    flushRaf()
    const ps4 = container.querySelectorAll('p')
    expect(ps4.length).toBe(4)
    expect(Array.from(ps4).map(p => p.textContent)).toEqual(['A', 'B', 'C', 'D'])
  })

  // 回归测试：跨帧跳跃增长（从1段直接跳到3段）也应保持顺序
  it('跳跃增长时保持 DOM 顺序（1段→3段）', async () => {
    const { containerRef, bind } = scope.run(() => useMorphRender())!
    containerRef.value = container
    const source = ref('')
    bind(() => source.value)

    source.value = 'A'
    await nextTick()
    flushRaf()

    // 直接跳到3段（跳过中间态）
    source.value = 'A\n\nB\n\nC'
    await nextTick()
    flushRaf()

    const ps = container.querySelectorAll('p')
    expect(ps.length).toBe(3)
    expect(Array.from(ps).map(p => p.textContent)).toEqual(['A', 'B', 'C'])
  })
})
