/**
 * 虚拟滚动 feature flag（任务 38）
 *
 * 安全审查 #38：本文件仅是 feature flag 开关，不涉及敏感数据处理，
 * 已在安全审查报告中记录为无需修改，仅保留说明。
 *
 * 生活类比：
 *   这个开关就像汽车上的"运动模式"按钮——默认关闭，日常驾驶用普通模式更稳；
 *   需要高性能时可显式开启（localStorage 设为 'true'），享受虚拟滚动。
 *
 * 设计要点：
 *   - 纯前端开关，不进入后端 Config，使用 localStorage 持久化（与 theme store 一致）
 *   - 模块级单例 ref，跨组件共享同一响应式状态
 *     （MessageList 读取控制渲染分支，ExperimentalSettings 写入切换）
 *   - 默认 false（关闭），用户可显式开启，保留原 v-for 作为默认平稳模式
 *
 * 为什么默认关闭：
 *   虚拟滚动模式下，流式占位气泡在 DynamicScroller 外部（作为 .message-list 的子元素），
 *   而 .virtual-scroller-wrap 设置了 flex:1 占满高度，导致流式占位气泡被推到视口外。
 *   滚动到底部时气泡从底部滑入视口，视觉上表现为"头像和气泡从下到上移动"。
 *   普通对话场景下 v-for 渲染性能足够，虚拟滚动仅在超长对话（数百条以上）才有明显优势。
 */
import { ref, watch } from 'vue'

const STORAGE_KEY = 'douya-enable-virtual-scroll'

// 模块级单例：默认关闭虚拟滚动，用户可显式开启（localStorage 设为 'true'）
const enableVirtualScroll = ref(localStorage.getItem(STORAGE_KEY) === 'true')

// 持久化：开关变化即写回 localStorage
watch(enableVirtualScroll, val => {
  localStorage.setItem(STORAGE_KEY, String(val))
})

export function useVirtualScroll() {
  return { enableVirtualScroll }
}
