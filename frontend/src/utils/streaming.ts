// streaming.ts: 流式累积内容的清理工具
// 主要功能：
//   1. cleanStreamingContent: 去除末尾重复段落
//   2. dedupeRepeats: 检测并去除"连续 N 字符完全重复一次"模式
// 注意：不再做 ASCII 算子替换，模型原生输出保持不变

/**
 * cleanStreamingContent: 流式内容清理
 * 去除末尾的重复段落（如果后一段与前一段完全相同）
 */
export function cleanStreamingContent(content: string): string {
    if (!content) return content
    return dedupeRepeats(content)
}

/**
 * dedupeRepeats: 检测并去除"末尾连续重复"模式
 * 场景：模型输出 "8 + 8 = 16 8 + 8 = 16" 时，识别为"前段重复一次"，去除后半
 *
 * 算法：
 *   1. 在 content 末尾向前找最近的"非空白"边界
 *   2. 取最后 N 个字符作为"疑似重复段"
 *   3. 在前 N*2 范围内寻找"是否与前面 N 字符完全相同"
 *   4. 如果找到，截断到前 N 字符
 */
export function dedupeRepeats(content: string): string {
    if (!content || content.length < 20) return content

    // 仅检查末尾 200 字符范围
    const checkStart = Math.max(0, content.length - 200)
    const tail = content.substring(checkStart)

    // 寻找最小重复单元（10-100 字符）
    // 尝试不同的 N 值
    for (let n = 100; n >= 10; n--) {
        if (tail.length < n * 2) continue

        const tailPart = tail.substring(tail.length - n)
        // 在 tailPart 之前查找是否还有相同内容
        const before = tail.substring(0, tail.length - n)
        // 向前找最近的"内容完全相同"段
        if (before.endsWith(tailPart)) {
            // 找到了重复段
            // 计算实际截断位置
            const dupStart = checkStart + (before.length - n)
            return content.substring(0, dupStart)
        }
    }

    return content
}
