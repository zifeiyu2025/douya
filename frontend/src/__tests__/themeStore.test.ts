import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { useThemeStore } from '../stores/theme'

describe('useThemeStore', () => {
  let storage: Record<string, string>
  let classListAdd: ReturnType<typeof vi.fn>
  let classListRemove: ReturnType<typeof vi.fn>
  // 系统深色偏好开关：matchMedia mock 的 matches getter 动态读取此变量
  // 这样即使 theme.ts 模块级 mql 被缓存，修改此变量也能让 mql.matches 反映新值
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
    // mock window.matchMedia：matches 用 getter 动态读取 systemDarkState
    // theme.ts 会缓存 mql（模块级单例），getter 保证缓存对象也能读到最新系统偏好
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
    setActivePinia(createPinia())
  })

  it('should default to light mode when localStorage has no setting', () => {
    const store = useThemeStore()
    expect(store.isDark).toBe(false)
  })

  it('should initialize to dark mode when localStorage is "dark"', () => {
    storage['douya-theme'] = 'dark'
    const store = useThemeStore()
    expect(store.isDark).toBe(true)
  })

  it('should initialize to light mode when localStorage is "light"', () => {
    storage['douya-theme'] = 'light'
    const store = useThemeStore()
    expect(store.isDark).toBe(false)
  })

  it('should toggle theme', () => {
    const store = useThemeStore()
    expect(store.isDark).toBe(false)
    store.toggleTheme()
    expect(store.isDark).toBe(true)
    store.toggleTheme()
    expect(store.isDark).toBe(false)
  })

  it('should persist theme to localStorage', async () => {
    const store = useThemeStore()
    store.toggleTheme()
    await nextTick()
    expect(storage['douya-theme-mode']).toBe('dark')
  })

  it('should apply dark class to documentElement when isDark is true', async () => {
    const store = useThemeStore()
    store.toggleTheme()
    await nextTick()
    expect(classListAdd).toHaveBeenCalledWith('dark')
  })

  it('should remove dark class from documentElement when switching to light', async () => {
    storage['douya-theme'] = 'dark'
    const store = useThemeStore()
    // 初始化为深色模式后切换到亮色
    store.toggleTheme()
    await nextTick()
    expect(classListRemove).toHaveBeenCalledWith('dark')
  })

  it('should apply dark class on initialization when localStorage is dark', () => {
    storage['douya-theme'] = 'dark'
    useThemeStore()
    // 初始化时即应用 dark class
    expect(classListAdd).toHaveBeenCalledWith('dark')
  })

  it('should not apply dark class on initialization when localStorage is light', () => {
    storage['douya-theme'] = 'light'
    useThemeStore()
    expect(classListAdd).not.toHaveBeenCalledWith('dark')
  })

  // ===== SubTask 18.1: mode='auto' 跟随系统偏好 =====
  it('auto mode resolves to dark when system prefers dark', () => {
    systemDarkState = true
    storage['douya-theme-mode'] = 'auto'
    const store = useThemeStore()
    expect(store.mode).toBe('auto')
    expect(store.resolvedMode).toBe('dark')
    expect(store.isDark).toBe(true)
    // 初始化时同步应用 dark class
    expect(classListAdd).toHaveBeenCalledWith('dark')
  })

  it('auto mode resolves to light when system prefers light', () => {
    systemDarkState = false
    storage['douya-theme-mode'] = 'auto'
    const store = useThemeStore()
    expect(store.mode).toBe('auto')
    expect(store.resolvedMode).toBe('light')
    expect(store.isDark).toBe(false)
    // 亮色模式下不应添加 dark class
    expect(classListAdd).not.toHaveBeenCalledWith('dark')
  })

  // ===== SubTask 18.2: resolvedMode 在三种 mode 下的返回值 =====
  it('resolvedMode is light when mode is light, regardless of systemDark', () => {
    // 系统为深色，但 mode 显式 light 应忽略系统偏好
    systemDarkState = true
    storage['douya-theme-mode'] = 'light'
    const store = useThemeStore()
    expect(store.mode).toBe('light')
    expect(store.resolvedMode).toBe('light')
  })

  it('resolvedMode is dark when mode is dark, regardless of systemDark', () => {
    // 系统为亮色，但 mode 显式 dark 应忽略系统偏好
    systemDarkState = false
    storage['douya-theme-mode'] = 'dark'
    const store = useThemeStore()
    expect(store.mode).toBe('dark')
    expect(store.resolvedMode).toBe('dark')
  })

  it('resolvedMode is dark when mode is auto and system prefers dark', () => {
    systemDarkState = true
    const store = useThemeStore()
    expect(store.mode).toBe('auto')
    expect(store.resolvedMode).toBe('dark')
  })

  it('resolvedMode is light when mode is auto and system prefers light', () => {
    systemDarkState = false
    const store = useThemeStore()
    expect(store.mode).toBe('auto')
    expect(store.resolvedMode).toBe('light')
  })

  // ===== SubTask 18.3: setMode 持久化到 localStorage =====
  it('setMode persists "dark" to localStorage under douya-theme-mode', async () => {
    const store = useThemeStore()
    store.setMode('dark')
    await nextTick()
    expect(localStorage.getItem('douya-theme-mode')).toBe('dark')
  })

  it('setMode persists "light" to localStorage under douya-theme-mode', async () => {
    const store = useThemeStore()
    store.setMode('light')
    await nextTick()
    expect(localStorage.getItem('douya-theme-mode')).toBe('light')
  })

  it('setMode persists "auto" to localStorage under douya-theme-mode', async () => {
    const store = useThemeStore()
    // 默认 mode 已是 auto，先切到非 auto 再切回，确保 mode 变化触发 watch 持久化
    store.setMode('dark')
    await nextTick()
    store.setMode('auto')
    await nextTick()
    expect(localStorage.getItem('douya-theme-mode')).toBe('auto')
  })

  // ===== SubTask 18.4: 旧键 douya-theme 迁移 =====
  it('migrates old key douya-theme=dark to douya-theme-mode', () => {
    storage['douya-theme'] = 'dark'
    const store = useThemeStore()
    expect(store.mode).toBe('dark')
    expect(localStorage.getItem('douya-theme-mode')).toBe('dark')
    // 旧键应已被删除
    expect(localStorage.getItem('douya-theme')).toBeNull()
  })

  it('migrates old key douya-theme=light to douya-theme-mode', () => {
    storage['douya-theme'] = 'light'
    const store = useThemeStore()
    expect(store.mode).toBe('light')
    expect(localStorage.getItem('douya-theme-mode')).toBe('light')
    expect(localStorage.getItem('douya-theme')).toBeNull()
  })

  it('defaults to auto mode when no theme key exists in localStorage', () => {
    const store = useThemeStore()
    expect(store.mode).toBe('auto')
  })
})
