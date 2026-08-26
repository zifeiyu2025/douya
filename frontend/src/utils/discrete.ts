/**
 * 全局 discrete API（message / dialog）
 *
 * 为什么用 Proxy？
 * - discrete API 需要在非组件代码（如 pinia store）中调用，无法用
 *   useMessage()/useDialog()（这俩只能在 setup 内用）
 * - createDiscreteApi 会创建独立 Vue 应用实例，开销大，不能每次调用都创建
 * - 用 Proxy 包装后，外部代码像使用普通对象一样调用 discreteMessage.success()，
 *   Proxy 内部自动委托给当前主题对应的真实 API 实例
 *
 * 主题切换时如何处理？
 * - 缓存上一次创建时的 isDark 值
 * - 每次通过 Proxy 访问时，检查当前 isDark 是否与缓存值不同
 * - 不同则销毁旧实例、创建新实例（确保 message/dialog 跟随主题切换）
 *
 * 注意：调用方必须在 pinia 已激活时调用（项目 main.ts 已全局激活 pinia，
 * App.vue / stores/*.ts 中的调用都满足此条件）
 */
import { createDiscreteApi, darkTheme } from 'naive-ui'
import type { MessageApi, DialogApi } from 'naive-ui'
import { useThemeStore } from '../stores/theme'
import { useThemeOverrides } from '../composables/useThemeOverrides'

interface CachedApi {
  message: MessageApi
  dialog: DialogApi
  isDark: boolean
}

let cached: CachedApi | null = null

/**
 * 获取（或按需重建）当前主题对应的 discrete API 实例
 *
 * 重建时机：isDark 发生变化（亮↔深切换）
 * - createDiscreteApi 创建的实例在创建时就绑定了 theme/themeOverrides，
 *   后续无法动态修改，所以主题变化时必须重建
 */
function getApi(): CachedApi {
  const themeStore = useThemeStore()
  const isDark = themeStore.isDark

  // 主题未变化且已缓存：直接复用（性能关键路径，避免重复 createDiscreteApi）
  if (cached && cached.isDark === isDark) {
    return cached
  }

  // 主题变化或首次调用：创建新实例
  // useThemeOverrides() 返回 ComputedRef，.value 取当前值
  const overrides = useThemeOverrides().value
  const { message, dialog } = createDiscreteApi(['message', 'dialog'], {
    configProviderProps: {
      theme: isDark ? darkTheme : undefined,
      themeOverrides: overrides
    }
  })

  cached = { message, dialog, isDark }
  return cached
}

/**
 * discreteMessage：message API 的主题感知代理
 *
 * 用法与 NaiveUI 的 useMessage() 返回值完全一致：
 *   discreteMessage.success('操作成功')
 *   discreteMessage.error('出错了')
 *   discreteMessage.warning('警告')
 *   discreteMessage.info('提示')
 *
 * 每次访问任意方法时，Proxy 会动态获取当前主题对应的 message 实例，
 * 确保主题切换后新提示使用新配色。
 */
export const discreteMessage: MessageApi = new Proxy({} as MessageApi, {
  get(_target, prop) {
    const api = getApi().message
    const value = (api as unknown as Record<string | symbol, unknown>)[prop]
    return typeof value === 'function' ? value.bind(api) : value
  }
})

/**
 * discreteDialog：dialog API 的主题感知代理
 *
 * 用法与 NaiveUI 的 useDialog() 返回值完全一致：
 *   discreteDialog.warning({ title: '确认', content: '...', onPositiveClick: ... })
 *   discreteDialog.error({ title: '错误', content: '...' })
 */
export const discreteDialog: DialogApi = new Proxy({} as DialogApi, {
  get(_target, prop) {
    const api = getApi().dialog
    const value = (api as unknown as Record<string | symbol, unknown>)[prop]
    return typeof value === 'function' ? value.bind(api) : value
  }
})
