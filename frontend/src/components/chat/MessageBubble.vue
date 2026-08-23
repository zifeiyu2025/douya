<template>
  <div class="message-bubble" :class="isUser ? 'user-bubble' : 'ai-bubble'">
    <template v-if="isUser">
      <!-- 编辑态：内联 textarea 替换正文展示，Ctrl+Enter 保存 / Esc 取消 -->
      <div v-if="editing" class="edit-area">
        <textarea
          ref="editTextareaRef"
          v-model="draft"
          class="edit-textarea"
          rows="4"
          @keydown.enter.ctrl.prevent="$emit('save', draft)"
          @keydown.esc.prevent="$emit('cancel')"
        />
        <div class="edit-footer">
          <span class="edit-hint">Ctrl+Enter 保存 · Esc 取消</span>
          <n-button size="tiny" type="primary" @click="$emit('save', draft)">
            保存并重新生成
          </n-button>
          <n-button size="tiny" quaternary @click="$emit('cancel')">取消</n-button>
        </div>
      </div>
      <template v-else>
        <div v-if="parsedImages.length > 0" class="message-images">
          <img
            v-for="src in parsedImages"
            :key="src"
            :src="src"
            class="message-image"
            @click="$emit('preview', src)"
          />
        </div>
        <div v-if="nonImageAttachments.length > 0" class="message-attachments">
          <div
            v-for="att in nonImageAttachments"
            :key="att.name"
            class="attachment-tag"
            :class="'att-' + att.type"
          >
            <AppIcon :name="attachmentIcon(att.type)" class="att-icon" :size="14" />
            <span class="att-name">{{ att.name }}</span>
          </div>
        </div>
        <div v-if="message.content" class="user-text">{{ message.content }}</div>
      </template>
    </template>
    <template v-else>
      <ThinkBlock
        v-if="message.thinking_content"
        :content="message.thinking_content"
        :duration="message.thinking_duration"
      />
      <MarkdownBlock :content="message.content" />
      <SearchStatus
        v-if="hasSearchResults"
        :searching="false"
        :results="message.search_results"
        :default-expanded="false"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
import MarkdownBlock from './MarkdownBlock.vue'
import AppIcon from '../ui/AppIcon.vue'
import type { Message, AttachmentSummary } from '../../services/wails'

const props = defineProps<{
  message: Message
  /** 是否处于编辑态（仅用户消息有意义，由父组件编排） */
  editing?: boolean
}>()

defineEmits<{
  save: [text: string]
  cancel: []
  preview: [src: string]
}>()

const ATTACHMENT_ICON_MAP: Record<string, 'audio' | 'video' | 'pdf' | 'file' | 'image'> = {
  audio: 'audio',
  video: 'video',
  pdf: 'pdf',
  text: 'file',
  image: 'image'
}

function attachmentIcon(type: string): 'audio' | 'video' | 'pdf' | 'file' | 'image' {
  return ATTACHMENT_ICON_MAP[type] || 'file'
}

const isUser = computed(() => props.message.role === 'user')

const hasSearchResults = computed(() => {
  if (!props.message.search_results) return false
  if (props.message.search_results === '[]') return false
  return props.message.search_results.length > 0
})

const parsedImages = computed(() => {
  if (!props.message.images) return []
  try {
    const arr = JSON.parse(props.message.images)
    if (Array.isArray(arr)) return arr
  } catch {
    // 忽略 JSON 解析错误，返回空数组
  }
  return []
})

const nonImageAttachments = computed<AttachmentSummary[]>(() => {
  if (!props.message.attachments || !Array.isArray(props.message.attachments)) return []
  return props.message.attachments.filter(a => a.type !== 'image')
})

// 编辑草稿：进入编辑态时以消息当前内容初始化，并自动聚焦输入框
const draft = ref('')
const editTextareaRef = ref<HTMLTextAreaElement>()

watch(
  () => props.editing,
  async val => {
    if (val) {
      draft.value = props.message.content
      await nextTick()
      editTextareaRef.value?.focus()
    }
  },
  { immediate: true }
)
</script>

