/**
 * 代码块复制按钮工具
 *
 * 使用事件委托模式：在容器上绑定一个 click 监听器，
 * 动态新增的 .code-copy-btn 也能响应，无需重新绑定。
 */

/**
 * 事件委托模式：在容器上绑定一个 click 监听器
 * 动态新增的 .code-copy-btn 也能响应，无需重新绑定
 * @returns cleanup 函数，调用后移除监听器
 */
export function setupCodeCopyDelegation(container: HTMLElement): () => void {
    const handleClick = (e: MouseEvent) => {
        const target = e.target as HTMLElement
        const btn = target.closest('.code-copy-btn') as HTMLElement | null
        if (!btn) return
        // 允许在嵌套委托容器中只处理一次，避免父级容器重复复制
        e.stopPropagation()
        // 从相邻的 <code> 元素提取文本内容
        const pre = btn.closest('pre')
        const codeEl = pre?.querySelector('code')
        const code = codeEl?.textContent || ''
        navigator.clipboard.writeText(code).then(() => {
            btn.textContent = '已复制'
            setTimeout(() => { btn.textContent = '复制' }, 1500)
        })
    }
    container.addEventListener('click', handleClick)
    return () => {
        container.removeEventListener('click', handleClick)
    }
}
