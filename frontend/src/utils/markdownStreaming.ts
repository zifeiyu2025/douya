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
