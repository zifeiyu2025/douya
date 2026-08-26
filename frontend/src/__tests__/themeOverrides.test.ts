import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick, isRef } from 'vue'
import { useThemeStore } from '../stores/theme'
import { useThemeOverrides } from '../composables/useThemeOverrides'

/**
 * useThemeOverrides 测试
 *
 * useThemeOverrides 按当前主题返回对应的主题覆写配置：
 * 亮色为"纸面"配色（纯白底 × 亮蓝主色 × 石墨文字），
 * 深色为"石墨"配色（深空黑底 × 辉光蓝主色 × 雾白文字）。
 * 本测试检查：返回值结构完整（每个组件都有对应项），且主题切换时内容随之变化。
 */
describe('useThemeOverrides', () => {
  let storage: Record<string, string>
  let classListAdd: ReturnType<typeof vi.fn>
  let classListRemove: ReturnType<typeof vi.fn>
  // 系统深色偏好开关：matchMedia mock 的 matches getter 动态读取此变量
  let systemDarkState: boolean

  beforeEach(() => {
    storage = {}
    classListAdd = vi.fn()
    classListRemove = vi.fn()
    systemDarkState = false
    vi.stubGlobal('localStorage', {
      getItem: vi.fn((key: string) => storage[key] ?? null),
      setItem: vi.fn((key: string, value: string) => {
        storage[key] = value
      }),
      removeItem: vi.fn((key: string) => {
        delete storage[key]
      }),
      clear: vi.fn(() => {
        for (const k in storage) delete storage[k]
      }),
      get length() {
        return Object.keys(storage).length
      },
      key: vi.fn((index: number) => Object.keys(storage)[index] ?? null)
    })
    vi.stubGlobal('document', {
      documentElement: {
        classList: {
          add: classListAdd,
          remove: classListRemove
        }
      }
    })
    // mock window.matchMedia：与 themeStore.test.ts 保持一致的 mock 方式
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      configurable: true,
      value: vi.fn(() => ({
        get matches() {
          return systemDarkState
        },
        addEventListener: vi.fn(),
        removeEventListener: vi.fn()
      }))
    })
    // 每个测试激活全新的 pinia 实例，保证 store 状态隔离
    setActivePinia(createPinia())
  })

  // ===== 验证 useThemeOverrides 返回值结构 =====
  describe('返回值结构', () => {
    it('returns a ComputedRef<GlobalThemeOverrides>', () => {
      const overrides = useThemeOverrides()
      // 是一个 ref
      expect(isRef(overrides)).toBe(true)
      // ComputedRef 在 Vue 3 内部会带 effect 属性（区别于普通 ref）
      expect((overrides as unknown as { effect: unknown }).effect).toBeDefined()
      // .value 当前是一个对象（GlobalThemeOverrides）
      expect(typeof overrides.value).toBe('object')
      expect(overrides.value).not.toBeNull()
    })

    it('contains common and all expected component overrides', () => {
      const overrides = useThemeOverrides()
      const value = overrides.value
      // common 必须存在
      expect(value.common).toBeDefined()
      // 各组件 overrides 必须存在（与 useThemeOverrides.ts 实际导出一致）
      expect(value.Button).toBeDefined()
      expect(value.Input).toBeDefined()
      expect(value.Select).toBeDefined()
      expect(value.Card).toBeDefined()
      expect(value.Dialog).toBeDefined()
      expect(value.Message).toBeDefined()
      expect(value.Drawer).toBeDefined()
      expect(value.Slider).toBeDefined()
      expect(value.Collapse).toBeDefined()
      expect(value.Form).toBeDefined()
    })

    it('common has key tokens as strings', () => {
      const overrides = useThemeOverrides()
      const common = overrides.value.common!
      // 关键 token 存在且为字符串
      expect(typeof common.primaryColor).toBe('string')
      expect(common.primaryColor).toBeTruthy()
      expect(typeof common.bodyColor).toBe('string')
      expect(common.bodyColor).toBeTruthy()
      expect(typeof common.textColor1).toBe('string')
      expect(common.textColor1).toBeTruthy()
      expect(typeof common.borderColor).toBe('string')
      expect(common.borderColor).toBeTruthy()
    })
  })

  // ===== 验证亮色与深色下 overrides 切换 =====
  describe('纸面与石墨切换', () => {
    it('light mode: key tokens match trae-paper light palette', async () => {
      const store = useThemeStore()
      const overrides = useThemeOverrides()
      // 显式切到亮色
      store.setMode('light')
      await nextTick()
      const common = overrides.value.common!
      // 亮色 accent-primary（TRAE 亮蓝章）
      expect(common.primaryColor).toBe('#2f74ff')
      // 亮色 --bg-primary（纯白基底，对背景图友好）
      expect(common.bodyColor).toBe('#ffffff')
      // 亮色 --text-primary（石墨墨）
      expect(common.textColor1).toBe('#31353a')
      // 亮色 --border-color（石灰细线）
      expect(common.borderColor).toBe('#dfe3ea')
    })

    it('dark mode: key tokens match trae-graphite dark palette', async () => {
      const store = useThemeStore()
      const overrides = useThemeOverrides()
      // 显式切到深色
      store.setMode('dark')
      await nextTick()
      const common = overrides.value.common!
      // 深色 accent-primary（TRAE 亮蓝）
      expect(common.primaryColor).toBe('#387bff')
      // 深色 --bg-primary（深空灰黑，非纯黑）
      expect(common.bodyColor).toBe('#1a1b1d')
      // 深色 --text-primary（雾白）
      expect(common.textColor1).toBe('#d1d3db')
      // 深色 --border-color（石墨细边）
      expect(common.borderColor).toBe('#303031')
    })

    it('reactively switches overrides when mode changes', async () => {
      const store = useThemeStore()
      const overrides = useThemeOverrides()
      // 先亮色：primaryColor 为 TRAE 亮蓝章
      store.setMode('light')
      await nextTick()
      const commonLight = overrides.value.common!
      expect(commonLight.primaryColor).toBe('#2f74ff')
      expect(commonLight.bodyColor).toBe('#ffffff')

      // 切到深色：primaryColor 变为 TRAE 亮蓝，bodyColor 变为深空灰黑
      store.setMode('dark')
      await nextTick()
      const commonDark = overrides.value.common!
      expect(commonDark.primaryColor).toBe('#387bff')
      expect(commonDark.bodyColor).toBe('#1a1b1d')

      // 切回亮色：再次回到纯白亮蓝
      store.setMode('light')
      await nextTick()
      const commonLight2 = overrides.value.common!
      expect(commonLight2.primaryColor).toBe('#2f74ff')
      expect(commonLight2.bodyColor).toBe('#ffffff')
    })
  })
})
