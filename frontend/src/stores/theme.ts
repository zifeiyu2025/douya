import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export const useThemeStore = defineStore('theme', () => {
    const stored = localStorage.getItem('douya-theme')
    const isDark = ref(stored === 'dark')

    watch(isDark, (val) => {
        localStorage.setItem('douya-theme', val ? 'dark' : 'light')
    })

    function toggleTheme() {
        isDark.value = !isDark.value
    }

    return {
        isDark,
        toggleTheme,
    }
})
