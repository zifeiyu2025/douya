import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { applyCodeTheme } from '../utils/markdown'

export const useThemeStore = defineStore('theme', () => {
    const stored = localStorage.getItem('douya-theme')
    const isDark = ref(stored === 'dark')

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

    applyThemeClass(isDark.value)

    watch(isDark, (val) => {
        localStorage.setItem('douya-theme', val ? 'dark' : 'light')
        applyThemeClass(val)
    })

    function toggleTheme() {
        isDark.value = !isDark.value
    }

    return {
        isDark,
        toggleTheme,
    }
})
