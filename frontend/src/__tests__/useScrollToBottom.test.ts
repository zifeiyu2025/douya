/**
 * useScrollToBottom composable 测试
 *
 * 验证流式滚动控制行为：
 * - 用户在底部时内容变化即时滚动到底部（behavior: 'auto'）
 * - 用户向上滚动超过 150px 时停止自动滚动（isAutoScrollEnabled = false）
 * - 用户滚回底部时恢复自动滚动（isAutoScrollEnabled = true）
 * - scheduleScroll 依据 isAutoScrollEnabled 而非 isNearBottom 判定（修复 smooth 动画期间锁定丢失）
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
    // mock scrollTo 同步更新 scrollTop（模拟 behavior: 'auto' 的即时行为）
    // 这样 isNearBottom 在程序化滚动后能反映新位置
    scrollToSpy = vi.spyOn(container, 'scrollTo').mockImplementation(((options?: ScrollToOptions) => {
      if (options && typeof options === 'object') {
        container.scrollTop = options.top ?? container.scrollTop
      }
    }) as Element['scrollTo'])
    document.body.appendChild(container)
  })

  afterEach(() => {
    scope.stop()
    container.remove()
    vi.restoreAllMocks()
  })

  // RED-1：用户在底部时，内容变化应即时滚动到底部
  it('用户在底部时内容变化应即时滚动到底部', async () => {
    // 用户在底部：scrollTop = 600，距底部 = 1000 - 600 - 400 = 0 < 150
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
    // 流式期间应使用 auto 行为（即时滚动）
    const lastCall = scrollToSpy.mock.calls[scrollToSpy.mock.calls.length - 1]
    expect(lastCall[0]).toMatchObject({ behavior: 'auto' })
  })

  // RED-2：用户向上滚动超过 150px 时，不自动滚动
  it('用户向上滚动超过150px时不自动滚动', async () => {
    const content = ref('a')

    scope.run(() => {
      const { containerRef, watchContentChange } = useScrollToBottom()
      containerRef.value = container
      watchContentChange(() => content.value)
    })

    // 模拟用户向上滚动：距底部 = 1000 - 400 - 400 = 200 > 150
    container.scrollTop = 400
    container.dispatchEvent(new Event('scroll'))

    content.value = 'ab'
    await new Promise((r) => requestAnimationFrame(r))
    await new Promise((r) => setTimeout(r, 120))

    expect(scrollToSpy).not.toHaveBeenCalled()
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

  // GREEN-5：用户向上滚动超过阈值后 isAutoScrollEnabled 变为 false
  it('用户向上滚动超过阈值后 isAutoScrollEnabled 变为 false', () => {
    scope.run(() => {
      const { containerRef, isAutoScrollEnabled } = useScrollToBottom()
      containerRef.value = container
      // 初始启用自动滚动
      expect(isAutoScrollEnabled.value).toBe(true)

      // 用户向上滚动：距底部 200 > 150
      container.scrollTop = 400
      container.dispatchEvent(new Event('scroll'))
      expect(isAutoScrollEnabled.value).toBe(false)
    })
  })

  // GREEN-6：用户滚回底部后 isAutoScrollEnabled 恢复 true
  it('用户滚回底部后 isAutoScrollEnabled 恢复 true', () => {
    scope.run(() => {
      const { containerRef, isAutoScrollEnabled } = useScrollToBottom()
      containerRef.value = container

      // 先向上滚动，停止自动滚动
      container.scrollTop = 400
      container.dispatchEvent(new Event('scroll'))
      expect(isAutoScrollEnabled.value).toBe(false)

      // 用户滚回底部：距底部 0 < 150
      container.scrollTop = 600
      container.dispatchEvent(new Event('scroll'))
      expect(isAutoScrollEnabled.value).toBe(true)
    })
  })

  // GREEN-7：scheduleScroll 依据 isAutoScrollEnabled 而非 isNearBottom 判定
  // 这是修复流式锁定丢失的关键：即使 isNearBottom 为 false，只要 isAutoScrollEnabled 为 true 就应滚动
  it('scheduleScroll 依据 isAutoScrollEnabled 而非 isNearBottom 判定', async () => {
    // 不在底部：距底部 200 > 150，isNearBottom 为 false
    container.scrollTop = 400
    const content = ref('a')

    scope.run(() => {
      const { containerRef, watchContentChange, isAutoScrollEnabled } = useScrollToBottom()
      containerRef.value = container
      // 不派发 scroll 事件，isAutoScrollEnabled 保持初始 true
      expect(isAutoScrollEnabled.value).toBe(true)
      watchContentChange(() => content.value)
    })

    content.value = 'ab'
    await new Promise((r) => requestAnimationFrame(r))
    await new Promise((r) => setTimeout(r, 120))

    // 即使 isNearBottom 为 false，因 isAutoScrollEnabled 为 true，仍应滚动
    expect(scrollToSpy).toHaveBeenCalled()
  })
})
