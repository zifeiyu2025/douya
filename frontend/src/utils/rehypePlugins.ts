/**
 * Markdown rehype 插件共享模块
 *
 * 主线程（utils/markdown.ts）和 Worker（workers/markdown.worker.ts）共用，
 * 避免逻辑漂移。包含：
 * - rehypeMermaidPre: mermaid 代码块转换为 div.mermaid
 * - rehypeExternalLinks: 外部链接新窗口打开
 * - hastToString: 从 hast 节点提取纯文本
 */
import { visit } from 'unist-util-visit'

/** 从 hast 节点提取纯文本内容（递归子节点） */
export function hastToString(node: any): string {
    if (node.type === 'text') return node.value || ''
    if (Array.isArray(node.children)) {
        return node.children.map(hastToString).join('')
    }
    return ''
}

/**
 * rehypeMermaidPre: 在 rehype-highlight 之前运行
 * 将 <pre><code class="language-mermaid"> 转换为 <div class="mermaid">
 * 避免 rehype-highlight 尝试高亮 mermaid 语法
 */
export function rehypeMermaidPre() {
    return (tree: any) => {
        visit(tree, 'element', (node: any) => {
            if (node.tagName !== 'pre') return

            const codeChild = node.children?.find(
                (child: any) => child.type === 'element' && child.tagName === 'code'
            )
            if (!codeChild) return

            const classes = codeChild.properties?.className
            if (!Array.isArray(classes)) return

            const hasMermaid = classes.some(
                (c: any) => typeof c === 'string' && c === 'language-mermaid'
            )
            if (!hasMermaid) return

            // 提取原始代码文本
            const rawCode = hastToString(codeChild)

            // 转换为 <div class="mermaid">
            node.tagName = 'div'
            node.properties = { className: ['mermaid'] }
            node.children = [{ type: 'text', value: rawCode }]
        })
    }
}

/**
 * rehypeExternalLinks: 外部链接在新窗口打开
 * 仅对 http:// 和 https:// 链接添加 target 和 rel 属性
 */
export function rehypeExternalLinks() {
    return (tree: any) => {
        visit(tree, 'element', (node: any) => {
            if (node.tagName !== 'a') return
            const href = node.properties?.href
            if (typeof href === 'string' && (href.startsWith('http://') || href.startsWith('https://'))) {
                node.properties.target = '_blank'
                node.properties.rel = 'noopener noreferrer'
            }
        })
    }
}
