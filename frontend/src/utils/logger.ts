/**
 * 统一日志工具（L-8 修复）
 *
 * 安全考虑：原代码 20+ 处直接 console.error 打印后端错误细节（含文件路径、堆栈），
 * 对打开 DevTools 的用户可见。本工具在生产环境压缩错误细节，仅保留消息前缀，
 * 避免泄漏后端实现细节；开发环境（import.meta.env.DEV）保留完整信息便于调试。
 *
 * 生活类比：就像客服对外通报"系统繁忙"而非把错误日志直接贴给客户，
 * 内部技术人员仍可查看完整日志定位问题。
 */

/** 是否为开发环境（Vite 注入，生产构建为 false） */
const isDev = import.meta.env.DEV

/**
 * 记录错误日志
 * @param prefix 错误描述前缀（如"加载会话列表失败"），始终输出
 * @param err 错误对象，生产环境仅输出类型不输出细节，开发环境输出完整信息
 */
export function logError(prefix: string, err: unknown): void {
  if (isDev) {
    // 开发环境：完整错误信息便于调试
    console.error(prefix, err)
  } else {
    // 生产环境：仅输出前缀和错误类型，不泄漏后端细节（文件路径/堆栈等）
    const errType = err instanceof Error ? err.name : typeof err
    console.error(`${prefix} (${errType})`)
  }
}
