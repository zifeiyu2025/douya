/**
 * 通用错误提示工具
 * 统一项目中所有"操作失败"类错误的提示格式，避免风格不一致。
 *
 * 生活类比：像客服中心的统一应答模板，无论哪个部门接到投诉，都用一致的方式回复客户。
 */

import type { MessageApiInjection } from 'naive-ui/es/message/src/MessageProvider'

/**
 * 提取任意错误对象的 message 字符串。
 * 兼容 Error 实例、字符串、{ message: string } 等多种形态。
 */
export function extractErrorMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  if (typeof e === 'string') return e
  if (e && typeof e === 'object' && 'message' in e) {
    const msg = (e as { message: unknown }).message
    if (typeof msg === 'string') return msg
  }
  return String(e)
}

/**
 * 显示统一的"操作失败"错误提示。
 * 自动先调用 destroyAll() 清除旧提示，再显示新错误。
 *
 * @param message NaiveUI message 实例（由调用方通过 useMessage() 获取）
 * @param prefix 操作描述，如"加载知识库数据失败"（不含冒号）
 * @param e 错误对象
 *
 * 示例：
 *   showError(message, '加载知识库数据失败', e)  // 显示 "加载知识库数据失败：xxx"
 */
export function showError(message: MessageApiInjection, prefix: string, e: unknown): void {
  message.destroyAll()
  // 安全实践：避免后端错误原文直接展示给用户，截断过长内容（见安全审查 #35）
  const detail = extractErrorMessage(e)
  const safeDetail = detail.length > 200 ? detail.slice(0, 200) + '...' : detail
  message.error(`${prefix}：${safeDetail}`)
}

/**
 * 显示统一的成功提示。
 * 自动先调用 destroyAll() 清除旧提示，再显示新成功消息。
 */
export function showSuccess(message: MessageApiInjection, content: string): void {
  message.destroyAll()
  message.success(content)
}

/**
 * 显示统一的信息提示。
 * 自动先调用 destroyAll() 清除旧提示，再显示新信息消息。
 */
export function showInfo(message: MessageApiInjection, content: string): void {
  message.destroyAll()
  message.info(content)
}

/**
 * 显示统一的警告提示。
 * 自动先调用 destroyAll() 清除旧提示，再显示新警告消息。
 */
export function showWarning(message: MessageApiInjection, content: string): void {
  message.destroyAll()
  message.warning(content)
}
