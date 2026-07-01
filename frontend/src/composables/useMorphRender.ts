/**
 * 流式渲染 composable（轻量版）
 *
 * 相比旧版（morphdom DOM Diff）：
 * - 移除 morphdom 依赖（项目记忆记载 morphdom 在流式场景导致 DOM diff 闪烁）
 * - 改用 v-html 直接设置 innerHTML + CSS containment（contain: style）
 * - 保留 RAF 合帧逻辑（60fps 节流）
 *
 * 生活类比：旧版像是一个"精细修理工"，每次都对比新旧家具只换变化的部分；
 * 新版像是一个"直接替换工"，把整个房间的内容一次性换掉。
 * 看似粗暴，但因为 marked 渲染非常快，加上 CSS 隔离（contain: style），
 * 浏览器只需要重新计算样式，不需要重排整个页面，性能反而更好。
 *
 * 用法：
 * ```ts
 * const { containerRef, bind, finalizeRender } = useMorphRender()
 * // 模板中：<div ref="streamingContainer" class="markdown-body streaming" />
 * bind(() => streamingContent.value)
 * finalizeRender()  // 流式结束时调用
 * ```
 */
import { ref, watch, onScopeDispose, type Ref } from 'vue'
import { marked } from 'marked'
import { lightSanitize } from '../utils/lightSanitize'
import { renderMarkdown, escapeHtml } from '../utils/markdown'

