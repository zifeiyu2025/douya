<template>
  <div class="attachment-preview-bar">
    <div
      v-for="(att, idx) in attachments"
      :key="att.name"
      class="attachment-preview-item"
      :class="att.type"
    >
      <div v-if="att.type === 'image'" class="att-thumb image-thumb">
        <img :src="att.data" alt="" />
      </div>
      <div v-else class="att-thumb file-thumb">
        <svg
          v-if="att.type === 'audio'"
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M9 18V5l12-2v13" />
          <circle cx="6" cy="18" r="3" />
          <circle cx="18" cy="16" r="3" />
        </svg>
        <svg
          v-else-if="att.type === 'video'"
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polygon points="23 7 16 12 23 17 23 7" />
          <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
        </svg>
        <svg
          v-else-if="att.type === 'pdf'"
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
          <line x1="16" y1="13" x2="8" y2="13" />
          <line x1="16" y1="17" x2="8" y2="17" />
          <polyline points="10 9 9 9 8 9" />
        </svg>
        <svg
          v-else
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
          <line x1="16" y1="13" x2="8" y2="13" />
          <line x1="16" y1="17" x2="8" y2="17" />
        </svg>
      </div>
      <div class="att-info">
        <span class="att-name" :title="att.name">{{ att.name }}</span>
        <span class="att-type-label">{{ typeLabel(att.type) }}</span>
      </div>
      <button class="remove-att-btn" @click="emit('remove', idx)">
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Attachment } from '../../services/wails'

// 附件预览栏：展示已选附件的缩略图、文件名、类型与删除按钮
// 通过 props 接收附件列表，删除操作通过 emit 交给父组件处理
defineProps<{
  attachments: Attachment[]
}>()

const emit = defineEmits<{
  remove: [index: number]
}>()

function typeLabel(type: string): string {
  switch (type) {
    case 'image':
      return '图片'
    case 'audio':
      return '音频'
    case 'text':
      return '文本'
    case 'video':
      return '视频'
    case 'pdf':
      return 'PDF'
    case 'docx':
      return 'DOCX'
    default:
      return type
  }
}
</script>

<style scoped>
.attachment-preview-bar {
  display: flex;
  gap: 10px;
  padding: 8px 12px 6px;
  flex-wrap: wrap;
}

.attachment-preview-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: var(--border-radius-sm);
  /* 书房风缩略卡：panel 纸面 + hairline 细边 */
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  max-width: 220px;
  transition:
    border-color 0.2s,
    box-shadow 0.2s;
}

/* hover：苔绿细线落笔提示可交互，配单层低透投影 */
.attachment-preview-item:hover {
  border-color: var(--accent-primary);
  box-shadow: var(--shadow-sm);
}

.attachment-preview-item.image {
  padding: 4px;
  max-width: none;
}

.att-thumb {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.att-thumb.image-thumb {
  width: 64px;
  height: 64px;
  border-radius: var(--border-radius-sm);
  overflow: hidden;
}

.att-thumb.image-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.att-thumb.file-thumb {
  width: 40px;
  height: 40px;
  border-radius: var(--border-radius-sm);
  /* hairline 细边替代色块底，更利落 */
  border: 1px solid var(--border-light);
  background: transparent;
  color: var(--text-secondary);
}

.att-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 3px;
}

.att-name {
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 130px;
  font-weight: 500;
}

.att-type-label {
  font-size: 11px;
  color: var(--text-muted);
}

.remove-att-btn {
  position: absolute;
  top: -6px;
  right: -6px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  /* 书房风：纸面小钮 + 朱砂细边，悬浮才落朱砂底，不做常驻色块 */
  background: var(--surface-panel);
  color: var(--accent-danger);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 45%, transparent);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  line-height: 1;
  opacity: 0;
  box-shadow: var(--shadow-sm);
  transition:
    opacity 0.2s,
    background-color 0.2s,
    color 0.2s;
}

.attachment-preview-item:hover .remove-att-btn {
  opacity: 1;
}

.remove-att-btn:hover {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
  /* 强调色底上的字色走纸面底色令牌 */
  color: var(--bg-primary);
}
</style>
