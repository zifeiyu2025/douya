import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { applyCodeTheme } from '../utils/markdown'

// 模块级变量：缓存 matchMedia 引用，避免 store 重复实例化时重复注册监听器
// store 在生产环境中是单例，只需注册一次；测试中虽会多次实例化，但不会触发 change 事件
let mql: MediaQueryList | null = null

export const useThemeStore = defineStore('theme', () => {
  // ===== mode 状态 + 旧键迁移 =====
  // 读取 localStorage，优先使用新键 douya-theme-mode，兼容旧键 douya-theme
  let initialMode: 'light' | 'dark' | 'auto' = 'auto'
  if (typeof localStorage !== 'undefined') {
    const newStored = localStorage.getItem('douya-theme-mode')
    if (newStored === 'light' || newStored === 'dark' || newStored === 'auto') {
      // 新键存在且合法，直接使用
      initialMode = newStored
    } else {
      const oldStored = localStorage.getItem('douya-theme')
      if (oldStored === 'dark' || oldStored === 'light') {
        // 旧键存在：迁移为对应 mode
        initialMode = oldStored
        localStorage.setItem('douya-theme-mode', initialMode)
        localStorage.removeItem('douya-theme')
      }
      // 两个键都不存在：默认 'auto'（首次用户跟随系统）
    }
  }

  const mode = ref<'light' | 'dark' | 'auto'>(initialMode)

  // ===== systemDark（跟随系统偏好） =====
  const systemDark = ref(false)
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    if (!mql) {
      mql = window.matchMedia('(prefers-color-scheme: dark)')
      mql.addEventListener('change', (e: MediaQueryListEvent) => {
        systemDark.value = e.matches
      })
    }
    // 每次实例化都同步当前状态（测试中 store 会重新实例化）
    systemDark.value = mql.matches
  }

  // ===== resolvedMode（实际生效的模式） =====
  const resolvedMode = computed<'light' | 'dark'>(() =>
    mode.value === 'auto' ? (systemDark.value ? 'dark' : 'light') : mode.value
  )

  // ===== isDark 向后兼容 =====
  const isDark = computed(() => resolvedMode.value === 'dark')

  // ===== applyThemeClass（操作 DOM classList + 代码高亮主题） =====
  function applyThemeClass(dark: boolean) {
    if (typeof document !== 'undefined') {
      if (dark) {
        document.documentElement.classList.add('dark')
      } else {
        document.documentElement.classList.remove('dark')
      }
      // 同步加载/切换代码高亮主题
      // 首次进入深色模式会动态加载 github-dark.css
      applyCodeTheme(dark)
    }
  }

  // 初始化时应用主题
  applyThemeClass(resolvedMode.value === 'dark')

  // watch resolvedMode：mode 或 systemDark 变化时都会触发，自动应用主题
  watch(resolvedMode, val => {
    applyThemeClass(val === 'dark')
  })

  // ===== setMode 方法 + 自动持久化 =====
  function setMode(m: 'light' | 'dark' | 'auto') {
    mode.value = m
  }

  // watch mode：自动写入 localStorage（持久化）
  watch(mode, val => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('douya-theme-mode', val)
    }
  })

  // toggleTheme：在 light/dark 之间切换，不切到 auto
  function toggleTheme() {
    mode.value = isDark.value ? 'light' : 'dark'
  }

  return {
    mode,
    resolvedMode,
    systemDark,
    isDark,
    setMode,
    toggleTheme
  }
})
