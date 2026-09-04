// 上下文长度滑块的档位与工具函数（自 SettingsView 迁出，纯数据 + 纯函数，无组件状态）
// 上限 131072 与后端校验对齐（internal/config floatFieldRules: context_size 0-131072）：
// 此前含 262144 档，用户拉到 256K 保存会被后端 Validate 拒绝并静默重置回默认值。
export const contextSizeSteps = [2048, 4096, 8192, 16384, 32768, 65536, 131072]
export const contextSizeMarks: Record<number, string> = {
  0: '2K',
  1: '4K',
  2: '8K',
  3: '16K',
  4: '32K',
  5: '64K',
  6: '128K'
}

export function formatContextSize(size: number): string {
  if (size >= 1024) {
    const k = size / 1024
    return k >= 1024 ? `${(k / 1024).toFixed(0)}M` : `${k >= 100 ? Math.round(k) : k}K`
  }
  return `${size}`
}

export function findClosestStepIndex(size: number): number {
  let closest = 0
  let minDiff = Math.abs(contextSizeSteps[0] - size)
  for (let i = 1; i < contextSizeSteps.length; i++) {
    const diff = Math.abs(contextSizeSteps[i] - size)
    if (diff < minDiff) {
      minDiff = diff
      closest = i
    }
  }
  return closest
}
