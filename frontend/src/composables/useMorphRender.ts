/**
 * 流式渲染 composable（stable/unstable 分块缓存 + 实时格式化）
 *
 * 参考 llama.cpp webui 的 MarkdownContent.svelte 渲染策略：
 * - 流式中实时显示 markdown 格式（代码高亮、表格、列表等）
 * - stable/unstable 分块：除最后一个 token 外都是 stable，缓存其 HTML
 * - append mode 检测：newContent.startsWith(previousContent) 时复用 stable 缓存
 * - 未闭合代码围栏补全：让 marked 识别为 code token，走 renderer.code 高亮
 *
 * 在现有 marked 管道上实现（不引入 remark/rehype）：
 * - marked.lexer 解析为 tokens
 * - marked.Parser.parse 渲染单个 token 为 HTML
 * - sanitizeHtml (DOMPurify) 消毒每个 token 的 HTML
 *
 * 生活类比：像翻译一本书，已经翻完的章节（stable blocks）存档，
 * 只翻译最新的一页（unstable block）。下次新内容来时，翻过的章节直接取档。
 *
 * 用法：
 * ```ts
 * const { containerRef, bind, finalizeRender } = useMorphRender()
 * // 模板中：<div ref="streamingContainer" class="markdown-body streaming" />
 * bind(() => streamingContent.value)
 * finalizeRender()  // 流式结束时调用，全量渲染确保完整
 * ```
 */
import { ref, watch, onScopeDispose, type Ref } from 'vue'
import { marked, Parser } from 'marked'
import { renderMarkdown, sanitizeHtml } from '../utils/markdown'

// marked token 的最小类型约束（marked 的 Token 类型较复杂，用结构化约束即可）
interface MarkedToken {
  type: string
  raw?: string
  [key: string]: unknown
}

