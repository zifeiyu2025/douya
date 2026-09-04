<template>
  <!-- 常驻会话侧栏：
       嵌入壳层的固定左栏；
       收起时宽度归零（内容按定宽裁切，不回流抖动），
       分组语义由 utils/conversationGroups 纯函数提供 -->
  <aside class="session-sidebar" :class="{ collapsed }" role="navigation" aria-label="会话列表">
    <div class="sidebar-inner">
      <!-- 头部：标题 + 新建 / 收起 -->
      <div class="drawer-head">
        <span class="drawer-title">会话</span>
        <span class="head-spacer"></span>
        <button type="button" class="head-btn" title="新建对话" @click="handleCreate">
          <AppIcon name="plus" :size="16" />
        </button>
        <button type="button" class="head-btn" title="收起侧栏" @click="emit('collapse')">
          <AppIcon name="back" :size="15" />
        </button>
      </div>

      <!-- 搜索行：即时过滤标题 + 全文搜索入口（对标 ChatGPT/Claude 对话历史搜索） -->
      <div class="search-row">
        <AppIcon name="search" :size="13" class="search-glyph" />
        <input
          v-model="keyword"
          class="search-input"
          type="text"
          :placeholder="searchMode === 'fulltext' ? '搜索消息内容…' : '检索会话…'"
          spellcheck="false"
          @keydown.enter="enterFulltext"
        />
        <button v-if="keyword" type="button" class="search-clear" title="清空" @click="resetSearch">
          <AppIcon name="close" :size="11" />
        </button>
      </div>

      <!-- 标题过滤模式下：提示可扩展为全文搜索 -->
      <button
        v-if="searchMode === 'title' && keyword.trim()"
        type="button"
        class="fulltext-entry"
        @click="enterFulltext"
      >
        <AppIcon name="search" :size="12" />
        <span>在全部对话中搜索「{{ keyword.trim() }}」</span>
      </button>

      <!-- 分组列表 -->
      <div class="group-scroll">
        <!-- ===== 全文搜索模式：结果按会话聚合展示 ===== -->
        <template v-if="searchMode === 'fulltext'">
          <p v-if="searching" class="empty-hint">正在搜索…</p>
          <template v-else-if="searchGroups.length > 0">
            <section
              v-for="group in searchGroups"
              :key="group.convId"
              class="conversation-group search-group"
            >
              <p class="group-label search-group-label">
                {{ group.title }}
                <span class="search-group-count">{{ group.items.length }} 条</span>
              </p>
              <ul class="group-list">
                <li
                  v-for="hit in group.items"
                  :key="hit.id"
                  class="search-hit"
                  @click="handleLocate(group.convId, hit.id)"
                >
                  <span class="search-hit-role" :class="'search-hit-role--' + hit.role">
                    {{ hit.role === 'user' ? '我' : 'AI' }}
                  </span>
                  <span class="search-hit-snippet">{{ snippet(hit.content) }}</span>
                </li>
              </ul>
            </section>
          </template>
          <p v-else class="empty-hint">没有找到包含「{{ keyword }}」的消息</p>
        </template>

        <!-- ===== 标题过滤模式：分组会话列表 ===== -->
        <template v-else>
          <p v-if="groups.length === 0" class="empty-hint">
            {{ keyword ? '没有匹配的会话' : '还没有会话，点上方 + 开始吧' }}
          </p>

          <section v-for="group in groups" :key="group.key" class="conversation-group">
            <p class="group-label">{{ group.label }}</p>
            <ul class="group-list">
              <li
                v-for="item in group.items"
                :key="item.conv.id"
                class="conversation-item"
                :class="{ active: item.conv.id === chatStore.currentConversationId }"
                :style="{ '--stagger-idx': item.staggerIdx }"
                @click="handleSelect(item.conv.id)"
                @contextmenu.prevent="openMenu($event, item.conv)"
              >
                <template v-if="renamingId === item.conv.id">
                  <input
                    ref="renameInputEl"
                    v-model="renamingText"
                    class="rename-input"
                    type="text"
                    spellcheck="false"
                    @click.stop
                    @keydown.enter.prevent="commitRename"
                    @keydown.esc.prevent="cancelRename"
                    @blur="commitRename"
                  />
                </template>
                <template v-else>
                  <span class="conversation-item-title">{{ item.conv.title }}</span>
                  <span class="item-time">{{ formatRelativeTime(item.conv.updated_at) }}</span>
                </template>
              </li>
            </ul>
          </section>
        </template>
      </div>

      <!-- 底部导航：知识库 / 设置 -->
      <nav class="drawer-foot">
        <button type="button" class="foot-link" @click="go('/knowledge')">
          <AppIcon name="book" :size="15" />
          <span>知识库</span>
        </button>
        <button type="button" class="foot-link" @click="go('/settings')">
          <AppIcon name="settings" :size="15" />
          <span>设置</span>
        </button>
      </nav>
    </div>
  </aside>

  <!-- 右键菜单：Teleport 到 body，避免被侧栏 overflow:hidden 裁切 -->
  <Teleport to="body">
    <div
      v-if="menu.visible"
      class="ctx-veil"
      @click.prevent="closeMenu"
      @contextmenu.prevent="closeMenu"
    >
      <div class="ctx-menu" :style="{ left: menu.x + 'px', top: menu.y + 'px' }">
        <button type="button" class="ctx-item" @click.stop="startRename(menu.conv!)">
          <AppIcon name="edit" :size="13" />
          重命名
        </button>
        <button type="button" class="ctx-item" @click.stop="exportAs(menu.conv!, 'markdown')">
          <AppIcon name="export-md" :size="13" />
          导出 Markdown
        </button>
        <button type="button" class="ctx-item" @click.stop="exportAs(menu.conv!, 'txt')">
          <AppIcon name="export-txt" :size="13" />
          导出纯文本
        </button>
        <span class="ctx-divider"></span>
        <button type="button" class="ctx-item ctx-danger" @click.stop="removeConv(menu.conv!)">
          <AppIcon name="trash" :size="13" />
          删除会话
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../ui/AppIcon.vue'
import { useChatStore } from '../../stores/chat'
import { discreteDialog, discreteMessage } from '../../utils/discrete'
import {
  groupConversationsByDate,
  formatRelativeTime,
  type ConversationGroup
} from '../../utils/conversationGroups'
import type { Conversation, Message } from '../../services/wails'

