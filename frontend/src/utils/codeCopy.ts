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
