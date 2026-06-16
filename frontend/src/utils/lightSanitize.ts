/**
 * 轻量级 HTML 安全过滤（用于 Web Worker）
 *
 * 背景：DOMPurify 依赖 DOM API，在 Worker 中不可用。
 * 此处用正则实现基础 XSS 防护，覆盖最常见的攻击向量。
 *
 * 保护范围（已足够处理 Markdown 输出）：
 * - <script> 标签
 * - <iframe> / <object> / <embed> 嵌入
 * - on* 事件属性（如 onclick, onerror）
 * - javascript: 伪协议链接
 * - data:text/html (可执行 HTML)
 *
 * 已知缺口（可接受，因为渲染的来源是 LLM 输出，攻击面有限）：
 * - 不解析 CSS 中的 expression()（现代浏览器已废弃）
 * - 不处理 SVG 中的 <use href="javascript:...">（极少见）
 * - 不处理 HTML 实体绕过
 *
 * 如果未来发现漏洞，优先考虑用 linkedom + DOMPurify 在 Worker 中运行。
 */
const DANGEROUS_PATTERNS: RegExp[] = [
    // script / style 标签
    /<script[\s\S]*?<\/script>/gi,
    /<style[\s\S]*?<\/style>/gi,
    // 嵌入元素
    /<iframe[\s\S]*?<\/iframe>/gi,
    /<object[\s\S]*?<\/object>/gi,
    /<embed[^>]*>/gi,
    /<frame[\s\S]*?<\/frame>/gi,
    // 事件属性 onclick="..." onerror='...' onload=...
    /\s+on\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi,
    // 危险伪协议
    /javascript\s*:/gi,
    /vbscript\s*:/gi,
    // data:text/html 可执行
    /data\s*:\s*text\/html/gi,
]

export function lightSanitize(html: string): string {
    let result = html
    for (const pattern of DANGEROUS_PATTERNS) {
        result = result.replace(pattern, '')
    }
    return result
}