<style scoped>
/* 以下气泡与用户内容样式自原 MessageItem 逐字迁移（模板归属本组件） */
.message-bubble {
  /* 共享基础样式 */
  box-sizing: border-box;
  position: relative;
  line-height: 1.65;
}

.user-bubble {
  width: auto;
  max-width: 100%;
  min-width: 0;
  padding: 12px 18px;
  background: var(--bg-user-msg);
  color: var(--text-ai-msg);
  /* 用户头像在右侧（row-reverse），右上角小贴近头像侧，暗示来源 */
  border-radius: var(--border-radius-lg) 4px var(--border-radius-lg) var(--border-radius-lg);
  border: none;
  box-shadow: none;
}

.ai-bubble {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding: 14px 20px;
  background: var(--bg-ai-msg);
  color: var(--text-ai-msg);
  /* AI 头像在左侧（默认 row），左上角小贴近头像侧，暗示来源 */
  border-radius: 4px var(--border-radius-lg) var(--border-radius-lg) var(--border-radius-lg);
  border: none;
  box-shadow: none;
}

.user-text {
  white-space: pre-wrap;
  line-height: 1.7;
  font-size: 15px;
  font-weight: 400;
}

/* 背景图模式：气泡半透明浮于背景图之上（三层透明度体系 - 气泡层 80%） */
.has-background .ai-bubble {
  background: color-mix(in srgb, var(--bg-ai-msg) 80%, transparent);
}

.has-background .user-bubble {
  background: color-mix(in srgb, var(--bg-user-msg) 80%, transparent);
}

.message-images {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.message-attachments {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.attachment-tag {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: var(--border-radius-md);
  /* 默认灰色系（file 附件）—— 语义色变量自动适配亮/暗主题 */
  background: var(--accent-n-soft);
  color: var(--accent-n-strong);
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
  max-width: 240px;
  overflow: hidden;
  transition: all 0.2s ease;
  border: 1px solid color-mix(in srgb, var(--accent-n-primary) 25%, transparent);
}

.attachment-tag:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

/* audio 附件 → 紫色系（与 style.css --accent-p-* 设计意图一致） */
.attachment-tag.att-audio {
  background: var(--accent-p-soft);
  color: var(--accent-p-strong);
  border-color: color-mix(in srgb, var(--accent-p-primary) 25%, transparent);
}

/* video 附件 → 绿色系（与 style.css --accent-g-* 设计意图一致） */
.attachment-tag.att-video {
  background: var(--accent-g-soft);
  color: var(--accent-g-strong);
  border-color: color-mix(in srgb, var(--accent-g-primary) 25%, transparent);
}

/* pdf 附件 → 红色系（与 style.css --accent-r-* 设计意图一致） */
.attachment-tag.att-pdf {
  background: var(--accent-r-soft);
  color: var(--accent-r-strong);
  border-color: color-mix(in srgb, var(--accent-r-primary) 25%, transparent);
}

.att-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.att-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-image {
  max-width: 260px;
  max-height: 260px;
  border-radius: var(--border-radius-lg);
  cursor: zoom-in;
  object-fit: cover;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-md);
}

.message-image:hover {
  transform: scale(1.03);
  box-shadow: var(--shadow-lg);
}

/* —— 编辑态样式（C-4 新增）—— */
.edit-area {
  width: 100%;
  min-width: 280px;
}

.edit-textarea {
  box-sizing: border-box;
  width: 100%;
  resize: vertical;
  padding: 10px 14px;
  font-size: 15px;
  line-height: 1.7;
  font-family: inherit;
  color: inherit;
  background: color-mix(in srgb, var(--bg-hover) 60%, transparent);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md);
  outline: none;
  transition: border-color 0.15s ease;
}

.edit-textarea:focus {
  border-color: var(--accent-primary);
}

.edit-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.edit-hint {
  font-size: 12px;
  color: var(--text-muted);
  margin-right: auto;
}
</style>