// collapsed 由 App 层持有（顶栏按钮与侧栏共用同一状态），
// true 时侧栏宽度收拢为 0，主区自动吃满
defineProps<{
  collapsed?: boolean
}>()

const emit = defineEmits<{
  collapse: []
}>()

const router = useRouter()
const chatStore = useChatStore()

// ===================== 过滤与分组 =====================
const keyword = ref('')
// 搜索模式：title = 仅过滤会话标题（默认）；fulltext = 消息内容全文搜索
const searchMode = ref<'title' | 'fulltext'>('title')
const groups = computed<ConversationGroup[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  const source = kw
    ? chatStore.conversations.filter(c => c.title.toLowerCase().includes(kw))
    : chatStore.conversations
  return groupConversationsByDate(source)
})

// ===================== 全文搜索（对标 ChatGPT/Claude 历史搜索） =====================
// 后端 SearchMessages 在最近 N 条消息内做内存解密匹配（内容/思考/RAG/工具调用），
// 这里仅负责"接线 + 展示"：结果按会话聚合，点击跳转到消息并高亮。
const searchResults = ref<Message[]>([])
const searching = ref(false)
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null

interface SearchGroup {
  convId: string
  title: string
  items: Message[]
}

/** 结果按会话聚合，会话标题映射自实时会话列表（会话被删时显示兜底标题） */
const searchGroups = computed<SearchGroup[]>(() => {
  const map = new Map<string, SearchGroup>()
  for (const hit of searchResults.value) {
    let g = map.get(hit.conversation_id)
    if (!g) {
      const conv = chatStore.conversations.find(c => c.id === hit.conversation_id)
      g = {
        convId: hit.conversation_id,
        title: conv?.title || '（已删除的会话）',
        items: []
      }
      map.set(hit.conversation_id, g)
    }
    g.items.push(hit)
  }
  return [...map.values()]
})

/** 执行全文搜索（防抖 300ms，输入暂停后才请求） */
function runFulltextSearch() {
  const q = keyword.value.trim()
  if (!q) {
    searchResults.value = []
    return
  }
  searching.value = true
  void chatStore.searchMessages(q).then(msgs => {
    // 结果只保留最近仍存在的会话消息（会话删除后旧结果应消失）
    const validIds = new Set(chatStore.conversations.map(c => c.id))
    searchResults.value = msgs.filter(m => validIds.has(m.conversation_id))
    searching.value = false
  })
}

function enterFulltext() {
  if (!keyword.value.trim()) return
  searchMode.value = 'fulltext'
  runFulltextSearch()
}

/** 清空搜索并返回标题过滤模式 */
function resetSearch() {
  keyword.value = ''
  searchMode.value = 'title'
  searchResults.value = []
  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer)
    searchDebounceTimer = null
  }
}

/** 点击搜索结果：打开会话并定位高亮到该消息 */
function handleLocate(convId: string, messageId: string) {
  void chatStore.selectConversationAndLocate(convId, messageId)
  resetSearch()
}

/** 消息摘要：压平空白后截断，超出加省略号 */
function snippet(text: string, max = 80): string {
  const flat = text.replace(/\s+/g, ' ').trim()
  if (!flat) return '（无文本内容）'
  return flat.length > max ? flat.slice(0, max) + '…' : flat
}

