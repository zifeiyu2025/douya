/**
 * 虚拟滚动 feature flag（任务 38）
 *
 * 安全审查 #38：本文件仅是 feature flag 开关，不涉及敏感数据处理，
 * 已在安全审查报告中记录为无需修改，仅保留说明。
 *
 * 生活类比：
 *   这个开关就像汽车上的“运动模式”按钮——默认关闭，按平时的方式平稳行驶（原 v-for）；
 *   打开后切换到更高性能但可能略颠簸的模式（虚拟滚动），出问题随时一键切回。
 *
 * 设计要点：
 *   - 纯前端开关，不进入后端 Config，使用 localStorage 持久化（与 theme store 一致）
 *   - 模块级单例 ref，跨组件共享同一响应式状态
 *     （MessageList 读取控制渲染分支，ExperimentalSettings 写入切换）
 *   - 默认 false（关闭），保留原 v-for 作为回滚兜底
 */
import { ref, watch } from 'vue'

const STORAGE_KEY = 'douya-enable-virtual-scroll'

// 模块级单例：首次加载时从 localStorage 读取，缺省为 false（关闭）
const enableVirtualScroll = ref(localStorage.getItem(STORAGE_KEY) === 'true')

// 持久化：开关变化即写回 localStorage
watch(enableVirtualScroll, (val) => {
  localStorage.setItem(STORAGE_KEY, String(val))
})

export function useVirtualScroll() {
  return { enableVirtualScroll }
}
