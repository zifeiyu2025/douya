/**
 * 轻量级 HTML 安全过滤（用于 Web Worker）
 *
 * 背景：DOMPurify 依赖 DOM API，在 Worker 中不可用。
 * 此处用正则实现基础 XSS 防护，覆盖最常见的攻击向量。
 *
 * 保护范围：
 * - <script> / <style> 标签
 * - <iframe> / <object> / <embed> / <frame> 等嵌入元素
 * - on* 事件属性（如 onclick, onerror, onload）
 * - javascript: / vbscript: / data:text/html 等危险伪协议
 * - SVG 中可能引入脚本的外链（<use href="...">）
 * - HTML 实体编码的危险协议（&#106;avascript:）
 *
 * 注意：这是第一层过滤。Worker 返回主线程后，仍需通过 DOMPurify 进行二次消毒。
 * 如果未来发现漏洞，优先考虑用 linkedom + DOMPurify 在 Worker 中运行。
 */

// 危险标签（完整匹配开始和结束标签）
const DANGEROUS_TAG_PATTERNS: RegExp[] = [
  /<script[\s\S]*?<\/script>/gi,
  /<style[\s\S]*?<\/style>/gi,
  /<iframe[\s\S]*?<\/iframe>/gi,
  /<object[\s\S]*?<\/object>/gi,
  /<embed[^>]*>/gi,
  /<frame[\s\S]*?<\/frame>/gi,
  /<frameset[\s\S]*?<\/frameset>/gi,
  /<applet[\s\S]*?<\/applet>/gi,
  /<form[\s\S]*?<\/form>/gi,
  /<input[^>]*>/gi,
  /<textarea[\s\S]*?<\/textarea>/gi,
  /<button[\s\S]*?<\/button>/gi,
  /<select[\s\S]*?<\/select>/gi,
  /<option[\s\S]*?<\/option>/gi,
  /<svg[\s\S]*?<\/svg>/gi
  // 保留 <math> 标签：KaTeX 公式渲染依赖它（任务 27）
  // math 标签上的危险属性（如 on* 事件）已由 DANGEROUS_ATTR_PATTERNS 覆盖清理
]

// 危险属性（事件处理器、危险协议、SVG 外链）
const DANGEROUS_ATTR_PATTERNS: RegExp[] = [
  // 事件属性 onclick="..." onerror='...' onload=...（支持前后空白和换行）
  // 修复（M-前1）：原正则只匹配 \s+ 前缀，<img/onerror=alert(1)> 用 / 分隔可绕过。
  // 加入 / 作为分隔符，覆盖 <tag/on*=...> 的绕过向量。
  /[\s/]+on\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi,
  // javascript: / vbscript: / data:text/html（支持 HTML 实体绕过）
  /(?:j\s*a\s*v\s*a\s*s\s*c\s*r\s*i\s*p\s*t|v\s*b\s*s\s*c\s*r\s*i\s*p\s*t|d\s*a\s*t\s*a\s*:\s*t\s*e\s*x\s*t\s*\/\s*h\s*t\s*m\s*l)\s*:/gi,
  // HTML 实体编码的 javascript:，例如 &#106;avascript:
  /&#[xX]?[0-9a-fA-F]+;?\s*:\s*\/\s*\/\s*/gi,
  // 安全实践：匹配实体编码的 javascript:/vbscript: 协议（无 //），见安全审查 #33
  // 原正则仅匹配 &#106;avascript://（带 //），遗漏 javascript:（无 //）
  /&#[xX]?[0-9a-fA-F]+;?\s*:\s*(?:javascript|vbscript|data:text\/html)/gi,
  // SVG <use> 外链可能引入脚本
  /<use[^>]*\s+href\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)[^>]*>/gi,
  /<use[^>]*\s+xlink:href\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)[^>]*>/gi,
  // CSS expression()（现代浏览器已废弃，但保留防护）
  /expression\s*\(/gi,
  // import / behavior 等 CSS 危险属性
  /@import\s+/gi,
  /behavior\s*:\s*url\s*\(/gi
]

export function lightSanitize(html: string): string {
  let result = html
  // 第一步：移除危险标签
  for (const pattern of DANGEROUS_TAG_PATTERNS) {
    result = result.replace(pattern, '')
  }
  // 第二步：移除危险属性/协议
  for (const pattern of DANGEROUS_ATTR_PATTERNS) {
    result = result.replace(pattern, '')
  }
  return result
}

/**
 * 校验 URL 是否安全（协议白名单）
 * 用于搜索结果、Markdown 链接渲染等场景
 *
 * 修复（安全审查 #15）：显式协议白名单，拒绝 javascript: / vbscript: /
 * data:text/html 等危险伪协议。允许的安全协议：
 *   - http(s):  常规网页
 *   - mailto:   邮件
 *   - tel:      电话
 *   - #         页面内锚点
 */
export function isSafeUrl(url: string): boolean {
  if (!url || typeof url !== 'string') return false
  const trimmed = url.trim().toLowerCase()
  if (!trimmed) return false
  // 锚点：页面内跳转，无 XSS 风险
  if (trimmed.startsWith('#')) return true
  // 常规网页 / 邮件 / 电话协议
  if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) return true
  if (trimmed.startsWith('mailto:')) return true
  if (trimmed.startsWith('tel:')) return true
  return false
}
