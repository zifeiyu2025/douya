<template>
  <div class="sidebar" :class="{ collapsed }">
    <!-- Logo 区：appicon + 品牌名横向排列，左对齐作为窗口左上角视觉锚点 -->
    <div class="sidebar-header" style="--wails-draggable: drag">
      <div class="sidebar-logo">
        <img :src="appLogo" alt="豆芽" class="logo-image" draggable="false" />
        <span class="logo-wordmark">DOUYA</span>
      </div>
    </div>
    <!-- SubTask 10.3/10.4: 新建对话按钮去 chrome + 搜索框密度优化 -->
    <div class="sidebar-search">
      <button class="create-btn" @click="handleCreate">
        <n-icon class="create-btn-icon" :size="16"><AddOutline /></n-icon>
        <span class="create-btn-text">新建对话</span>
      </button>
      <n-input
        v-model:value="searchQuery"
        placeholder="搜索对话"
        clearable
        size="small"
        class="search-input"
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
    </div>
    <!-- SubTask 10.2: 会话列表去卡片化 — row 布局，无边框，hover/active 用 bg 层级 -->
    <!-- C-3: 列表按更新时间分为 今天/昨天/最近7天/更早 四组，组间以小字标题分隔 -->
    <div class="conversation-list">
      <div v-if="chatStore.isLoadingConversations" class="loading-container">
        <div class="loading-spinner"></div>
        <span class="loading-text">加载对话中...</span>
      </div>
      <template v-else>
        <template v-if="groupedConversations.length > 0">
          <section
            v-for="group in groupedConversations"
            :key="group.key"
            class="conversation-group"
          >
            <div class="group-label">{{ group.label }}</div>
            <div
              v-for="{ conv, staggerIdx } in group.items"
              :key="conv.id"
              class="conversation-item"
              :class="{ active: conv.id === chatStore.currentConversationId }"
              :style="{ '--stagger-idx': staggerIdx }"
              role="button"
              tabindex="0"
              :aria-label="`会话：${fixUtf8(conv.title) || '新对话'}`"
              :aria-current="conv.id === chatStore.currentConversationId ? 'true' : undefined"
              @click="handleSelect(conv.id)"
              @keydown.enter.prevent="handleSelect(conv.id)"
              @contextmenu.prevent="handleContextMenu($event, conv)"
            >
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
          </section>
        </template>
        <!-- 搜索无结果时的空态提示 -->
        <div v-else-if="searchQuery" class="empty-search">未找到相关对话</div>
      </template>
    </div>
    <!-- SubTask 10.4: 底部入口 row 布局，hover bg-hover，无边框 -->
    <div class="sidebar-footer">
      <div class="sidebar-footer-actions">
        <button class="footer-btn" @click="$router.push('/knowledge')">
          <n-icon :size="16"><BookOutline /></n-icon>
          <span>知识库</span>
        </button>
        <button class="footer-btn" @click="$router.push('/settings')">
          <n-icon :size="16"><SettingsOutline /></n-icon>
          <span>设置</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h } from 'vue'
import { NIcon, NInput, NDropdown, useDialog, useMessage } from 'naive-ui'
import {
  AddOutline,
  SearchOutline,
  SettingsOutline,
  BookOutline,
  PencilOutline,
  DocumentTextOutline,
  CodeSlashOutline,
  TrashOutline,
  FileTrayFullOutline,
  GridOutline
} from '@vicons/ionicons5'
import { useChatStore } from '../../stores/chat'
import { fixUtf8 } from '../../utils/utf8'
import { showSuccess } from '../../utils/showError'
import type { Conversation } from '../../services/wails'
import appLogo from '../../assets/images/appicon.png'

defineProps<{ collapsed: boolean }>()

const chatStore = useChatStore()
const dialog = useDialog()
const message = useMessage()

const searchQuery = ref('')
const contextMenuConv = ref<Conversation | null>(null)
const contextMenuX = ref(0)
const contextMenuY = ref(0)

function createMenuItem(key: string, icon: any, text: string, danger = false) {
  return {
    key,
    props: danger ? { style: { color: 'var(--accent-danger)' } } : undefined,
    label: () =>
      h('div', { class: `context-menu-item${danger ? ' danger' : ''}` }, [
        h(NIcon, { size: 16, class: 'menu-icon' }, { default: () => h(icon) }),
        h('span', { class: 'menu-text' }, text)
      ])
  }
}

