/**
 * 滚动到底部 composable
 * 主流方案：增量滚动 + 用户滚动检测 + 回到底部按钮
 *   - 流式期间增量滚动（scrollBy delta），每帧只滚增量部分，视觉匀速下移（对标千问）
 *   - 用户向上滚动超过阈值时停止自动滚动，显示"回到底部"按钮
 *   - 用户滚回底部时自动恢复自动滚动
 *   - RAF 批处理防止过于频繁的滚动
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
  let observer: MutationObserver | null = null
  // 命名 scroll handler 引用，便于后续 removeEventListener 移除
  let scrollHandler: (() => void) | null = null
  // 当前已绑定 scroll 监听器的元素，用于 containerRef 变化或销毁时正确解绑
  let boundElement: HTMLElement | null = null
  // 增量滚动：记录上次滚动高度，只滚动差值部分
  let lastScrollHeight = 0
  // 上次消息数（提到外层，便于 resetState 重置）
  let prevMsgLen = 0

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
      // 原硬编码 500ms，长内容 smooth 动画可能 >500ms，
      // 标志提前重置期间用户滚动会被误判为"主动上滑"关闭自动滚动。
      // 改为根据 scrollHeight 动态估算时长（每 1000px 约 100ms，上限 2000ms），
      // 保留 500ms 下限兜底短内容场景。
      const animEstimate = Math.min(Math.max(el.scrollHeight / 10, 500), 2000)
      // smooth 动画期间监听用户 wheel/touch 事件，
      // 一旦用户主动滚动立即取消程序化滚动标志，让用户可以接管滚动
      const cancelProg = () => {
        isProgrammaticScroll = false
      }
      el.addEventListener('wheel', cancelProg, { passive: true, once: true })
      el.addEventListener('touchstart', cancelProg, { passive: true, once: true })
      setTimeout(() => {
        isProgrammaticScroll = false
        // once: true 会自动移除，但 setTimeout 可能先触发，手动移除确保清理
        el.removeEventListener('wheel', cancelProg)
        el.removeEventListener('touchstart', cancelProg)
      }, animEstimate)
    } else {
      // auto 滚动是同步的，下一帧重置即可
      requestAnimationFrame(() => {
        isProgrammaticScroll = false
      })
    }
  }

  /**
   * 流式期间增量滚动：只滚动新增内容的高度差
   * 比 scrollTo(scrollHeight) 更平滑，避免整页跳跃（对标千问丝滑下滑）
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
   * 调度滚动：RAF 批处理 + 增量滚动
   * - 增量滚动（scrollBy delta）：每帧只滚新增高度，视觉匀速下移（对标千问）
   */
  function scheduleScroll() {
    if (!isAutoScrollEnabled.value) return
    // RAF 批处理：同一帧内已调度则跳过
    if (rafId !== null) return
    rafId = requestAnimationFrame(() => {
      rafId = null
      scrollByDelta()
    })
  }

  function watchContentChange(getContent: () => unknown) {
    watch(getContent, () => {
      scheduleScroll()
    })
  }

  function watchMessagesLength(getLength: () => number, getLastRole?: () => string) {
    watch(getLength, newLen => {
      if (newLen > prevMsgLen) {
        const lastRole = getLastRole?.()
        if (lastRole === 'user') {
          // 用户发送新消息：强制滚动到底部（绕过节流）
          isAutoScrollEnabled.value = true
          requestAnimationFrame(() => {
            scrollToBottom('smooth')
            const el = containerRef.value
            if (el) lastScrollHeight = el.scrollHeight
          })
        } else {
          // AI 回复完成加入列表：尊重用户查看历史的意图，
          // 仅在启用自动滚动时才滚动，避免把上滑看历史的用户拉回底部
          if (isAutoScrollEnabled.value) {
            requestAnimationFrame(() => {
              scrollToBottom('smooth')
              const el = containerRef.value
              if (el) lastScrollHeight = el.scrollHeight
            })
          }
        }
      } else if (isAutoScrollEnabled.value) {
        scheduleScroll()
      }
      prevMsgLen = newLen
    })
  }

  /**
   * 重置滚动状态：切换会话时调用
   * 重置自动滚动标志和消息计数基准，避免新会话误显示"回到底部"按钮
   */
  function resetState() {
    isAutoScrollEnabled.value = true
    prevMsgLen = 0
    lastScrollHeight = 0
  }

  /** 启动 MutationObserver 监听容器子节点变化 */
  function startObserver() {
    const el = containerRef.value
    if (!el || observer) return
    observer = new MutationObserver(() => {
      scheduleScroll()
    })
    // 移除 characterData 观察——流式期间 v-html 重写 DOM 会让
    // characterData 在每个 token 都回调，虽有 RAF 批处理但仍有微任务开销。
    // childList 已覆盖新消息节点插入；流式内容变化由 watchContentChange
    // 通过响应式 watch 触发 scheduleScroll，无需 characterData。
    observer.observe(el, { childList: true, subtree: true })
  }

  function stopObserver() {
    observer?.disconnect()
    observer = null
  }

  // containerRef 变化时同步绑定 scroll listener
  watch(
    containerRef,
    el => {
      if (el) bindScrollListener()
    },
    { flush: 'sync' }
  )

  onScopeDispose(() => {
    stopObserver()
    if (rafId !== null) cancelAnimationFrame(rafId)
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
    resetState,
    startObserver,
    stopObserver
  }
}