export function useMorphRender() {
    const containerRef = ref<HTMLElement | null>(null)
    let latestContent = ''
    let rafId: number | null = null
    let isFinalized = false
    // 已完成段落的 DOM 节点缓存（稳定块）
    // 生活类比：像快递员把已打包好的包裹（稳定段落）码上车，下次只往最后那个未封口的箱子里塞新东西
    let stableBlocks: HTMLElement[] = []

    /**
     * 按块级闭合标签拆分 HTML 为段落
     *
     * marked 输出的 HTML 中，段落由 </p>、</pre>、</h1-6>、</ul>、</ol>、</table>、</blockquote>、</div>
     * 等块级标签闭合。我们以这些标签为边界拆分，每段包含完整的开闭标签。
     *
     * 注意：不能用 \n\n 简单拆分，因为 <pre> 块内部可能包含 \n\n（代码缩进）
     *
     * 生活类比：像把一串糖葫芦按竹节切断，每段都是完整的一颗，不会切到山楂中间
     */
    function splitParagraphs(html: string): string[] {
        const blocks: string[] = []
        // 用捕获组保留分隔符，按块级闭合标签拆分
        const parts = html.split(/(<\/(?:p|pre|h[1-6]|ul|ol|table|blockquote|div)>\s*)/)
        let current = ''
        for (let i = 0; i < parts.length; i++) {
            current += parts[i]
            // 奇数索引是闭合标签（捕获组），其后为一个完整段落边界
            if (i % 2 === 1) {
                blocks.push(current)
                current = ''
            }
        }
        // 收集尾部未闭合的内容（如最后一段还在流式生成中）
        if (current.trim()) blocks.push(current)
        return blocks
    }

    /**
     * 将 HTML 字符串应用到容器（段落级增量更新）
     *
     * 核心思路：
     * - 按 \n\n 拆分出的段落中，除最后一段外都是"稳定块"（已完成）
     * - 稳定块只渲染一次，缓存 DOM 节点，后续帧不再触碰
     * - 最后一段是"不稳定块"，每帧 innerHTML 替换
     * - 这样每帧复杂度 O(Δ)（仅最后一段），而非 O(N)（整个文档）
     *
     * 生活类比：像写文章，已写完的段落不动，只重写正在写的那一段
     *
     * 安全实践：见安全审查 #18，按段落拆分 + 稳定块缓存
     *
     * 配合 CSS containment（contain: style，已在 MessageList.vue .markdown-body.streaming 配置）
     * 隔离样式重计算，避免重排扩散到整页
     */
    function applyHtml(html: string) {
        const el = containerRef.value
        if (!el) return

        // 1. 拆分段落
        const paragraphs = splitParagraphs(html)

        // 空内容：清空容器和缓存
        if (paragraphs.length === 0) {
            el.innerHTML = ''
            stableBlocks = []
            return
        }

        // 2. 计算稳定块数量（除最后一段外）
        const stableCount = paragraphs.length - 1

        // 3. 检测是否需要全量重建
        //    - 稳定块数量减少（流式回退，理论不应发生但防御）
        //    - 从"无稳定块"切换到"有稳定块"（1段→2段）：上次直接 innerHTML，需清空后追加 div
        const needRebuild =
            stableBlocks.length > stableCount ||
            (stableBlocks.length === 0 && stableCount > 0)

        if (needRebuild) {
            el.innerHTML = ''
            stableBlocks = []
        }

        // 4. 增量追加新增的稳定块（O(Δ)）
        //    关键修复：当段落从 N 段增长到 N+1 段时，原有 .unstable-block
        //    （其内容正是上一帧最后一行 = 本次要凝固的 paragraphs[idx]）
        //    必须"原地提升"为 stable-block，而不是新建在末尾。
        //    否则旧 unstable-block 留在原位，新 stable-block 追加其后，
        //    第5步又会把 unstable-block 更新为新段落内容，
        //    导致 DOM 顺序变成 A、C、B（先稳定行→末尾新稳定行→原位的更新行）。
        while (stableBlocks.length < stableCount) {
            const idx = stableBlocks.length
            const blockHtml = paragraphs[idx]
            const unstableDiv = el.querySelector('.unstable-block') as HTMLElement | null
            if (unstableDiv) {
                // 原地提升：改 class 即可，内容已是该段；为防御性也覆盖一次
                unstableDiv.className = 'stable-block'
                unstableDiv.innerHTML = blockHtml
                stableBlocks.push(unstableDiv)
            } else {
                const div = document.createElement('div')
                div.className = 'stable-block'
                div.innerHTML = blockHtml
                el.appendChild(div)
                stableBlocks.push(div)
            }
        }

        // 5. 更新最后一段（不稳定块）
        //    注意：第4步若把 unstable-block 提升了，这里会新建一个新的 unstable-block
        const lastBlock = paragraphs[paragraphs.length - 1]
        if (stableBlocks.length > 0) {
            // 有稳定块：最后一段作为独立 .unstable-block div，仅替换其 innerHTML
            let lastDiv = el.querySelector('.unstable-block') as HTMLElement | null
            if (!lastDiv) {
                lastDiv = document.createElement('div')
                lastDiv.className = 'unstable-block'
                el.appendChild(lastDiv)
            }
            lastDiv.innerHTML = lastBlock
        } else {
            // 无稳定块（仅一段）：直接设置 innerHTML，避免多余包装
            el.innerHTML = lastBlock
        }
    }

    /**
     * 同步渲染：marked.parse 生成 HTML + lightSanitize 临时消毒
     *
     * 流式期间用 lightSanitize（正则实现）替代 DOMPurify，性能更快
     * 流式结束后 finalizeRender 会用 renderMarkdown（marked + DOMPurify）二次消毒
     *
     * 生活类比：流式期间像"草稿"，用快速但不完美的方式消毒；
     * 最终渲染像"定稿"，用严格的方式彻底消毒。
     */
    function doRenderSync(content: string) {
        try {
            // marked.parse 默认同步，返回 string
            const html = marked.parse(content, { async: false }) as string
            applyHtml(lightSanitize(html))
        } catch (err) {
            console.warn('[stream-render] sync render failed, fallback:', err)
            applyHtml(escapeHtml(content))
        }
    }

    /**
     * 最终渲染：流式结束时用 renderMarkdown（marked + DOMPurify）渲染
     *
     * 流式期间用 lightSanitize 临时消毒，结束时用 DOMPurify 正式消毒
     * 确保最终内容安全无 XSS 风险
     *
     * 注意：最终渲染是全量消毒，需清空段落缓存并重置容器，
     * 让 applyHtml 走"无稳定块"路径直接设置 innerHTML，避免 .unstable-block 包装残留
     */
    async function doFinalRender(content: string) {
        try {
            const html = await renderMarkdown(content)
            // 重置段落缓存：最终渲染是全量的，不再需要增量更新
            stableBlocks = []
            const el = containerRef.value
            if (el) el.innerHTML = ''
            applyHtml(html)
        } catch (err) {
            console.warn('[stream-render] final render failed:', err)
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
        return doFinalRender(latestContent)
    }

    /**
     * 调度渲染：RAF 合帧
     *
     * 生活类比：就像快递员攒一批包裹一起送，不会来一个送一个。
     * 同一帧内多次 token 到达，只调度一次 RAF，回调时读取最新内容。
     *
     * 这样保证 60fps（每帧最多渲染一次），避免高频 token 导致主线程阻塞
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
     *
     * 重置段落缓存（stableBlocks），
     * 避免下次渲染复用过期的 DOM 节点
     */
    function clear() {
        if (rafId !== null) {
            cancelAnimationFrame(rafId)
            rafId = null
        }
        isFinalized = false
        const el = containerRef.value
        if (el) el.innerHTML = ''
        latestContent = ''
        stableBlocks = []
    }

    /**
     * 绑定响应式内容源
     * 内容变化时自动调度渲染；空内容时清空容器
     *
     * 容器延迟挂载修复：
     * 容器由 v-if="streamingContent" 控制，首次对话时流程是：
     *   streamingContent="" → watch 触发 clear()（容器不存在，跳过）
     *   首 token 到达 → streamingContent="第" → watch 触发 scheduleRender
     *   → Vue patch 创建容器 DOM → containerRef 赋值
     *   → RAF 回调 applyHtml（此时 containerRef 应已有效）
     * 但如果 watch pre-flush 在 DOM patch 之前触发 RAF，RAF 回调执行时
     * containerRef 可能为 null，applyHtml 直接 return，内容丢失。
     * 这里额外监听 containerRef，当 ref 从 null 变为非 null 时，
     * 如果有 pending 内容，重新触发渲染。
     */
    function bind(getContent: () => string) {
        watch(
            getContent,
            (newContent) => {
                if (!newContent) {
                    clear()
                    return
                }
                scheduleRender(newContent)
            },
            { immediate: true },
        )
        // 容器挂载后补偿渲染：解决 v-if 延迟挂载导致的首次内容丢失
        watch(containerRef, (newEl) => {
            if (newEl && latestContent && !isFinalized && rafId === null) {
                scheduleRender(latestContent)
            }
        })
    }

    onScopeDispose(() => {
        if (rafId !== null) cancelAnimationFrame(rafId)
    })

    return {
        containerRef: containerRef as Ref<HTMLElement | null>,
        bind,
        clear,
        finalizeRender,
    }
}
