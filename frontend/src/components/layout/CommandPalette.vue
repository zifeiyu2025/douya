<template>
  <!-- Ctrl+K 命令面板：
       居中纸卡 + 发丝边；命令源聚合 固定动作 / 模型切换 / 会话跳转 三类 -->
  <Teleport to="body">
    <Transition name="palette-pop">
      <div
        v-if="open"
        class="palette-veil"
        @click.self="emit('close')"
        @keydown.esc="emit('close')"
      >
        <div class="palette-card" role="dialog" aria-label="命令面板">
          <!-- 输入行 -->
          <div class="palette-input-row">
            <AppIcon name="search" :size="15" class="input-glyph" />
            <input
              ref="inputEl"
              v-model="query"
              class="palette-input"
              type="text"
              placeholder="搜索命令、模型或会话…"
              spellcheck="false"
              @keydown.down.prevent="move(1)"
              @keydown.up.prevent="move(-1)"
              @keydown.enter.prevent="runActive()"
              @keydown.esc.prevent="emit('close')"
            />
            <kbd class="palette-kbd">Esc</kbd>
          </div>

          <!-- 结果列表 -->
          <ul ref="listEl" class="palette-list">
            <li v-if="items.length === 0" class="palette-empty">没有匹配的命令</li>
            <li
              v-for="(item, idx) in items"
              :key="item.key"
              class="palette-item"
              :class="{ active: idx === activeIndex }"
              @mouseenter="activeIndex = idx"
              @click="runItem(item)"
            >
              <AppIcon :name="item.icon" :size="14" class="item-glyph" />
              <span class="item-title">{{ item.title }}</span>
              <span v-if="item.hint" class="item-hint">{{ item.hint }}</span>
            </li>
          </ul>

          <!-- 底部提示 -->
          <div class="palette-foot">
            <span>
              <kbd class="palette-kbd">↑↓</kbd>
              选择
            </span>
            <span>
              <kbd class="palette-kbd">Enter</kbd>
              执行
            </span>
            <span>
              <kbd class="palette-kbd">Esc</kbd>
              关闭
            </span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../ui/AppIcon.vue'
import { useChatStore } from '../../stores/chat'
import { useThemeStore } from '../../stores/theme'
import { useModelSelector } from '../../composables/useModelSelector'

const props = defineProps<{ open: boolean }>()

const emit = defineEmits<{
  close: []
  'toggle-console': []
}>()

const router = useRouter()
const chatStore = useChatStore()
const themeStore = useThemeStore()
// 与 TopCommandBar 共享同一单例：列表只拉一次，状态天然同步
const { modelOptions, switchToModel, isModelSwitching } = useModelSelector()

/** 图标字面量联合必须为 AppIcon IconName 的子集 */
type Glyph = 'plus' | 'book' | 'settings' | 'theme-sun' | 'theme-moon' | 'search' | 'document'

interface PaletteItem {
  key: string
  icon: Glyph
  title: string
  hint?: string
  keywords?: string
  run: () => void
}

const query = ref('')
const activeIndex = ref(0)
const inputEl = ref<HTMLInputElement>()
const listEl = ref<HTMLUListElement>()

// ===================== 命令源 =====================
const fixedCommands = computed<PaletteItem[]>(() => [
  {
    key: 'cmd:new-chat',
    icon: 'plus',
    title: '新建对话',
    keywords: 'new chat',
    run: () => {
      chatStore.createConversation()
      router.push('/')
    }
  },
  {
    key: 'cmd:knowledge',
    icon: 'book',
    title: '打开知识库',
    keywords: 'knowledge rag',
    run: () => router.push('/knowledge')
  },
  {
    key: 'cmd:settings',
    icon: 'settings',
    title: '打开设置',
    keywords: 'settings',
    run: () => router.push('/settings')
  },
  {
    key: 'cmd:theme',
    icon: themeStore.isDark ? 'theme-sun' : 'theme-moon',
    title: themeStore.isDark ? '切换到晨读模式' : '切换到夜读模式',
    keywords: 'theme dark light',
    run: () => themeStore.toggleTheme()
  },
  {
    key: 'cmd:console',
    icon: 'search',
    title: '服务器控制台',
    keywords: 'console log terminal',
    run: () => emit('toggle-console')
  }
])

