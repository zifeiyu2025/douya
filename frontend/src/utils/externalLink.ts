/**
 * 外部链接统一打开工具（F-1.13 抽取）
 *
 * 安全实践（基于 VUE-XSS-004 + SEC-L-1）：
 *  - 所有 http/https 链接必须走系统默认浏览器（BrowserOpenURL），禁止 WebView 内部导航
 *  - 调用前用 isSafeUrl 校验协议白名单（http/https/mailto/tel），防止 javascript:/file: 等危险协议
 *
 * 生活类比：像快递柜的"代收点"——所有外来包裹（链接）都先在这里验货（isSafeUrl），
 * 确认安全后再派送到正确的地址（系统浏览器），不让陌生包裹直接进你家（WebView）。
 */
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { isSafeUrl } from './lightSanitize'

/**
 * 安全打开外部链接
 * 校验通过后走系统默认浏览器，校验失败静默忽略（已记录在 isSafeUrl 内部逻辑）
 * @param url 待打开的 URL
 */
export function openExternal(url: string): void {
    if (isSafeUrl(url)) {
        BrowserOpenURL(url)
    }
}
