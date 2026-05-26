import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

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
