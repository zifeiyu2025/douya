import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { useThemeStore } from '../stores/theme'

describe('useThemeStore', () => {
    let storage: Record<string, string>
    let classListAdd: ReturnType<typeof vi.fn>
    let classListRemove: ReturnType<typeof vi.fn>

    beforeEach(() => {
        storage = {}
        classListAdd = vi.fn()
        classListRemove = vi.fn()
        vi.stubGlobal('localStorage', {
            getItem: vi.fn((key: string) => storage[key] ?? null),
            setItem: vi.fn((key: string, value: string) => { storage[key] = value }),
            removeItem: vi.fn((key: string) => { delete storage[key] }),
            clear: vi.fn(() => { for (const k in storage) delete storage[k] }),
            get length() { return Object.keys(storage).length },
            key: vi.fn((index: number) => Object.keys(storage)[index] ?? null),
        })
        vi.stubGlobal('document', {
            documentElement: {
                classList: {
                    add: classListAdd,
                    remove: classListRemove,
                },
            },
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
        expect(storage['douya-theme']).toBe('dark')
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
})