export function useMorphRender() {
  const containerRef = ref<HTMLElement | null>(null)
  let latestContent = ''
  let rafId: number | null = null
  let isFinalized = false

  // stable blocks 缓存：token key → HTML
  // 绑定到组件实例，会话切换/组件销毁时自动清理
  const blockCache = new Map<string, string>()
  let previousContent = ''

  /**
   * 简单字符串哈希（djb2 变种）
   * 用于生成 token 缓存 key，比直接用 raw 字符串做 key 更省内存
   */
  function simpleHash(str: string): string {
    let hash = 5381
    for (let i = 0; i < str.length; i++) {
      hash = (hash << 5) + hash + str.charCodeAt(i)
      hash |= 0
    }
    return (hash >>> 0).toString(36)
  }

  /**
   * 生成 token 的缓存 key
   * 用 type + raw 的哈希 + index，确保内容不变时 key 不变
   */
  function tokenKey(token: MarkedToken, index: number): string {
    const raw = token.raw || ''
    return `${token.type}:${simpleHash(raw)}:${index}`
  }

  /**
   * 检测末尾未闭合的代码围栏，补全闭合
   *
   * 流式中代码块的结尾 ``` 还没到，marked 会把它当作普通段落处理，
   * 导致没有语法高亮。补全闭合让 marked 识别为 code token，走 renderer.code 高亮。
   *
   * 安全性：
   * - 行内代码（`code` 或 ``code``）不以 ``` 开头，不会被误判
   * - 补全的 ``` 只是让 marked 识别为代码块，不改变实际内容
   * - finalizeRender 会用原始 content 全量渲染，最终结果正确
   */
  function closeUnclosedFences(content: string): string {
    let inFence = false
    let fenceMarker = ''
    const lines = content.split('\n')
    for (const line of lines) {
      const trimmed = line.trimStart()
      // 检测围栏开始（``` 或 ~~~，3个或以上相同字符）
      const fenceMatch = trimmed.match(/^(`{3,}|~{3,})/)
      if (fenceMatch) {
        const marker = fenceMatch[1].slice(0, 3) // 取前3个字符作为围栏标记
        if (!inFence) {
          inFence = true
          fenceMarker = marker
        } else if (trimmed.startsWith(fenceMarker)) {
          inFence = false
          fenceMarker = ''
        }
      }
    }
    if (inFence) {
      return content + '\n' + fenceMarker
    }
    return content
  }

  /**
   * 渲染单个 token 为 HTML（带消毒）
   *
   * Parser.parse 每次创建新实例，无状态依赖，安全用于单 token 渲染
   */
  function renderToken(token: MarkedToken): string {
    try {
      const html = Parser.parse([token] as any) as string
      return sanitizeHtml(html)
    } catch (_) {
      return ''
    }
  }

  /**
   * 分块渲染：stable blocks 缓存 + unstable block 实时渲染
   *
   * 核心优化（对标 llama.cpp webui）：
   * 1. marked.lexer 解析为 tokens
   * 2. 除最后一个 token 外都是 stable，走缓存或首次渲染后缓存
   * 3. 最后一个 token（unstable）每次重新渲染（流式中持续变化）
   * 4. append mode 下复用 stable 缓存，避免重复渲染
   */
  function doRenderSync(content: string) {
    const el = containerRef.value
    if (!el) return

    // 补全未闭合围栏，让 marked 识别为 code token
    const closed = closeUnclosedFences(content)

    // 检测 append mode：新内容是否是旧内容的追加
    const appendMode = previousContent.length > 0 && closed.startsWith(previousContent)
    if (!appendMode) {
      // 内容结构变化（非追加），清空缓存重新渲染
      blockCache.clear()
    }

    // 解析为 tokens（marked.lexer 使用 marked.defaults 配置：gfm、breaks、renderer）
    const tokens = marked.lexer(closed) as unknown as MarkedToken[]

    // 除最后一个 token 外都是 stable
    const stableCount = Math.max(tokens.length - 1, 0)
    const htmls: string[] = []

    for (let i = 0; i < stableCount; i++) {
      const token = tokens[i]
      const key = tokenKey(token, i)
      let html = blockCache.get(key)
      if (!html) {
        html = renderToken(token)
        blockCache.set(key, html)
      }
      htmls.push(html)
    }

    // unstable block（最后一个 token）每次重新渲染
    if (tokens.length > stableCount) {
      htmls.push(renderToken(tokens[stableCount]))
    }

    el.innerHTML = htmls.join('')
    previousContent = closed
  }

  /**
   * 最终渲染：流式结束时用 renderMarkdown 全量渲染
   *
   * 流式中已实时格式化，结束时全量渲染确保最终结果完整正确
   * （补全的围栏已移除，用原始 content 渲染）
   */
  async function doFinalRender(content: string) {
    try {
      const html = await renderMarkdown(content)
      const el = containerRef.value
      if (el) el.innerHTML = html
    } catch (_) {
      // 降级：保持流式中的渲染结果
    } finally {
      isFinalized = false
    }
  }

  function finalizeRender() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
    isFinalized = true
    blockCache.clear()
    return doFinalRender(latestContent)
  }

  /**
   * 调度渲染：RAF 合帧
   *
   * 同一帧内多次 token 到达，只调度一次 RAF，回调时读取最新内容
   */
  function scheduleRender(content: string) {
    latestContent = content
    if (isFinalized) return
    if (rafId !== null) return
    rafId = requestAnimationFrame(() => {
      rafId = null
      doRenderSync(latestContent)
    })
  }

  /**
   * 清空容器，重置状态
   */
  function clear() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
    isFinalized = false
    blockCache.clear()
    const el = containerRef.value
    if (el) el.innerHTML = ''
    latestContent = ''
    previousContent = ''
  }

  /**
   * 绑定响应式内容源
   * 内容变化时自动调度渲染；空内容时清空容器
   *
   * 容器延迟挂载修复：容器由 v-if="streamingContent" 控制，
   * 首次对话时 containerRef 可能在 RAF 回调时还未赋值。
   * 额外监听 containerRef，挂载后补偿渲染。
   */
  function bind(getContent: () => string) {
    watch(
      getContent,
      newContent => {
        if (!newContent) {
          clear()
          return
        }
        scheduleRender(newContent)
      },
      { immediate: true }
    )
    // 容器挂载后补偿渲染：解决 v-if 延迟挂载导致的首次内容丢失
    watch(containerRef, newEl => {
      if (newEl && latestContent && !isFinalized && rafId === null) {
        scheduleRender(latestContent)
      }
    })
  }

  onScopeDispose(() => {
    if (rafId !== null) cancelAnimationFrame(rafId)
    blockCache.clear()
  })

  return {
    containerRef: containerRef as Ref<HTMLElement | null>,
    bind,
    clear,
    finalizeRender
  }
}