const contextMenuOptions = [
  createMenuItem('rename', PencilOutline, '重命名'),
  createMenuItem('export-md', DocumentTextOutline, '导出 Markdown'),
  createMenuItem('export-json', CodeSlashOutline, '导出 JSON'),
  createMenuItem('export-txt', FileTrayFullOutline, '导出纯文本'),
  createMenuItem('export-csv', GridOutline, '导出 CSV'),
  { type: 'divider', key: 'divider' },
  createMenuItem('delete', TrashOutline, '删除', true)
]

// ----- 会话日期分组（C-3）-----
// 按更新时间把会话归入四组：今天 / 昨天 / 最近 7 天 / 更早。
// 分组依据"日历天"而非毫秒差：跨午夜后"23 小时前"的会话应显示为"昨天"而非"今天"。
type DateGroupKey = 'today' | 'yesterday' | 'week' | 'older'

const GROUP_LABELS: Record<DateGroupKey, string> = {
  today: '今天',
  yesterday: '昨天',
  week: '最近 7 天',
  older: '更早'
}

const GROUP_ORDER: DateGroupKey[] = ['today', 'yesterday', 'week', 'older']

/** 取某天的零点时间戳（本地时区），用于按日历天比较 */
function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

/** 把更新时间归类到分组键；解析失败时安全回落到"更早" */
function classifyByDate(dateStr: string): DateGroupKey {
  const d = new Date(dateStr)
  const dayDiff = Math.floor((startOfDay(new Date()) - startOfDay(d)) / (24 * 60 * 60 * 1000))
  if (Number.isNaN(dayDiff)) return 'older'
  if (dayDiff <= 0) return 'today'
  if (dayDiff === 1) return 'yesterday'
  if (dayDiff < 7) return 'week'
  return 'older'
}

const filteredConversations = computed(() => {
  if (!searchQuery.value) return chatStore.conversations
  const q = searchQuery.value.toLowerCase()
  return chatStore.conversations.filter(c => c.title.toLowerCase().includes(q))
})

/** 过滤后的会话按日期分组；空组不出现，组内保持原排序，stagger 索引跨组连续递增 */
const groupedConversations = computed(() => {
  const buckets = new Map<DateGroupKey, Conversation[]>()
  for (const conv of filteredConversations.value) {
    const key = classifyByDate(conv.updated_at)
    const list = buckets.get(key)
    if (list) list.push(conv)
    else buckets.set(key, [conv])
  }
  let staggerIdx = 0
  return GROUP_ORDER.filter(key => buckets.has(key)).map(key => ({
    key,
    label: GROUP_LABELS[key],
    items: buckets.get(key)!.map(conv => ({ conv, staggerIdx: Math.min(staggerIdx++, 12) }))
  }))
})

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

function handleCreate() {
  chatStore.createConversation()
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
  const exportFormatMap: Record<string, string> = {
    'export-md': 'markdown',
    'export-json': 'json',
    'export-txt': 'txt',
    'export-csv': 'csv'
  }
  if (exportFormatMap[key]) {
    await handleExport(conv.id, exportFormatMap[key])
    return
  }
  switch (key) {
    case 'rename': {
      const input = ref(conv.title)
      dialog.create({
        title: '重命名对话',
        content: () => {
          return h(NInput, {
            value: input.value,
            'onUpdate:value': (v: string) => {
              input.value = v
            },
            placeholder: '请输入新标题'
          })
        },
        positiveText: '确定',
        negativeText: '取消',
        onPositiveClick: async () => {
          await chatStore.renameConversation(conv.id, input.value)
        }
      })
      break
    }
    case 'delete':
      dialog.warning({
        title: '删除对话',
        content: () =>
          h('div', { class: 'delete-confirm-content' }, [
            h('div', { class: 'delete-confirm-row' }, [
              h(
                NIcon,
                { size: 18, class: 'delete-confirm-icon' },
                { default: () => h(TrashOutline) }
              ),
              h('span', { class: 'delete-confirm-text' }, '确定要删除这个对话吗？')
            ]),
            h('div', { class: 'delete-confirm-hint' }, '此操作不可撤销，对话将永久消失')
          ]),
        positiveText: '删除',
        negativeText: '取消',
        onPositiveClick: async () => {
          await chatStore.deleteConversation(conv.id)
          showSuccess(message, '已删除')
        }
      })
      break
  }
}

async function handleExport(id: string, format: string) {
  const success = await chatStore.exportConversationWithDialog(id, format)
  if (success) {
    showSuccess(message, '导出成功')
  }
}
</script>

<style scoped>
/* ===== Logo 区：appicon + 品牌名横向排列，左对齐 =====
 * 作为窗口左上角的视觉锚点，采用左对齐（非居中）
 * LOGO 图像 24px + 品牌名 18px 横向排列，克制精致
 * 用实色（避免 background-clip:text 在 WebView2 中不可靠）
 */
