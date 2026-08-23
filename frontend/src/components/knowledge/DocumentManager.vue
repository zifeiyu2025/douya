<!--
  DocumentManager: 文档管理区（C-6 自 KnowledgeView 拆分）
  展示当前知识库的文档列表，支持多文件上传与删除。
-->
<template>
  <div class="kb-section">
    <div class="kb-section-header">
      <span class="section-icon">📄</span>
      <span class="section-title">文档列表</span>
      <span v-if="documents.length > 0" class="kb-count">{{ documents.length }} 个文档</span>
    </div>

    <div v-if="documents.length > 0" class="doc-list">
      <div v-for="doc in documents" :key="doc.id" class="doc-item">
        <div class="doc-icon-box">
          <span class="doc-icon-text">{{ getFileIcon(doc.file_name) }}</span>
        </div>
        <div class="doc-info">
          <div class="doc-details">
            <span class="doc-name">{{ doc.file_name }}</span>
            <span class="doc-meta">
              <span class="meta-tag">{{ formatFileSize(doc.file_size) }}</span>
              <span class="meta-sep">·</span>
              <span class="meta-tag">{{ doc.chunk_count }} 分块</span>
              <span class="meta-sep">·</span>
              <span class="meta-tag">{{ formatTime(doc.ingested_at) }}</span>
            </span>
          </div>
        </div>
        <n-button
          quaternary
          type="error"
          size="tiny"
          class="doc-delete-btn"
          @click="handleDeleteDoc(doc.id)"
        >
          <template #icon>
            <n-icon size="15"><CloseOutline /></n-icon>
          </template>
        </n-button>
      </div>
    </div>
    <div v-else class="doc-empty">
      <div class="empty-icon">📁</div>
      <p class="empty-text">暂无文档</p>
      <p class="empty-hint">上传文档以构建知识库索引</p>
    </div>

    <div class="doc-upload-row">
      <n-upload
        :show-file-list="false"
        :custom-request="handleFileUpload"
        :multiple="true"
        accept=".txt,.md,.csv,.json,.xml,.html,.pdf,.docx,.yaml,.yml,.toml,.go,.py,.js,.ts,.java,.c,.cpp,.h,.rs,.sh,.rb,.php,.swift,.kt,.sql,.log,.ini,.cfg"
      >
        <n-button type="primary" size="small" :loading="uploading" ghost>
          <template #icon>
            <n-icon size="16"><CloudUploadOutline /></n-icon>
          </template>
          上传文档
        </n-button>
      </n-upload>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { NButton, NIcon, NUpload } from 'naive-ui'
import { CloseOutline, CloudUploadOutline } from '@vicons/ionicons5'
import { KNOWLEDGE_CONTEXT_KEY, type KnowledgeContext } from './knowledgeContext'

defineOptions({ name: 'DocumentManager' })

const ctx = inject<KnowledgeContext>(KNOWLEDGE_CONTEXT_KEY)
if (!ctx) {
  throw new Error('DocumentManager 必须在 KnowledgeView 内使用（缺少 knowledgeContext provide）')
}
// 本组件仅负责：文档列表展示 / 上传 / 删除（上传删除依赖当前活跃知识库 activeKB）
const { documents, uploading, handleDeleteDoc, handleFileUpload } = ctx
const { getFileIcon, formatFileSize, formatTime } = ctx
</script>

<style scoped>
.doc-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
}

.doc-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-radius: var(--border-radius-md);
  transition: background var(--transition-fast);
}

/* 文档列表项 hover → --bg-hover */
.doc-item:hover {
  background: var(--bg-hover);
}

/* 文档图标背景 → --accent-p-soft（紫色系，与附件标签色协调） */
.doc-icon-box {
  width: 36px;
  height: 36px;
  border-radius: var(--border-radius-md);
  background: var(--accent-p-soft);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.doc-icon-text {
  font-size: 18px;
  line-height: 1;
}

.doc-info {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
}

.doc-details {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.doc-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.doc-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.meta-tag {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
}

.meta-sep {
  font-size: 11px;
  color: var(--text-tertiary);
}

.doc-delete-btn {
  opacity: 0;
  transition: opacity var(--transition-fast);
  flex-shrink: 0;
}

.doc-item:hover .doc-delete-btn {
  opacity: 1;
}

/* 上传区域（空状态）边框 → --border-color */
.doc-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 36px 20px;
  border: 2px dashed var(--border-color);
  border-radius: var(--border-radius-lg);
  margin-bottom: 14px;
}

.empty-icon {
  font-size: 32px;
  margin-bottom: 8px;
  opacity: 0.5;
}

.empty-text {
  font-size: 14px;
  color: var(--text-secondary);
  margin: 0 0 4px 0;
}

.empty-hint {
  font-size: 12px;
  color: var(--text-muted);
  margin: 0;
}

.doc-upload-row {
  display: flex;
  justify-content: center;
}
</style>
