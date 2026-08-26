<template>
  <div class="markdown-body" v-html="renderedContent" />
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { renderMarkdown, escapeHtml } from '../../utils/markdown'

const props = defineProps<{ content: string }>()

// remark 是异步的，使用 ref + watch 模式
const renderedContent = ref('')
// 渲染版本号防止异步竞态——若 content 在短时间内多次变化，
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
/* 排版说明：本组件 v-html 注入的内容不带 scoped 属性标记，
 * 组件内 scoped 样式对它无效，因此 .markdown-body 的元素排版规则
 * 全部放在全局 style.css 的 ".markdown-body" 命名空间下
 * （与流式占位符共用同一套，避免生成结束瞬间样式跳变） */
</style>
