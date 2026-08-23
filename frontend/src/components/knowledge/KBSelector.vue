<!--
  KBSelector: 知识库选择区（C-6 自 KnowledgeView 拆分）
  下拉切换活跃知识库，支持新建与删除；删除走确认弹窗。
-->
<template>
  <div class="kb-section">
    <div class="kb-section-header">
      <span class="section-icon">📚</span>
      <span class="section-title">知识库</span>
      <span v-if="knowledgeBases.length > 0" class="kb-count">
        {{ knowledgeBases.length }} 个知识库
      </span>
    </div>

    <div class="kb-selector-row">
      <n-select
        v-model:value="activeKB"
        :options="kbOptions"
        placeholder="选择知识库"
        class="kb-select"
        @update:value="handleKBChange"
      />
      <n-button quaternary size="small" class="action-btn" @click="showCreateKB = true">
        <template #icon>
          <n-icon size="16"><AddOutline /></n-icon>
        </template>
        新建
      </n-button>
      <n-button
        quaternary
        type="error"
        size="small"
        :disabled="activeKB === 'default'"
        class="action-btn"
        @click="handleDeleteKB"
      >
        <template #icon>
          <n-icon size="16"><TrashOutline /></n-icon>
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
import { NButton, NIcon, NInput, NModal, NSelect } from 'naive-ui'
import { AddOutline, TrashOutline } from '@vicons/ionicons5'
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
.kb-selector-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.kb-select {
  flex: 1;
}

.action-btn {
  flex-shrink: 0;
  font-weight: 500;
}
</style>
