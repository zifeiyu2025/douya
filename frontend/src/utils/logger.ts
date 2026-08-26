/**
 * 统一日志工具
 *
 * 安全考虑：原代码 20+ 处直接 console.error 打印后端错误细节（含文件路径、堆栈），
 * 对打开 DevTools 的用户可见。本工具在生产环境压缩错误细节，仅保留消息前缀，
 * 避免泄漏后端实现细节；开发环境（import.meta.env.DEV）保留完整信息便于调试。
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

/**
 * 记录警告日志
 *
 * 用于降级、兼容性回退等非致命情况。
 *
 * @param prefix 警告描述前缀，始终输出
 * @param detail 详细信息（可选），生产环境仅输出前缀，开发环境输出完整信息
 */
export function logWarn(prefix: string, detail?: unknown): void {
  if (isDev) {
    console.warn(prefix, detail)
  } else {
    // 生产环境：仅输出前缀，不泄漏细节
    console.warn(prefix)
  }
}
