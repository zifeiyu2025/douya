/**
 * 代码块复制按钮工具
 *
 * 两种模式：
 * 1. bindCodeCopyButtons（旧）：全量 querySelectorAll 绑定，每次流式更新都需调用
 * 2. setupCodeCopyDelegation（新）：事件委托，只在容器绑定一次，动态新增按钮自动响应
 *
 * 流式场景推荐用 setupCodeCopyDelegation，避免每次更新都全量重绑。
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

/**
 * 全量绑定模式（旧，兼容历史调用）
 * 遍历所有 .code-copy-btn 逐个绑定 click 事件
 */
export function bindCodeCopyButtons(el: HTMLElement) {
    const btns = el.querySelectorAll('.code-copy-btn')
    btns.forEach((btn) => {
        btn.addEventListener('click', () => {
            // 从相邻的 <code> 元素提取文本内容
            // 比使用 data-code 属性更可靠，避免 HTML 属性转义问题
            const pre = (btn as HTMLElement).closest('pre')
            const codeEl = pre?.querySelector('code')
            const code = codeEl?.textContent || ''
            navigator.clipboard.writeText(code).then(() => {
                btn.textContent = '已复制'
                setTimeout(() => { btn.textContent = '复制' }, 1500)
            })
        })
    })
}
