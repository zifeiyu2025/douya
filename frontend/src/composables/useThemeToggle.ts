/**
 * 主题切换 composable
 */
import { computed } from 'vue'
import { useThemeStore } from '../stores/theme'

export function useThemeToggle() {
    const themeStore = useThemeStore()
    return {
        isDark: computed(() => themeStore.isDark),
        toggle: () => themeStore.toggleTheme(),
    }
}
