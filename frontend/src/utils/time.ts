/**
 * 时间格式化工具
 */

/**
 * 格式化相对时间（如 Sidebar 列表显示）
 * 规则：
 *   < 1 分钟 → "刚刚"
 *   < 1 小时 → "X 分钟前"
 *   < 1 天  → "HH:MM"
 *   < 2 天  → "昨天"
 *   < 7 天  → "周X"
 *   其它    → "M/D"
 */
export function formatRelativeTime(dateStr: string): string {
    const d = new Date(dateStr)
    if (isNaN(d.getTime())) return ''

    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const oneDay = 24 * 60 * 60 * 1000

    if (diff < 60_000) return '刚刚'
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
    if (diff < oneDay) {
        const hour = String(d.getHours()).padStart(2, '0')
        const minute = String(d.getMinutes()).padStart(2, '0')
        return `${hour}:${minute}`
    }
    if (diff < oneDay * 2) return '昨天'
    if (diff < oneDay * 7) {
        const weekDays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
        return weekDays[d.getDay()]
    }
    return `${d.getMonth() + 1}/${d.getDate()}`
}

/**
 * 格式化文件大小（B → KB/MB/GB）
 */
export function formatFileSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
    return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}