const modelCommands = computed<PaletteItem[]>(() =>
  modelOptions.value.map(m => ({
    key: 'model:' + m.value,
    icon: 'document',
    title: m.label,
    hint: [m.sizeLabel, m.quantType].filter(Boolean).join(' · ') || undefined,
    keywords: ('切换模型 model ' + (m.fullName || '')).toLowerCase(),
    run: () => {
      if (!isModelSwitching.value) void switchToModel(m.value)
    }
  }))
)

const conversationCommands = computed<PaletteItem[]>(() =>
  chatStore.conversations.slice(0, 30).map(c => ({
    key: 'conv:' + c.id,
    icon: 'search',
    title: c.title,
    hint: '会话',
    keywords: ('打开会话 conversation ' + c.title).toLowerCase(),
    run: () => {
      router.push('/')
      void chatStore.selectConversation(c.id)
    }
  }))
)

const items = computed<PaletteItem[]>(() => {
  const kw = query.value.trim().toLowerCase()
  if (!kw) {
    // 空查询：固定动作优先，其后展示最近若干会话作为快捷入口
    return [...fixedCommands.value, ...conversationCommands.value.slice(0, 6)]
  }
  const all = [...fixedCommands.value, ...modelCommands.value, ...conversationCommands.value]
  return all.filter(i => i.title.toLowerCase().includes(kw) || (i.keywords || '').includes(kw))
})

// ===================== 键盘导航 =====================
function move(delta: number) {
  const len = items.value.length
  if (len === 0) return
  activeIndex.value = (activeIndex.value + delta + len) % len
  nextTick(() => {
    listEl.value?.querySelector('.palette-item.active')?.scrollIntoView({ block: 'nearest' })
  })
}

function runActive() {
  const item = items.value[activeIndex.value]
  if (item) runItem(item)
}

function runItem(item: PaletteItem) {
  item.run()
  emit('close')
}

// ===================== 开合生命周期 =====================
watch(
  () => props.open,
  open => {
    if (!open) return
    query.value = ''
    activeIndex.value = 0
    nextTick(() => inputEl.value?.focus())
  }
)
</script>

<style scoped>
.palette-veil {
  position: fixed;
  inset: 0;
  z-index: calc(var(--z-command-bar) + 40);
  display: flex;
  justify-content: center;
  /* 略偏上放置，符合启动器类交互惯例 */
  align-items: flex-start;
  padding-top: 14vh;
  background: var(--surface-veil);
}

.palette-card {
  width: min(520px, calc(100vw - 48px));
  max-height: 60vh;
  display: flex;
  flex-direction: column;
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}

.palette-pop-enter-active,
.palette-pop-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s cubic-bezier(0.4, 0, 0.2, 1);
}

.palette-pop-enter-from,
.palette-pop-leave-to {
  opacity: 0;
  transform: translateY(-8px) scale(0.985);
}

/* ===== 输入行 ===== */
.palette-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 16px;
  border-bottom: 1px solid var(--border-light);
}

.input-glyph {
  color: var(--text-muted);
  flex-shrink: 0;
}

.palette-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font-family: inherit;
  font-size: 14px;
  color: var(--text-primary);
}

.palette-input::placeholder {
  color: var(--text-muted);
}

/* ===== 结果列表 ===== */
.palette-list {
  flex: 1;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 6px;
}

.palette-empty {
  padding: 28px 0;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
}

.palette-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 11px;
  border-radius: var(--border-radius-sm);
  cursor: pointer;
}

.palette-item.active {
  background: var(--bg-active);
}

.item-glyph {
  color: var(--text-muted);
  flex-shrink: 0;
}

.palette-item.active .item-glyph {
  color: var(--accent-primary);
}

.item-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--text-primary);
}

.item-hint {
  flex-shrink: 0;
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-muted);
}

/* ===== 底部提示 ===== */
.palette-foot {
  display: flex;
  gap: 16px;
  padding: 8px 14px;
  border-top: 1px solid var(--border-light);
  font-size: 11px;
  color: var(--text-muted);
}

.palette-kbd {
  font-family: var(--font-mono);
  font-size: 10px;
  line-height: 1;
  padding: 3px 5px;
  border: 1px solid var(--border-light);
  border-bottom-width: 2px;
  border-radius: 3px;
  background: var(--bg-secondary);
  color: var(--text-muted);
}
</style>
