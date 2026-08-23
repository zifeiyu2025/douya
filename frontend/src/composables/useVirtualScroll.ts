/**
 * 虚拟滚动开关 + 小会话自动降级（改进计划 C-4）
 *
 * 生活类比：
 *   虚拟滚动就像餐厅只做顾客看得见的那几桌菜——几百桌的大宴会厅（超长对话）
 *   能省下大量后厨人力（DOM 渲染开销）；但小餐馆（小会话）本来就只有几桌，
 *   全做反而更省事。所以：默认走"智能模式"，大会话自动启用虚拟滚动，小会话
 *   自动退回普通渲染；用户仍可通过 localStorage 显式关闭该能力。
 *
 * 设计要点：
 *   - C-4 转正：enableVirtualScroll 语义从"实验性开关"升级为"允许虚拟滚动"，
 *     默认 true；仅当显式写入 'false' 时才整体关闭（老用户已写入的 'true'/'false'
 *     均被尊重，行为兼容）
 *   - 小会话自动降级：消息数 < VIRTUAL_ENTER_THRESHOLD 时普通渲染；
 *     已进入虚拟模式后，消息数回落到 VIRTUAL_EXIT_THRESHOLD 以下才退出。
 *     两个阈值构成"滞回"，避免在临界点附近删除消息导致渲染路径反复切换抖动
 *   - 模块级单例 ref，跨组件共享同一响应式状态
 */
import { ref, watch, computed, type Ref, type ComputedRef } from 'vue'

const STORAGE_KEY = 'douya-enable-virtual-scroll'

// 进入虚拟滚动的最小消息数（低于此值普通渲染性能足够，且规避虚拟模式的占位气泡布局特例）
export const VIRTUAL_ENTER_THRESHOLD = 50
// 退出虚拟滚动的回落线（与进入线构成滞回区间 [40, 50)）
export const VIRTUAL_EXIT_THRESHOLD = 40

// 模块级单例：转正后默认允许；显式设为 'false' 才整体关闭
const enableVirtualScroll = ref(localStorage.getItem(STORAGE_KEY) !== 'false')

// 持久化：开关变化即写回 localStorage
watch(enableVirtualScroll, val => {
  localStorage.setItem(STORAGE_KEY, String(val))
})

/**
 * 不关心会话规模时的用法（保持旧签名兼容）：
 *   const { enableVirtualScroll } = useVirtualScroll()
 * 关心会话规模的用法（C-4 推荐路径）：
 *   const { shouldUseVirtualScroll } = useVirtualScroll(messageCountRef)
 */
// 重载签名：让类型系统按入参区分两种返回形状，
// 否则联合类型会把 shouldUseVirtualScroll 推断为 possibly undefined
export function useVirtualScroll(messageCount: Ref<number>): {
  enableVirtualScroll: Ref<boolean>
  shouldUseVirtualScroll: ComputedRef<boolean>
}
export function useVirtualScroll(messageCount?: undefined): {
  enableVirtualScroll: Ref<boolean>
}
export function useVirtualScroll(messageCount?: Ref<number>) {
  if (!messageCount) return { enableVirtualScroll }

  // 当前是否处于虚拟模式（内部状态，带滞回：进入难、退出易反向）
  const virtualActive = ref(false)

  const shouldUseVirtualScroll = computed(() => {
    if (!enableVirtualScroll.value) {
      virtualActive.value = false
      return false
    }
    const count = messageCount.value ?? 0
    if (virtualActive.value) {
      // 已在虚拟模式：跌破退出线才回到普通渲染
      if (count < VIRTUAL_EXIT_THRESHOLD) virtualActive.value = false
    } else if (count >= VIRTUAL_ENTER_THRESHOLD) {
      // 普通模式：达到进入线才切虚拟滚动
      virtualActive.value = true
    }
    return virtualActive.value
  })

  return { enableVirtualScroll, shouldUseVirtualScroll }
}
