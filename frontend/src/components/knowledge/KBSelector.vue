<!--
  KBSelector: 知识库选择工具栏（自 KnowledgeView 拆分）
  承载：活跃知识库下拉切换 / 新建 / 删除（删除走确认弹窗）。
  对外契约保持不变：无 props、无 emits，依赖 KnowledgeView 的 provide 上下文。
-->
<template>
  <div class="kb-toolbar">
    <!-- 柜名铭牌：等宽小字 + 竖 hairline -->
    <span class="toolbar-label">档案柜</span>
    <span class="toolbar-sep" aria-hidden="true" />

    <n-select
      v-model:value="activeKB"
      :options="kbOptions"
      placeholder="选择知识库"
      class="kb-select"
      size="small"
      :to="false"
      @update:value="handleKBChange"
    />

    <span v-if="knowledgeBases.length > 0" class="toolbar-count">
      {{ knowledgeBases.length }} 个知识库
    </span>

    <div class="toolbar-actions">
      <n-button quaternary size="small" class="toolbar-btn" @click="showCreateKB = true">
        <template #icon>
          <AppIcon name="plus" :size="14" />
        </template>
        新建
      </n-button>
      <n-button
        quaternary
        type="error"
        size="small"
        :disabled="activeKB === 'default'"
        class="toolbar-btn"
        @click="handleDeleteKB"
      >
        <template #icon>
          <AppIcon name="trash" :size="14" />
        </template>
        删除
      </n-button>
    </div>

    <n-modal
      v-model:show="showCreateKB"
      preset="dialog"
      title="新建知识库"
      positive-text="创建"
      negative-text="取消"
      @positive-click="handleCreateKB"
    >
      <n-input v-model:value="newKBName" placeholder="输入知识库名称" />
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { NButton, NInput, NModal, NSelect } from 'naive-ui'
import AppIcon from '../ui/AppIcon.vue'
import { KNOWLEDGE_CONTEXT_KEY, type KnowledgeContext } from './knowledgeContext'

defineOptions({ name: 'KBSelector' })

const ctx = inject<KnowledgeContext>(KNOWLEDGE_CONTEXT_KEY)
if (!ctx) {
  throw new Error('KBSelector 必须在 KnowledgeView 内使用（缺少 knowledgeContext provide）')
}
// 本组件仅负责：知识库下拉切换 / 新建 / 删除
const { knowledgeBases, activeKB, kbOptions, showCreateKB, newKBName } = ctx
const { handleKBChange, handleCreateKB, handleDeleteKB } = ctx
</script>

<style scoped>
/* ===== 细工具栏：panel 表面 + hairline 底边 ===== */
.kb-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 32px;
  background: var(--surface-panel);
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
}

.toolbar-label {
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.18em;
  color: var(--text-tertiary);
  flex-shrink: 0;
}

/* 铭牌后的竖 hairline */
.toolbar-sep {
  width: 1px;
  height: 14px;
  background: var(--border-light);
  flex-shrink: 0;
}

.kb-select {
  width: min(300px, 34vw);
}

.toolbar-count {
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--text-muted);
  white-space: nowrap;
}

.toolbar-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 4px;
}

.toolbar-btn {
  flex-shrink: 0;
}

/* ===== 下拉浮层：panel 表面 + hairline 行分隔 ===== */
/* :to="false" 使浮层就地渲染，scoped 深度选择器方可命中 */
.kb-select :deep(.n-base-select-menu) {
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-sm);
  box-shadow: var(--shadow-md); /* 全页唯一一层投影 */
}

.kb-select :deep(.n-base-select-option) {
  border-bottom: 1px solid var(--border-light);
}

.kb-select :deep(.n-base-select-option:last-child) {
  border-bottom: none;
}
</style>
