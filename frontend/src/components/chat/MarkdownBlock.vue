<template>
  <div class="markdown-body" v-html="renderedContent" />
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { renderMarkdown, escapeHtml } from '../../utils/markdown'

const props = defineProps<{ content: string }>()

// remark 是异步的，使用 ref + watch 模式
const renderedContent = ref('')
// L-7：渲染版本号防止异步竞态——若 content 在短时间内多次变化，
// 先发起的渲染任务可能后完成并覆盖最新内容。版本号校验确保只采用最新结果。
let renderVersion = 0

watch(
  () => props.content,
  async newContent => {
    if (!newContent) {
      renderedContent.value = ''
      return
    }
    const version = ++renderVersion
    try {
      const html = await renderMarkdown(newContent)
      // 版本号不匹配说明期间有更新的渲染任务发起，丢弃本次过期结果
      if (version !== renderVersion) return
      renderedContent.value = html
    } catch (_) {
      if (version !== renderVersion) return
      // 渲染失败时转义后作为纯文本显示，避免直接赋值原始未消毒内容到 v-html（XSS 防护）
      renderedContent.value = escapeHtml(newContent)
    }
  },
  { immediate: true }
)
</script>

<style scoped>
/* 以下样式自原 MessageItem 的 ".message-bubble :deep(.markdown-body)" 系列逐字迁移，
   分解后 .markdown-body 由本组件直接持有，无需再穿透 */
.markdown-body blockquote {
  border-left: 4px solid var(--accent-primary);
  padding-left: 18px;
  margin: 16px 0;
  color: var(--text-secondary);
  background: color-mix(in srgb, var(--accent-primary) 5%, transparent);
  padding-top: 12px;
  padding-bottom: 12px;
  padding-right: 16px;
  border-radius: 0 var(--border-radius-md) var(--border-radius-md) 0;
}

.markdown-body table {
  border-collapse: collapse;
  width: 100%;
  margin: 16px 0;
  font-size: 14.5px;
  border-radius: var(--border-radius-md);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}

.markdown-body th,
.markdown-body td {
  border: 1px solid var(--border-color);
  padding: 14px 18px;
  text-align: left;
}

.markdown-body th {
  background: var(--bg-hover);
  font-weight: 600;
}

.markdown-body ul,
.markdown-body ol {
  padding-left: 28px;
  margin: 14px 0;
}

.markdown-body li {
  margin: 10px 0;
  line-height: 1.7;
}

.markdown-body img {
  max-width: 100%;
  border-radius: var(--border-radius-md);
  margin: 14px 0;
  box-shadow: var(--shadow-sm);
}
</style>
