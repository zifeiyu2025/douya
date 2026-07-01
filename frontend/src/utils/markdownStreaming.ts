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
 * 按块级边界拆分内容为稳定块和不稳定块
 * 优先按 \n\n（段落边界）拆分，回退到 \n（行边界）拆分
 * - 段落边界：之前为 stable，之后为 unstable
 * - 行边界（无空行）：之前为 stable，之后为 unstable（最后一行）
 * - 无任何换行：全部为 unstable
 * - 以空行/换行结尾：全部为 stable，unstable 为空
 */
export function splitStableUnstable(content: string): { stable: string; unstable: string } {
    if (!content) return { stable: '', unstable: '' }
    // 优先按 \n\n（段落边界）拆分
    const lastParaBoundary = content.lastIndexOf('\n\n')
    if (lastParaBoundary !== -1) {
        const stable = content.slice(0, lastParaBoundary + 2)
        const unstable = content.slice(lastParaBoundary + 2)
        return { stable, unstable }
    }
    // 回退：按 \n（行边界）拆分，最后一行为 unstable
    const lastLineBoundary = content.lastIndexOf('\n')
    if (lastLineBoundary !== -1) {
        const stable = content.slice(0, lastLineBoundary + 1)
        const unstable = content.slice(lastLineBoundary + 1)
        return { stable, unstable }
    }
    // 无任何换行：全部为不稳定块
    return { stable: '', unstable: content }
}
