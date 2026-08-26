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
  /* 用户消息保留气泡形态——与 AI 回复形成"人说话/机器著文"的形态对比；
   * 冷灰便签底 × 石墨字即当前品牌语言，alpha 驱动自动适配背景图模式 */
  background: var(--bg-user-msg);
  color: var(--text-user-msg);
  /* 用户头像在右侧（row-reverse），右上角小贴近头像侧，暗示来源 */
  border-radius: var(--border-radius-lg) 4px var(--border-radius-lg) var(--border-radius-lg);
  /* 发丝描边：气泡半透明浮于背景图上时提供玻璃边缘感，无背景图时几乎不可见 */
  border: 1px solid var(--border-light);
  /* 我方手记：描边掺入一缕亮蓝印章色，与 AI 回复形成「对话双方」辨识度 */
  border-color: color-mix(in srgb, var(--accent-primary) 22%, var(--border-light));
  box-shadow: none;
}

.ai-bubble {
  width: auto;
  max-width: 100%;
  min-width: 0;
  /* 自适应宽度（与用户气泡同规则）：短回复收成小气泡贴住头像，
   * 长段落撑到容器上限（外层 wrapper 限宽 72%）后换行；
   * panel 阅读层底 + 发丝描边，左上角小圆角贴近左侧 AI 头像 */
  padding: 14px 20px;
  background: var(--bg-ai-msg);
  color: var(--text-ai-msg);
  border-radius: 4px var(--border-radius-lg) var(--border-radius-lg) var(--border-radius-lg);
  border: 1px solid var(--border-light);
  box-shadow: none;
}

.user-text {
  white-space: pre-wrap;
  line-height: 1.7;
  font-size: 15px;
  font-weight: 400;
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
  /* 书房风：附件标签统一为纸面信息件——panel 表面 + hairline 边，
   * 类型差异由图标与文件名表达，不再使用彩色分类底 */
  background: var(--surface-panel);
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
  max-width: 240px;
  overflow: hidden;
  transition:
    border-color 0.2s ease,
    background 0.2s ease;
  border: 1px solid var(--border-light);
}

.attachment-tag:hover {
  /* 悬浮反馈：亮蓝细边落印，不做位移与投影 */
  border-color: var(--accent-primary);
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
  transition: border-color 0.2s ease;
  border: 1px solid var(--border-light);
}

.message-image:hover {
  /* 书房风：悬浮仅细边落亮蓝，不做缩放与投影叠加 */
  border-color: var(--accent-primary);
}

/* —— 编辑态样式 —— */
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
