export function bindCodeCopyButtons(el: HTMLElement) {
    const btns = el.querySelectorAll('.code-copy-btn')
    btns.forEach((btn) => {
        btn.addEventListener('click', () => {
            const code = (btn as HTMLElement).getAttribute('data-code') || ''
            const decoded = code.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'")
            navigator.clipboard.writeText(decoded).then(() => {
                btn.textContent = '已复制'
                setTimeout(() => { btn.textContent = '复制' }, 1500)
            })
        })
    })
}
