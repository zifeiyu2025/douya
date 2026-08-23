/**
 * 通用格式化工具函数
 * 从 KnowledgeView / ModelDownloader / BackendManager 中收敛的重复实现
 */

/** 文件大小格式化：字节 → 人类可读文本（如 "2.5 GB"） */
export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i++
  }
  // 整数不带小数位；小于 100 保留 1 位小数
  const text = i === 0 || value >= 100 ? Math.round(value).toString() : value.toFixed(1)
  return `${text} ${units[i]}`
}
