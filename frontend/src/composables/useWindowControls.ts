/**
 * 窗口控制 composable
 * 封装 Wails 窗口 API：最小化、最大化、关闭
 */
import { ref, onMounted, onUnmounted } from 'vue'
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised, WindowHide } from '../../wailsjs/runtime/runtime'

export function useWindowControls() {
    const isMaximized = ref(false)

    async function refreshMaximized() {
        try {
            isMaximized.value = await WindowIsMaximised()
        } catch {
            isMaximized.value = false
        }
    }

    function minimize() {
        WindowMinimise()
    }

    async function toggleMaximize() {
        WindowToggleMaximise()
        await refreshMaximized()
    }

    function close() {
        WindowHide()
    }

    onMounted(() => {
        refreshMaximized()
        window.addEventListener('resize', refreshMaximized)
    })

    onUnmounted(() => {
        window.removeEventListener('resize', refreshMaximized)
    })

    return {
        isMaximized,
        minimize,
        toggleMaximize,
        close,
        refreshMaximized,
    }
}
