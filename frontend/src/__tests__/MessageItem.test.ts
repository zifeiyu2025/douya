import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, ref, type App as VueApp } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import MessageItem from '../components/MessageItem.vue'
import { setupCodeCopyDelegation } from '../utils/codeCopy'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { DEFAULT_CONFIG, type Message } from '../services/wails'

vi.mock('../utils/markdown', () => ({
    renderMarkdown: vi.fn(async (content: string) => (
        `<pre class="hljs"><div class="code-header"><button class="code-copy-btn">复制</button></div><code>${content}</code></pre>`
    )),
    escapeHtml: vi.fn((text: string) => text),
}))

vi.mock('naive-ui', () => ({
    createDiscreteApi: () => ({
        message: {
            success: vi.fn(),
            error: vi.fn(),
            warning: vi.fn(),
            info: vi.fn(),
        },
    }),
    useMessage: () => ({
        success: vi.fn(),
        error: vi.fn(),
    }),
    useDialog: () => ({
        create: vi.fn(),
    }),
}))

vi.mock('../components/ThinkBlock.vue', () => ({
    default: {
        name: 'ThinkBlockStub',
        template: '<div class="think-block-stub" />',
    },
}))

vi.mock('../components/SearchStatus.vue', () => ({
    default: {
        name: 'SearchStatusStub',
        template: '<div class="search-status-stub" />',
    },
}))

vi.mock('../components/ui/AppIcon.vue', () => ({
    default: {
        name: 'AppIconStub',
        template: '<span class="app-icon-stub" />',
    },
}))

function createMessage(content: string): Message {
    return {
        id: 'assistant-1',
        conversation_id: 'conv-1',
        role: 'assistant',
        content,
        search_results: '',
        created_at: '2026-06-24T00:00:00Z',
    }
}

async function flushRendering() {
    await Promise.resolve()
    await nextTick()
    await Promise.resolve()
    await nextTick()
}

describe('MessageItem 代码复制绑定', () => {
    let app: VueApp<Element> | null = null
    let host: HTMLDivElement
    let listCleanup: (() => void) | null = null
    let writeTextSpy: ReturnType<typeof vi.fn>

    beforeEach(() => {
        host = document.createElement('div')
        document.body.appendChild(host)

        writeTextSpy = vi.fn().mockResolvedValue(undefined)
        Object.defineProperty(navigator, 'clipboard', {
            value: { writeText: writeTextSpy },
            configurable: true,
        })
    })

    afterEach(() => {
        listCleanup?.()
        listCleanup = null
        if (app && host.firstChild) {
            app.unmount()
        }
        app = null
        host.remove()
        vi.restoreAllMocks()
    })

    it('同一消息多次重渲染后，点击复制按钮只触发一次复制', async () => {
        const pinia = createPinia()
        setActivePinia(pinia)

        const message = ref(createMessage('first code'))
        const Host = defineComponent({
            name: 'MessageItemHost',
            setup() {
                return () => h('div', { class: 'message-list-host' }, [
                    h(MessageItem, { message: message.value }),
                ])
            },
        })

        app = createApp(Host)
        app.use(pinia)

        const chatStore = useChatStore()
        const settingsStore = useSettingsStore()
        settingsStore.config = { ...DEFAULT_CONFIG }
        chatStore.messages = [message.value]

        app.mount(host)
        listCleanup = setupCodeCopyDelegation(host)
        await flushRendering()

        message.value = createMessage('second code')
        chatStore.messages = [message.value]
        await flushRendering()

        message.value = createMessage('final code')
        chatStore.messages = [message.value]
        await flushRendering()

        const copyButton = host.querySelector('.code-copy-btn') as HTMLButtonElement | null
        expect(copyButton).not.toBeNull()

        copyButton?.click()
        await flushRendering()

        expect(writeTextSpy).toHaveBeenCalledTimes(1)
        expect(writeTextSpy).toHaveBeenCalledWith('final code')
    })
})
