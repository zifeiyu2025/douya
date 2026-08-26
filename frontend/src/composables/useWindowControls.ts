/**
 * useWindowControls - 窗口控制 composable
 *
 * 从 App.vue 抽取的窗口控制相关状态与逻辑（最小化/最大化/关闭）。
 * 仅负责窗口操作 UI 层，关闭行为三态（exit/tray/ask）由后端根据
 * config 中的关闭行为设置决定，通过 wails.handleCloseRequest() 返回。
 *
 * 设计原则：
 *  - 纯搬迁，不优化不重构，逻辑与原 App.vue 完全一致
 *  - 事件监听在 onMounted 注册、onUnmounted 清理，避免内存泄漏
 *  - resize 防抖定时器随组件卸载一并清理
 *
 * 关闭行为三态逻辑（与原 App.vue handleClose 完全一致）：
 *  - exit 模式：直接调用 wails.gracefulExit() 退出应用
 *  - tray 模式：调用 WindowHide() 隐藏到系统托盘
 *  - ask  模式：弹出 discreteDialog 询问用户"最小化到托盘"还是"直接退出"，
 *              用户选择后通过 wails.setCloseAction() 持久化选择并执行对应操作
 *  具体进入哪个模式由后端 handleCloseRequest 根据 config 中的关闭行为设置决定。
 */
import { onMounted, onUnmounted, readonly, ref } from 'vue'
import { wails } from '../services/wails'
import { discreteDialog } from '../utils/discrete'
import {
  WindowMinimise,
  WindowToggleMaximise,
  WindowIsMaximised,
  WindowHide
} from '../../wailsjs/runtime/runtime'

export function useWindowControls() {
  // ----- 窗口状态 -----
  const isMaximized = ref(false)

  // ----- resize 防抖定时器 -----
  let resizeTimer: ReturnType<typeof setTimeout> | null = null

  // 关闭询问对话框防重入标志
  // 避免 ALT+F4 和前端关闭按钮同时触发导致弹出两个对话框
  let isCloseDialogShowing = false

  function handleMinimize() {
    WindowMinimise()
  }

  function handleToggleMaximize() {
    WindowToggleMaximise()
    updateMaximizedState()
  }

  // 双击标题栏切换最大化/还原。
  // 只检查 target 是否落在真正的交互元素上，不命中才触发最大化。
  // 这样双击菜单按钮/模型选择器/窗口控制按钮不会误触发，双击标题文字或元素间空隙会触发。
  function handleHeaderDoubleClick(e: MouseEvent) {
    const target = e.target as HTMLElement | null
    if (!target) return
    if (
      target.closest('button, .n-select, .n-base-selection, input, a, [role="button"], .win-btn')
    ) {
      return
    }
    handleToggleMaximize()
  }

  // showCloseDialog 弹出关闭询问对话框（抽取为独立函数，供前端按钮和 ALT+F4 共用）
  function showCloseDialog() {
    if (isCloseDialogShowing) return
    isCloseDialogShowing = true
    discreteDialog.warning({
      title: '关闭窗口',
      content: '你希望将豆芽最小化到系统托盘后台运行，还是直接退出程序？',
      positiveText: '最小化到托盘',
      negativeText: '直接退出',
      onPositiveClick: async () => {
        isCloseDialogShowing = false
        await wails.setCloseAction('tray')
        WindowHide()
      },
      onNegativeClick: async () => {
        isCloseDialogShowing = false
        await wails.setCloseAction('exit')
        wails.gracefulExit()
      },
      onMaskClick: () => {
        isCloseDialogShowing = false
      }
    })
  }

  async function handleClose() {
    const action = await wails.handleCloseRequest()
    if (action === 'exit') {
      wails.gracefulExit()
      return
    }
    if (action === 'tray') {
      WindowHide()
      return
    }
    // action === 'ask'：首次关闭时询问
    showCloseDialog()
  }

  async function updateMaximizedState() {
    try {
      isMaximized.value = await WindowIsMaximised()
    } catch {
      isMaximized.value = false
    }
  }

  // 防抖处理 resize 事件：200ms 内多次触发只执行最后一次，避免频繁查询窗口状态
  function handleResize() {
    if (resizeTimer) clearTimeout(resizeTimer)
    resizeTimer = setTimeout(updateMaximizedState, 200)
  }

  // 事件监听取消器集合（含 window:closeRequest 监听）
  const unsubscribers: Array<() => void> = []

  onMounted(() => {
    // 初始化最大化状态（与原 App.vue 在 onMounted 内通过 Promise.all 调用一致）
    updateMaximizedState()
    window.addEventListener('resize', handleResize)

    // 监听 ALT+F4 / 系统关闭按钮事件，与前端关闭按钮行为一致
    // 后端 beforeClose 在 'ask' 模式下不再直接隐藏窗口，而是发送事件让前端弹出询问对话框
    unsubscribers.push(
      wails.subscribeCloseRequest(() => {
        showCloseDialog()
      })
    )
  })

  onUnmounted(() => {
    window.removeEventListener('resize', handleResize)
    // 清理 resize 防抖定时器
    if (resizeTimer) {
      clearTimeout(resizeTimer)
      resizeTimer = null
    }
    // 清理事件监听
    while (unsubscribers.length > 0) {
      const unsubscribe = unsubscribers.pop()
      try {
        unsubscribe?.()
      } catch {
        // ignore
      }
    }
  })

  return {
    isMaximized: readonly(isMaximized), // Readonly<Ref<boolean>>：外部只读，内部可变
    handleMinimize, // () => void
    handleToggleMaximize, // () => void
    handleHeaderDoubleClick, // (e: MouseEvent) => void
    handleClose // () => void
  }
}