.sidebar-header {
  justify-content: flex-start; /* 覆盖全局 center，改为左对齐 */
  padding: 8px 16px;
}

.sidebar-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  user-select: none;
}

/* LOGO 图像：appicon.png
 * 24px 尺寸，圆角 6px，微妙阴影增强立体感
 * 避免使用 filter:drop-shadow 在容器上，直接用 box-shadow
 */
.logo-image {
  width: 24px;
  height: 24px;
  border-radius: 6px;
  object-fit: cover;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
  user-select: none;
  -webkit-user-drag: none;
  flex-shrink: 0;
}

/* 暗色主题下 LOGO 阴影调整 */
:global(body.theme-dark) .logo-image {
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
}

.logo-wordmark {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 3px;
  line-height: 1;
  color: var(--accent-primary);
  user-select: none;
  padding-left: 3px; /* 视觉补偿 letter-spacing 尾部留白 */
}

/* ===== 加载状态 ===== */
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;
}

.loading-spinner {
  width: 28px;
  height: 28px;
  border: 2px solid var(--border-color);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.loading-text {
  font-size: 12px;
  color: var(--text-muted);
}

/* ===== C-3: 日期分组 =====
 * 组标题：小字号弱化文本，仅作视觉分隔，不参与交互
 */
.conversation-group {
  display: flex;
  flex-direction: column;
}

.group-label {
  padding: 10px 12px 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  user-select: none;
}

/* 搜索无结果空态 */
.empty-search {
  padding: 24px 16px;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
}

/* ===== SubTask 10.2: 会话列表去卡片化 — row 布局 =====
 * 覆盖 style.css 全局 .conversation-item 的边框、padding、背景
 * 移除每项 border-bottom、avatar；hover/active 用 bg 层级而非 card 边框
 */
.conversation-item {
  padding: 8px 12px;
  cursor: pointer;
  transition: background var(--transition-fast);
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
  border: none;
  border-radius: 0;
  background: transparent;
  animation: item-slide-in 0.4s cubic-bezier(0.4, 0, 0.2, 1) both;
  animation-delay: calc(var(--stagger-idx, 0) * 30ms);
  will-change: opacity, transform;
}

.conversation-item:hover {
  background: var(--bg-hover);
}

.conversation-item.active {
  background: var(--bg-active);
}

/* 键盘聚焦视觉反馈（无障碍）：Tab 键聚焦时显示蓝色描边 */
.conversation-item:focus-visible {
  outline: 2px solid var(--accent-primary);
  outline-offset: -2px;
}

.conversation-item-info {
  flex: 1;
  overflow: hidden;
  min-width: 0;
}

.conversation-item-title {
  font-size: 14px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--text-primary);
  line-height: 1.4;
  margin-bottom: 2px;
}

.conversation-item-preview {
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

/* ===== SubTask 10.3: 新建对话按钮去 chrome =====
 * 无边框、无背景（hover 显示 bg-hover）；图标 accent-primary 强调
 * 强排版 14px/500；宽度填满内容区
 */
.create-btn {
  width: 100%;
  height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  background: transparent;
  border: none;
  border-radius: var(--border-radius-sm);
  color: var(--text-primary);
  font-size: 14px;
  font-weight: 500;
  font-family: inherit;
  line-height: 1;
  cursor: pointer;
  transition: background var(--transition-fast);
  margin-bottom: 8px;
  appearance: none;
  -webkit-appearance: none;
}

.create-btn:hover {
  background: var(--bg-hover);
}

.create-btn-icon {
  color: var(--accent-primary);
}

.create-btn-text {
  color: var(--text-primary);
}

/* ===== SubTask 10.4: 搜索框密度优化（32px 高度，border-radius-sm）===== */
.search-input {
  border-radius: var(--border-radius-sm);
}

.search-input :deep(.n-input) {
  height: 32px;
  border-radius: var(--border-radius-sm);
}

.search-input :deep(.n-input-wrapper) {
  border-radius: var(--border-radius-sm);
  height: 32px;
  min-height: 32px;
}

.search-input :deep(.n-input__input-el) {
  height: 32px;
}

/* ===== SubTask 10.4: 底部入口 row 布局 =====
 * 无边框；图标用 --text-secondary，hover 时 --text-primary
 * hover 背景 --bg-hover（背景图模式下由 style.css .has-background 规则覆盖为毛玻璃）
 */
.footer-btn {
  flex: 1;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 0 12px;
  background: transparent;
  border: none;
  border-radius: var(--border-radius-sm);
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 500;
  font-family: inherit;
  line-height: 1;
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
  appearance: none;
  -webkit-appearance: none;
}

.footer-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
</style>
