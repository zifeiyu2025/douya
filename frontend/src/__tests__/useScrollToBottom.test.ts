/**
 * useScrollToBottom composable 测试
 *
 * 验证流式滚动控制的平滑行为：
 * - 用户在底部时平滑滚动跟随
 * - 用户向上滚动超过 10px 时停止自动滚动
 * - 程序化滚动不误判为用户滚动（防循环）
 * - RAF 批处理 + 100ms 节流，避免高频滚动抖动
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, ref } from 'vue'
import { useScrollToBottom } from '../composables/useScrollToBottom'

describe('useScrollToBottom', () => {
  let scope: ReturnType<typeof effectScope>
  let container: HTMLElement
  let scrollToSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    scope = effectScope()
    container = document.createElement('div')
    // 模拟可滚动容器：内容高 1000px，视口高 400px
    Object.defineProperty(container, 'scrollHeight', { configurable: true, get: () => 1000 })
    Object.defineProperty(container, 'clientHeight', { configurable: true, get: () => 400 })
    Object.defineProperty(container, 'scrollTop', { configurable: true, writable: true, value: 0 })
    scrollToSpy = vi.spyOn(container, 'scrollTo').mockImplementation(() => {})
    document.body.appendChild(container)
  })

  afterEach(() => {
    scope.stop()
    container.remove()
    vi.restoreAllMocks()
  })

  // RED-1：用户在底部时，内容变化应平滑滚动到底部
  it('用户在底部时内容变化应平滑滚动到底部', async () => {
    // 用户在底部：scrollTop = 600，距底部 = 1000 - 600 - 400 = 0 < 10
    container.scrollTop = 600
    const content = ref('a')

    scope.run(() => {
      const { containerRef, watchContentChange } = useScrollToBottom()
      containerRef.value = container
      watchContentChange(() => content.value)
    })

    // 触发内容变化
    content.value = 'ab'
    // 等待 RAF + 微任务
    await new Promise((r) => requestAnimationFrame(r))
    await new Promise((r) => setTimeout(r, 120))

    expect(scrollToSpy).toHaveBeenCalled()
    // 应使用 smooth 行为
    const lastCall = scrollToSpy.mock.calls[scrollToSpy.mock.calls.length - 1]
    expect(lastCall[0]).toMatchObject({ behavior: 'smooth' })
  })

  // RED-2：用户向上滚动超过 10px 时，不自动滚动
  it('用户向上滚动超过10px时不自动滚动', async () => {
    // 距底部 = 1000 - 500 - 400 = 100 > 10，用户已向上滚动
    container.scrollTop = 500
    const content = ref('a')

    scope.run(() => {
      const { containerRef, watchContentChange } = useScrollToBottom()
      containerRef.value = container
      watchContentChange(() => content.value)
    })

    content.value = 'ab'
    await new Promise((r) => requestAnimationFrame(r))
    await new Promise((r) => setTimeout(r, 120))

    expect(scrollToSpy).not.toHaveBeenCalled()
  })

  // RED-3：程序化滚动不触发"用户滚动"判定（isProgrammaticScroll 防循环）
  it('程序化滚动不触发用户滚动回调', () => {
    const userScrollCallback = vi.fn()

    scope.run(() => {
      const { containerRef, scrollToBottom, onUserScroll } = useScrollToBottom()
      containerRef.value = container
      onUserScroll(userScrollCallback)
      // 程序化滚动
      scrollToBottom()
    })

    // 模拟程序化滚动产生的 scroll 事件
    container.dispatchEvent(new Event('scroll'))

    // 用户滚动回调不应被触发
    expect(userScrollCallback).not.toHaveBeenCalled()
  })

  // RED-3 补充：用户主动滚动应触发回调
  it('用户主动滚动应触发用户滚动回调', () => {
    const userScrollCallback = vi.fn()

    scope.run(() => {
      const { containerRef, onUserScroll } = useScrollToBottom()
      containerRef.value = container
      onUserScroll(userScrollCallback)
    })

    // 用户主动滚动（非程序化）
    container.dispatchEvent(new Event('scroll'))

    expect(userScrollCallback).toHaveBeenCalled()
  })

  // RED-4：RAF 批处理 + 100ms 节流，高频内容变化只滚动一次
  it('高频内容变化时RAF批处理合并为一次滚动', async () => {
    container.scrollTop = 600 // 在底部
    const content = ref('a')

    scope.run(() => {
      const { containerRef, watchContentChange } = useScrollToBottom()
      containerRef.value = container
      watchContentChange(() => content.value)
    })

    // 一个 RAF 周期内多次变化
    content.value = 'ab'
    content.value = 'abc'
    content.value = 'abcd'
    content.value = 'abcde'

    await new Promise((r) => requestAnimationFrame(r))
    await new Promise((r) => setTimeout(r, 10))

    // 批处理后应只调用一次 scrollTo
    expect(scrollToSpy.mock.calls.length).toBe(1)
  })

  // RED-4 补充：100ms 节流间隔
  it('100ms内多次内容变化只滚动一次', async () => {
    vi.useFakeTimers()
    container.scrollTop = 600
    const content = ref('a')

    scope.run(() => {
      const { containerRef, watchContentChange } = useScrollToBottom()
      containerRef.value = container
      watchContentChange(() => content.value)
    })

    // 第一次变化触发滚动（RAF 16ms 后执行）
    content.value = 'ab'
    await vi.advanceTimersByTimeAsync(20)
    const callsAfterFirst = scrollToSpy.mock.calls.length
    expect(callsAfterFirst).toBeGreaterThanOrEqual(1)

    // 50ms 后再变化（距上次滚动 20ms，未到 100ms 间隔）
    content.value = 'abc'
    await vi.advanceTimersByTimeAsync(50)
    // 再次变化（距上次滚动 70ms，仍未到 100ms）
    content.value = 'abcd'
    await vi.advanceTimersByTimeAsync(10)

    // 100ms 间隔内不应再次滚动（scrollTimer 尚未触发）
    expect(scrollToSpy.mock.calls.length).toBe(callsAfterFirst)

    vi.useRealTimers()
  })
})
