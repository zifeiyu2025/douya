/**
 * 流式 Markdown 渲染优化工具
 *
 * 核心思想（对标 llama.cpp 原生 webui 的平滑流式体验）：
 * 1. 内容拆分：按块级边界（空行 \n\n）拆分为稳定块和不稳定块
 *    - 稳定块：已完成段落，不会变化，可缓存渲染结果
 *    - 不稳定块：正在生成的最后一段，会持续变化
 * 2. 渲染合并：同一 microtask 内多次调用只处理最新内容，避免脉冲式渲染
 * 3. 稳定块缓存：stable 部分命中缓存时只渲染 unstable，减少 Worker 负载
 * 4. 让出主线程：每次渲染后用 setTimeout(0) 让出，避免长任务阻塞滚动
 */
import { renderMarkdownStreaming } from './markdown'

/**
 * 按块级边界（空行）拆分内容为稳定块和不稳定块
 * 从末尾向前找最后一个 \n\n，之前为 stable，之后为 unstable
 * - 无空行边界：全部为 unstable
 * - 以空行结尾：全部为 stable，unstable 为空
 */
export function splitStableUnstable(content: string): { stable: string; unstable: string } {
    if (!content) return { stable: '', unstable: '' }
    // 从末尾向前找最后一个块级边界（\n\n）
    const lastBoundary = content.lastIndexOf('\n\n')
    if (lastBoundary === -1) {
        // 无边界，全部为不稳定块
        return { stable: '', unstable: content }
    }
    // 边界位置 + 2（跳过 \n\n）为 unstable 起始
    const stable = content.slice(0, lastBoundary + 2)
    const unstable = content.slice(lastBoundary + 2)
    return { stable, unstable }
}

/**
 * 创建流式渲染器
 * @param renderFn 实际渲染函数（默认使用 renderMarkdownStreaming）
 * @returns { render, reset, getCachedHtml }
 *  - render(content): Promise<string> 合并多次调用，缓存 stable，只渲染 unstable
 *  - reset(): 清除缓存（切换对话或重新开始时调用）
 *  - getCachedHtml(): 获取当前缓存的 HTML（同步）
 */
export function createStreamingRenderer(
    renderFn: (content: string) => Promise<string> = renderMarkdownStreaming
) {
    let lastStable = ''
    let lastStableHtml = ''
    let pendingPromise: Promise<string> | null = null
    let latestContent = ''
    // 渲染合并：用 microtask 延迟，同一 microtask 内多次调用只执行最后一次
    let scheduledMicrotask = false
    let resolveFns: Array<(html: string) => void> = []

    async function doRender(content: string): Promise<string> {
        const { stable, unstable } = splitStableUnstable(content)
        let stableHtml: string
        if (stable === lastStable && lastStableHtml) {
            // stable 命中缓存
            stableHtml = lastStableHtml
        } else {
            // stable 变化或首次渲染，需要渲染 stable
            stableHtml = stable ? await renderFn(stable) : ''
            lastStable = stable
            lastStableHtml = stableHtml
        }
        // 渲染 unstable（可能为空）
        const unstableHtml = unstable ? await renderFn(unstable) : ''
        // 让出主线程，避免长任务阻塞滚动
        await yieldToMain()
        return stableHtml + unstableHtml
    }

    function render(content: string): Promise<string> {
        latestContent = content
        // 若已有 pending promise，复用它（合并多次调用）
        if (pendingPromise) {
            return pendingPromise
        }
        // 用 microtask 调度，确保同步代码内多次调用只执行一次
        pendingPromise = new Promise<string>((resolve) => {
            resolveFns.push(resolve)
            if (!scheduledMicrotask) {
                scheduledMicrotask = true
                queueMicrotask(async () => {
                    scheduledMicrotask = false
                    const contentToRender = latestContent
                    try {
                        const html = await doRender(contentToRender)
                        // 通知所有等待的 resolve
                        const fns = resolveFns
                        resolveFns = []
                        pendingPromise = null
                        fns.forEach((fn) => fn(html))
                    } catch (e) {
                        const fns = resolveFns
                        resolveFns = []
                        pendingPromise = null
                        fns.forEach((fn) => fn(''))
                        console.warn('[markdownStreaming] render error:', e)
                    }
                })
            }
        })
        return pendingPromise
    }

    function reset() {
        lastStable = ''
        lastStableHtml = ''
        latestContent = ''
        pendingPromise = null
        resolveFns = []
    }

    function getCachedHtml(): string {
        return lastStableHtml
    }

    return { render, reset, getCachedHtml }
}

/** 让出主线程：用 setTimeout(0) 或 MessageChannel 实现 */
function yieldToMain(): Promise<void> {
    return new Promise((resolve) => {
        // 优先用 MessageChannel（比 setTimeout(0) 更快）
        if (typeof MessageChannel !== 'undefined') {
            const channel = new MessageChannel()
            channel.port1.onmessage = () => {
                channel.port1.close()
                channel.port2.close()
                resolve()
            }
            channel.port2.postMessage(null)
        } else {
            setTimeout(resolve, 0)
        }
    })
}
