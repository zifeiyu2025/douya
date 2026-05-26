<template>
  <div class="sidebar" :class="{ collapsed }">
    <div class="sidebar-header" style="--wails-draggable:drag">
      <div class="sidebar-logo" style="--wails-draggable:no-drag">
        <div class="logo-text">
          <span class="logo-dou">Dou</span><span class="logo-ya">Ya</span>
        </div>
      </div>
    </div>
    <div class="sidebar-search">
      <div class="create-btn-wrapper">
        <n-button type="primary" block @click="handleCreate" size="medium" class="create-btn">
          <template #icon>
            <n-icon><AddOutline /></n-icon>
          </template>
          新建对话
        </n-button>
      </div>
      <n-input v-model:value="searchQuery" placeholder="搜索对话" clearable size="medium" class="search-input">
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
    </div>
    <div class="conversation-list">
      <div v-if="chatStore.isLoadingConversations" class="loading-container">
        <div class="loading-spinner"></div>
        <span class="loading-text">加载对话中...</span>
      </div>
      <template v-else>
        <div
          v-for="conv in filteredConversations"
          :key="conv.id"
          class="conversation-item"
          :class="{ active: conv.id === chatStore.currentConversationId }"
          @click="handleSelect(conv.id)"
          @contextmenu.prevent="handleContextMenu($event, conv)"
        >
          <div class="conversation-avatar">
            {{ getAvatarText(conv.title) }}
          </div>
          <div class="conversation-item-info">
            <div class="conversation-item-title" :title="fixUtf8(conv.title) || '新对话'">
              {{ fixUtf8(conv.title) || '新对话' }}
            </div>
            <div class="conversation-item-preview">
              <template v-if="chatStore.generatingConvId === conv.id">生成中...</template>
              <template v-else>{{ getPreview(conv) }}</template>
            </div>
          </div>
          <n-dropdown
            :options="contextMenuOptions"
            :show="contextMenuConv?.id === conv.id"
            :x="contextMenuX"
            :y="contextMenuY"
            placement="bottom-start"
            @select="handleContextAction($event, conv)"
            @clickoutside="contextMenuConv = null"
          />
        </div>
      </template>
    </div>
    <div class="sidebar-footer">
      <div class="sidebar-footer-actions">
        <n-button quaternary @click="$router.push('/knowledge')" size="medium" class="footer-btn">
          <template #icon>
            <n-icon size="18"><BookOutline /></n-icon>
          </template>
          <span>知识库</span>
        </n-button>
        <n-button quaternary @click="$router.push('/settings')" size="medium" class="footer-btn">
          <template #icon>
            <n-icon size="18"><SettingsOutline /></n-icon>
          </template>
          <span>设置</span>
        </n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h } from 'vue'
import { NButton, NIcon, NInput, NDropdown, useDialog, useMessage } from 'naive-ui'
import { AddOutline, SearchOutline, SettingsOutline, BookOutline, PencilOutline, DocumentTextOutline, CodeSlashOutline, TrashOutline, FileTrayFullOutline, GridOutline } from '@vicons/ionicons5'
import { useChatStore } from '../stores/chat'
import { fixUtf8 } from '../utils/utf8'
import type { Conversation } from '../services/wails'

const props = defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ toggle: [] }>()

const chatStore = useChatStore()
const dialog = useDialog()
const message = useMessage()

const searchQuery = ref('')
const contextMenuConv = ref<Conversation | null>(null)
const contextMenuX = ref(0)
const contextMenuY = ref(0)

const contextMenuOptions = [
  { 
    key: 'rename',
    label: () => h('div', { class: 'context-menu-item' }, [
      h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(PencilOutline) }),
      h('span', { class: 'menu-text' }, '重命名')
    ])
  },
  { 
    key: 'export-md',
    label: () => h('div', { class: 'context-menu-item' }, [
      h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(DocumentTextOutline) }),
      h('span', { class: 'menu-text' }, '导出 Markdown')
    ])
  },
  { 
    key: 'export-json',
    label: () => h('div', { class: 'context-menu-item' }, [
      h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(CodeSlashOutline) }),
      h('span', { class: 'menu-text' }, '导出 JSON')
    ])
  },
  { 
    key: 'export-txt',
    label: () => h('div', { class: 'context-menu-item' }, [
      h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(FileTrayFullOutline) }),
      h('span', { class: 'menu-text' }, '导出纯文本')
    ])
  },
  { 
    key: 'export-csv',
    label: () => h('div', { class: 'context-menu-item' }, [
      h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(GridOutline) }),
      h('span', { class: 'menu-text' }, '导出 CSV (微调)')
    ])
  },
  { type: 'divider', key: 'divider' },
  { 
    key: 'delete',
    props: { style: { color: 'var(--accent-danger)' } },
    label: () => h('div', { class: 'context-menu-item danger' }, [
      h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(TrashOutline) }),
      h('span', { class: 'menu-text' }, '删除')
    ])
  },
]

