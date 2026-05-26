import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { useThemeStore } from '../stores/theme'

describe('useThemeStore', () => {
    let storage: Record<string, string>

    beforeEach(() => {
        storage = {}
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
                    add: vi.fn(),
                    remove: vi.fn(),
                },
            },
        })
        setActivePinia(createPinia())
    })

    it('should default to light mode when localStorage has no setting', () => {
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
})