// fulltext 模式下继续输入：防抖重新搜索
watch(keyword, () => {
  if (searchMode.value !== 'fulltext') return
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  searchDebounceTimer = setTimeout(() => {
    searchDebounceTimer = null
    runFulltextSearch()
  }, 300)
})

// 组件卸载时清理全文搜索防抖定时器
onUnmounted(() => {
  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer)
    searchDebounceTimer = null
  }
})

// ===================== 选中 / 新建 / 导航 =====================
// 常驻侧栏不再"选完即关"，保持展开让用户连续切换
function handleSelect(id: string) {
  void chatStore.selectConversation(id)
}

function handleCreate() {
  chatStore.createConversation()
}

function go(path: string) {
  router.push(path)
}

// ===================== 行内重命名 =====================
const renamingId = ref('')
const renamingText = ref('')
const renameInputEl = ref<HTMLInputElement | HTMLInputElement[]>([])

function startRename(conv: Conversation) {
  closeMenu()
  renamingId.value = conv.id
  renamingText.value = conv.title
  nextTick(() => {
    const el = Array.isArray(renameInputEl.value) ? renameInputEl.value[0] : renameInputEl.value
    el?.focus()
    el?.select()
  })
}

function commitRename() {
  const id = renamingId.value
  if (!id) return
  const title = renamingText.value.trim()
  if (title) {
    void chatStore.renameConversation(id, title)
  }
  renamingId.value = ''
}

function cancelRename() {
  renamingId.value = ''
}

// ===================== 右键菜单 =====================
const menu = reactive<{ visible: boolean; x: number; y: number; conv: Conversation | null }>({
  visible: false,
  x: 0,
  y: 0,
  conv: null
})

function openMenu(e: MouseEvent, conv: Conversation) {
  // 简易防溢出：贴近右/下边缘时向内收
  const maxX = window.innerWidth - 150
  const maxY = window.innerHeight - 190
  menu.x = Math.min(e.clientX, maxX)
  menu.y = Math.min(e.clientY, maxY)
  menu.conv = conv
  menu.visible = true
}

function closeMenu() {
  menu.visible = false
  menu.conv = null
}

// ===================== 导出 / 删除 =====================
async function exportAs(conv: Conversation, format: string) {
  closeMenu()
  try {
    await chatStore.exportConversationWithDialog(conv.id, format)
  } catch {
    discreteMessage.error('导出失败，请重试')
  }
}

