import type { InjectionKey } from 'vue'
// C-5 设置域重建：45 字段平铺 context 重构为五个类型化域切片
// 各域 API 类型直接取自对应 composable 的返回值（ReturnType），新增成员自动同步，无需手写接口
import type { useSettingsCore } from './composables/useSettingsCore'
import type { useAppearanceSettings } from './composables/useAppearanceSettings'
import type { useAIChatSettings } from './composables/useAIChatSettings'
import type { usePerformanceSettings } from './composables/usePerformanceSettings'
import type { useAPIServiceSettings } from './composables/useAPIServiceSettings'

export interface SettingsContext {
  /** 核心：formConfig / autoSave / dirty 状态 / 模型切换 hook 注册 */
  core: ReturnType<typeof useSettingsCore>
  /** 外观：背景图、头像 */
  appearance: ReturnType<typeof useAppearanceSettings>
  /** AI 对话：模型参考参数、推理、模型预设、上下文推荐 */
  aiChat: ReturnType<typeof useAIChatSettings>
  /** 性能：GPU 检测、上下文滑块、KV 缓存与推测解码选项 */
  performance: ReturnType<typeof usePerformanceSettings>
  /** API 服务：搜索密钥、服务端密钥、局域网 / Web UI 开关 */
  apiService: ReturnType<typeof useAPIServiceSettings>
}

export const SETTINGS_CONTEXT_KEY: InjectionKey<SettingsContext> = Symbol('settingsContext')
