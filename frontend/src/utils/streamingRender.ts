/**
 * 纯同步流式 Markdown 格式化
 *
 * 设计原则：
 * - 完全同步，不使用 async/await，保证每帧内容立即渲染（解决"蹦字"问题）
 * - 实时渲染块级格式（标题、列表、引用、分隔线、代码块）和行内格式（粗体、斜体、代码）
 * - 只处理闭合的标记，未闭合的标记保持原样作为文本（避免破损标签）
 * - morphdom DOM Diff 保证结构变化时只更新增量节点，不整体重建
 * - finalizeRender 时会用完整 renderMarkdown（remark + DOMPurify）做最终渲染
 */
import { lightSanitize } from './lightSanitize'

function escapeHtml(str: string): string {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;')
}

/**
 * 处理行内格式（粗体、斜体、行内代码、链接）
 * 只处理闭合的标记，未闭合的保持原样
 */
function renderInline(text: string): string {
    let result = escapeHtml(text)

    // 行内代码（优先处理，避免内部被其他规则误处理）
    result = result.replace(/`([^`]+)`/g, '<code>$1</code>')

    // 链接 [text](url)
    result = result.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')

    // 粗体（** 或 __）
    result = result.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    result = result.replace(/\_\_([^_]+)\_\_/g, '<strong>$1</strong>')

    // 斜体（* 或 _，避免与粗体冲突）
    result = result.replace(/\*([^*]+)\*/g, '<em>$1</em>')
    result = result.replace(/\_([^_]+)\_/g, '<em>$1</em>')

    return result
}

/**
 * 纯同步流式渲染
 *
 * 按行解析，支持：
 * - 标题：# ~ ######
 * - 无序列表：- / * / + 开头
 * - 有序列表：数字. 开头
 * - 引用：> 开头
 * - 分隔线：--- / *** / ___
 * - 代码块：``` 开头和结尾
 * - 段落：其他内容
 *
 * 未闭合的代码块保持原样作为文本，避免破损标签
 * lightSanitize 消毒防 XSS
 */
export function renderStreamingSync(content: string): string {
    if (!content) return ''

    const sanitized = lightSanitize(content)
    const lines = sanitized.split('\n')
    const htmlParts: string[] = []

    let i = 0
    while (i < lines.length) {
        const line = lines[i]
        const trimmed = line.trim()

        // 空行：跳过（段落分隔由 \n\n 处理）
        if (!trimmed) {
            i++
            continue
        }

        // 代码块开始：``` 或 ```language
        if (/^```/.test(trimmed)) {
            // 查找代码块结束
            const codeLines: string[] = [trimmed]
            i++
            let closed = false
            while (i < lines.length) {
                codeLines.push(lines[i])
                if (/^```/.test(lines[i].trim())) {
                    closed = true
                    i++
                    break
                }
                i++
            }
            if (closed) {
                // 闭合的代码块：渲染为 <pre><code>
                const langMatch = codeLines[0].match(/^```(\w*)/)
                const lang = langMatch?.[1] || ''
                const codeContent = codeLines.slice(1, -1).join('\n')
                const langClass = lang ? ` class="language-${lang}"` : ''
                htmlParts.push(`<pre><code${langClass}>${escapeHtml(codeContent)}</code></pre>`)
            } else {
                // 未闭合的代码块：保持原样作为文本，等待后续 token 闭合
                const rawText = codeLines.join('\n')
                htmlParts.push(`<p>${renderInline(rawText)}</p>`)
            }
            continue
        }

        // 标题：# ~ ######
        const headingMatch = trimmed.match(/^(#{1,6})\s+(.*)$/)
        if (headingMatch) {
            const level = headingMatch[1].length
            const text = headingMatch[2]
            htmlParts.push(`<h${level}>${renderInline(text)}</h${level}>`)
            i++
            continue
        }

        // 分隔线：--- / *** / ___（至少3个）
        if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) {
            htmlParts.push('<hr>')
            i++
            continue
        }

        // 引用：> 开头（连续多行合并为一个 blockquote）
        if (/^>\s?/.test(trimmed)) {
            const quoteLines: string[] = []
            while (i < lines.length && /^>\s?/.test(lines[i].trim())) {
                quoteLines.push(lines[i].trim().replace(/^>\s?/, ''))
                i++
            }
            const inner = quoteLines.map(l => renderInline(l)).join('<br>')
            htmlParts.push(`<blockquote>${inner}</blockquote>`)
            continue
        }

        // 无序列表：- / * / + 开头
        if (/^[-*+]\s+/.test(trimmed)) {
            const items: string[] = []
            while (i < lines.length && /^[-*+]\s+/.test(lines[i].trim())) {
                const itemText = lines[i].trim().replace(/^[-*+]\s+/, '')
                items.push(`<li>${renderInline(itemText)}</li>`)
                i++
            }
            htmlParts.push(`<ul>${items.join('')}</ul>`)
            continue
        }

        // 有序列表：数字. 开头
        if (/^\d+\.\s+/.test(trimmed)) {
            const items: string[] = []
            while (i < lines.length && /^\d+\.\s+/.test(lines[i].trim())) {
                const itemText = lines[i].trim().replace(/^\d+\.\s+/, '')
                items.push(`<li>${renderInline(itemText)}</li>`)
                i++
            }
            htmlParts.push(`<ol>${items.join('')}</ol>`)
            continue
        }

        // 段落：收集连续的非块级标记行，合并为一个 <p>
        const paraLines: string[] = [line]
        i++
        while (i < lines.length) {
            const nextLine = lines[i]
            const nextTrimmed = nextLine.trim()
            if (!nextTrimmed) {
                // 空行：段落结束
                break
            }
            // 遇到块级标记：段落结束
            if (
                /^#{1,6}\s+/.test(nextTrimmed) ||
                /^```/.test(nextTrimmed) ||
                /^(-{3,}|\*{3,}|_{3,})$/.test(nextTrimmed) ||
                /^>\s?/.test(nextTrimmed) ||
                /^[-*+]\s+/.test(nextTrimmed) ||
                /^\d+\.\s+/.test(nextTrimmed)
            ) {
                break
            }
            paraLines.push(nextLine)
            i++
        }
        const inner = paraLines.map(l => renderInline(l)).join('<br>')
        htmlParts.push(`<p>${inner}</p>`)
    }

    return htmlParts.join('')
}