function removeConv(conv: Conversation) {
  closeMenu()
  discreteDialog.warning({
    title: '删除会话',
    content: `确定删除「${conv.title}」吗？此操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => {
      void chatStore.deleteConversation(conv.id)
    }
  })
}
</script>

<style scoped>
/* ===== 常驻左栏：定宽纸面 + 右缘发丝线 =====
 * 展开/收起的节奏设计（对齐 Material/Apple 侧栏惯例）：
 *  - 收起：内容先快速淡出（0.12s）→ 宽度随后收拢（0.28s），避免收拢中
 *    内容被 overflow:hidden 硬裁切造成的"生硬感"
 *  - 展开：宽度先展开（0.28s）→ 内容延迟淡入（0.1s 后 0.2s），
 *    避免展开初期内容从窄缝"挤出来"
 *  - 缓动用强调曲线 cubic-bezier(0.32, 0.72, 0, 1)，收尾比标准曲线更柔和 */
.session-sidebar {
  flex-shrink: 0;
  width: var(--sidebar-width);
  height: 100%;
  overflow: hidden;
  background: var(--surface-panel);
  border-right: 1px solid var(--border-light);
  transition: width 0.28s cubic-bezier(0.32, 0.72, 0, 1);
}

.session-sidebar.collapsed {
  width: 0;
  border-right-color: transparent;
}

.sidebar-inner {
  display: flex;
  flex-direction: column;
  width: var(--sidebar-width);
  height: 100%;
  opacity: 1;
  transform: translateX(0);
  /* 展开态：宽度先展开，内容延迟淡入（delay 与宽度节奏衔接） */
  transition:
    opacity 0.2s ease 0.1s,
    transform 0.28s cubic-bezier(0.32, 0.72, 0, 1);
}

/* 收起态：内容立即快速淡出并顺势左移，宽度随后收拢 */
.session-sidebar.collapsed .sidebar-inner {
  opacity: 0;
  transform: translateX(-20px);
  transition:
    opacity 0.12s ease,
    transform 0.28s cubic-bezier(0.32, 0.72, 0, 1);
}

/* ===== 头部 ===== */
.drawer-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 12px 12px 8px;
}

.drawer-title {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 3px;
  color: var(--text-primary);
}

.head-spacer {
  flex: 1;
}

.head-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.head-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* ===== 搜索行 ===== */
.search-row {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0 12px 10px;
  padding: 0 9px;
  height: 30px;
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-sm);
  background: var(--surface-card);
  transition: border-color var(--transition-fast);
}

.search-row:focus-within {
  border-color: var(--accent-primary);
}

.search-glyph {
  color: var(--text-muted);
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  color: var(--text-primary);
}

.search-input::placeholder {
  color: var(--text-muted);
}

.search-clear {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  flex-shrink: 0;
}

.search-clear:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* ===== 全文搜索入口按钮（标题过滤模式下的扩展提示） ===== */
.fulltext-entry {
  display: flex;
  align-items: center;
  gap: 6px;
  width: calc(100% - 24px);
  margin: 0 12px 10px;
  padding: 6px 10px;
  border: 1px dashed var(--border-color);
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-secondary);
  font-family: inherit;
  font-size: 12px;
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast),
    border-color var(--transition-fast);
}

.fulltext-entry:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
  border-color: var(--accent-primary);
}

/* ===== 全文搜索结果 ===== */
.search-group-label {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.search-group-count {
  font-size: 11px;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.search-hit {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 7px 8px;
  border-radius: var(--border-radius-sm);
  cursor: pointer;
  transition: background var(--transition-fast);
}

.search-hit:hover {
  background: var(--bg-hover);
}

.search-hit-role {
  flex-shrink: 0;
  margin-top: 1px;
  padding: 0 5px;
  height: 16px;
  line-height: 16px;
  font-size: 10px;
  border-radius: 4px;
  background: var(--bg-tertiary);
  color: var(--text-muted);
}

.search-hit-role--user {
  background: rgba(176, 67, 46, 0.12);
  color: var(--seal-color);
}

.search-hit-role--assistant {
  background: rgba(61, 220, 151, 0.14);
  color: #0e9f6e;
}

.search-hit-snippet {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
}

/* ===== 分组滚动区 ===== */
.group-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 8px 12px;
}

.empty-hint {
  margin: 40px 0 0;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
}

/* 小节标：衬线 + 印章点 */
.group-label {
  position: relative;
  margin: 14px 6px 6px;
  padding-left: 11px;
  font-family: var(--font-display);
  font-size: 11px;
  letter-spacing: 2px;
  color: var(--text-muted);
}

.group-label::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 5px;
  height: 5px;
  border-radius: 1px;
  background: var(--seal-color);
  opacity: 0.85;
}

.group-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

/* 条目：横格笔记行 */
.conversation-item {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 1px 0;
  padding: 7px 9px;
  border-radius: var(--border-radius-sm);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    opacity var(--transition-fast);
  animation: item-in 0.35s both;
  animation-delay: calc(var(--stagger-idx, 0) * 24ms);
}

@keyframes item-in {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.conversation-item:hover {
  background: var(--bg-hover);
}

.conversation-item.active {
  background: var(--bg-active);
}

.conversation-item-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  color: var(--text-primary);
}

.item-time {
  flex-shrink: 0;
  font-size: 10.5px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.conversation-item:hover .item-time,
.conversation-item.active .item-time {
  opacity: 1;
}

.rename-input {
  flex: 1;
  min-width: 0;
  padding: 2px 6px;
  border: 1px solid var(--accent-primary);
  border-radius: var(--border-radius-xs);
  outline: none;
  background: var(--surface-card);
  font-family: inherit;
  font-size: 12.5px;
  color: var(--text-primary);
}

/* ===== 底部导航 ===== */
.drawer-foot {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
  border-top: 1px solid var(--border-light);
}

.foot-link {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 32px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  color: var(--text-secondary);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.foot-link:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* ===== 右键菜单（Teleport 到 body，定位不受侧栏裁切影响） ===== */
.ctx-veil {
  position: fixed;
  inset: 0;
  z-index: calc(var(--z-command-bar) + 30);
}

.ctx-menu {
  position: fixed;
  min-width: 140px;
  padding: 5px;
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-md);
  box-shadow: var(--shadow-lg);
}

.ctx-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  text-align: left;
  color: var(--text-primary);
  cursor: pointer;
  transition: background var(--transition-fast);
}

.ctx-item:hover {
  background: var(--bg-hover);
}

.ctx-danger {
  color: var(--accent-danger);
}

.ctx-divider {
  display: block;
  height: 1px;
  margin: 4px 6px;
  background: var(--border-light);
}

@media (prefers-reduced-motion: reduce) {
  .session-sidebar,
  .sidebar-inner,
  .session-sidebar.collapsed .sidebar-inner {
    transition: none;
  }
  .session-sidebar.collapsed .sidebar-inner {
    transform: none;
  }
}
</style>