const filteredConversations = computed(() => {
  if (!searchQuery.value) return chatStore.conversations
  const q = searchQuery.value.toLowerCase()
  return chatStore.conversations.filter(c =>
    c.title.toLowerCase().includes(q)
  )
})

function getAvatarText(title: string): string {
  const fixedTitle = fixUtf8(title)
  if (!fixedTitle || fixedTitle === '新对话') return '新'
  return fixedTitle.charAt(0).toUpperCase()
}

function getPreview(conv: Conversation): string {
  return formatTime(conv.updated_at)
}

function formatTime(dateStr: string): string {
  const d = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  const oneDay = 24 * 60 * 60 * 1000

  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < oneDay) {
    const hour = String(d.getHours()).padStart(2, '0')
    const minute = String(d.getMinutes()).padStart(2, '0')
    return `${hour}:${minute}`
  }
  if (diff < oneDay * 2) return '昨天'
  if (diff < oneDay * 7) {
    const weekDays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
    return weekDays[d.getDay()]
  }
  return `${d.getMonth() + 1}/${d.getDate()}`
}

async function handleCreate() {
  await chatStore.createConversation()
}

function handleSelect(id: string) {
  chatStore.selectConversation(id)
}

function handleContextMenu(e: MouseEvent, conv: Conversation) {
  contextMenuConv.value = conv
  contextMenuX.value = e.clientX
  contextMenuY.value = e.clientY
}

async function handleContextAction(key: string, conv: Conversation) {
  contextMenuConv.value = null
  switch (key) {
    case 'rename':
      const input = ref(conv.title)
      dialog.create({
        title: '重命名对话',
        content: () => {
          return h(NInput, {
            value: input.value,
            'onUpdate:value': (v: string) => { input.value = v },
            placeholder: '请输入新标题',
          })
        },
        positiveText: '确定',
        negativeText: '取消',
        onPositiveClick: async () => {
          await chatStore.renameConversation(conv.id, input.value)
        },
      })
      break
    case 'export-md':
      await handleExport(conv.id, 'markdown')
      break
    case 'export-json':
      await handleExport(conv.id, 'json')
      break
    case 'export-txt':
      await handleExport(conv.id, 'txt')
      break
    case 'export-csv':
      await handleExport(conv.id, 'csv')
      break
    case 'delete':
      dialog.warning({
        title: '删除对话',
        content: '确定要删除这个对话吗？此操作不可撤销。',
        positiveText: '删除',
        negativeText: '取消',
        onPositiveClick: async () => {
          await chatStore.deleteConversation(conv.id)
          message.success('已删除')
        },
      })
      break
  }
}

async function handleExport(id: string, format: string) {
    const success = await chatStore.exportConversationWithDialog(id, format)
    if (success) {
        message.success('导出成功')
    }
}
</script>

<style scoped>
.sidebar-logo {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--border-color);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.loading-text {
  font-size: 14px;
  color: var(--text-secondary);
}

.logo-text {
  font-size: 28px;
  font-weight: 800;
  letter-spacing: 2px;
  line-height: 1;
  text-transform: uppercase;
}

.logo-dou {
  color: var(--text-primary);
}

.logo-ya {
  color: var(--accent-primary);
}

.create-btn-wrapper {
  margin-bottom: 12px;
}

.create-btn {
  border-radius: 12px !important;
  height: 44px !important;
  font-weight: 600;
}

.search-input {
  border-radius: 12px !important;
}

.footer-btn {
  border-radius: 12px !important;
  display: flex !important;
  gap: 8px;
  padding: 10px 12px !important;
  height: 44px !important;
  flex: 1;
  justify-content: center;
  font-weight: 500;
}
</style>
