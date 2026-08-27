<!--
  DocumentManager: 文档列表区（自 KnowledgeView 拆分）
  版式隐喻：账本式目录 —— 行间 hairline 分隔，每行前置 § 序号，
  解析状态以 5×5px 印章方点表达（解析中=朱砂呼吸 / 已就绪=苔绿 / 解析失败=暗红）。
  对外契约保持不变：无 props、无 emits，依赖 KnowledgeView 的 provide 上下文。
-->
<template>
  <section class="doc-section">
    <!-- 节头细工具栏：§ 编号 + 标题 + 计数 + 上传入口 -->
    <header class="section-bar">
      <span class="section-num bar-no"><span class="sec-mark">§</span> 一</span>
      <h2 class="bar-title">档案目录</h2>
      <span v-if="documents.length > 0" class="bar-count">{{ documents.length }} 卷</span>
      <span class="bar-space" />
      <n-upload
        :show-file-list="false"
        :custom-request="handleFileUpload"
        :multiple="true"
        accept=".txt,.md,.csv,.json,.xml,.html,.pdf,.docx,.yaml,.yml,.toml,.go,.py,.js,.ts,.java,.c,.cpp,.h,.rs,.sh,.rb,.php,.swift,.kt,.sql,.log,.ini,.cfg"
      >
        <n-button text size="small" class="upload-btn" :loading="uploading">
          <template #icon>
            <AppIcon name="attach" :size="14" />
          </template>
          上传文档
        </n-button>
      </n-upload>
    </header>

    <!-- 目录正文：账本式行列表 -->
    <ol v-if="documents.length > 0" class="doc-ledger">
      <li v-for="(doc, idx) in documents" :key="doc.id" class="doc-row">
        <span class="row-no">{{ rowNo(idx) }}</span>
        <span
          class="row-status"
          :class="'is-' + statusOf(doc)"
          :title="statusText(statusOf(doc))"
        />
        <span class="row-name" :title="doc.file_name">{{ doc.file_name }}</span>
        <span class="row-meta">{{ metaOf(doc) }}</span>
        <button
          class="row-del"
          type="button"
          aria-label="删除文档"
          @click="handleDeleteDoc(doc.id)"
        >
          <AppIcon name="close" :size="13" />
        </button>
      </li>
    </ol>

    <!-- 空状态：衬线引言 + 一个克制入口，不放插画 -->
    <div v-else class="doc-empty">
      <p class="empty-lead">书斋尚空。</p>
      <p class="empty-sub">上传第一批文档，为这座档案柜建立可检索的索引。</p>
      <n-upload
        :show-file-list="false"
        :custom-request="handleFileUpload"
        :multiple="true"
        accept=".txt,.md,.csv,.json,.xml,.html,.pdf,.docx,.yaml,.yml,.toml,.go,.py,.js,.ts,.java,.c,.cpp,.h,.rs,.sh,.rb,.php,.swift,.kt,.sql,.log,.ini,.cfg"
      >
        <n-button quaternary size="small" class="empty-action">
          <template #icon>
            <AppIcon name="plus" :size="14" />
          </template>
          归入第一批档案
        </n-button>
      </n-upload>
    </div>
  </section>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { NButton, NUpload } from 'naive-ui'
import AppIcon from '../ui/AppIcon.vue'
import type { DocumentMeta } from '../../types/search'
import { KNOWLEDGE_CONTEXT_KEY, type KnowledgeContext } from './knowledgeContext'

defineOptions({ name: 'DocumentManager' })

const ctx = inject<KnowledgeContext>(KNOWLEDGE_CONTEXT_KEY)
if (!ctx) {
  throw new Error('DocumentManager 必须在 KnowledgeView 内使用（缺少 knowledgeContext provide）')
}
// 本组件仅负责：文档目录展示 / 上传 / 删除（依赖当前活跃知识库 activeKB）
// 注：不再使用 ctx.getFileIcon（emoji 映射与整体风格不符），元信息仍复用 ctx 的格式化方法
const { documents, uploading, handleDeleteDoc, handleFileUpload } = ctx
const { formatFileSize, formatTime } = ctx

/** 目录行序号：§ + 两位零填充（mono 弱化色由样式负责） */
function rowNo(idx: number): string {
  return '§' + String(idx + 1).padStart(2, '0')
}

type DocStatus = 'processing' | 'ready' | 'failed'

/**
 * 从既有字段派生解析状态（后端未提供显式 status 字段）：
 * 优先采信 tags.status（若后端将来写入 failed/error），否则按分块数推断。
 */
function statusOf(doc: DocumentMeta): DocStatus {
  const tag = doc.tags?.['status']?.toLowerCase()
  if (tag === 'failed' || tag === 'error') return 'failed'
  return doc.chunk_count > 0 ? 'ready' : 'processing'
}

/** 印章方点的悬浮提示文案 */
function statusText(status: DocStatus): string {
  if (status === 'ready') return '已就绪'
  if (status === 'failed') return '解析失败'
  return '解析中'
}

/** 元信息串：过滤空段后以间隔点连接 */
function metaOf(doc: DocumentMeta): string {
  return [formatFileSize(doc.file_size), `${doc.chunk_count} 分块`, formatTime(doc.ingested_at)]
    .filter(Boolean)
    .join(' · ')
}
</script>

<style scoped>
.doc-section {
  max-width: 880px;
  margin: 0 auto;
}

/* ===== 节头细工具栏 ===== */
.section-bar {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding-bottom: 10px;
  /* 节界 hairline，作章节界 */
  border-bottom: 1px solid var(--border-light);
}

.bar-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.05em;
  color: var(--text-primary);
  /* 标题是短词，非必要不折行：窄窗挤压时保持单行，过长才由调用方兜底 */
  white-space: nowrap;
}

.bar-count {
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--text-muted);
}

.bar-space {
  flex: 1;
}

.upload-btn {
  align-self: center;
  flex-shrink: 0;
}

/* ===== 目录行：hairline 分隔的账本 ===== */
.doc-ledger {
  list-style: none;
  margin: 0;
  padding: 0;
}

.doc-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 6px 11px 2px;
  border-bottom: 1px solid var(--border-light);
  transition: background var(--transition-fast);
}

/* 行悬浮：纸底一阶色阶变化 */
.doc-row:hover {
  background: var(--bg-hover);
}

/* § 序号：等宽弱化色，定宽对齐 */
.row-no {
  min-width: 34px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-tertiary);
  flex-shrink: 0;
}

/* 5×5px 印章方点 */
.row-status {
  width: 5px;
  height: 5px;
  border-radius: 1px;
  flex-shrink: 0;
}

.row-status.is-ready {
  background: var(--accent-primary);
}

/* 解析中：朱砂 + 轻微呼吸（动效降级由全局 reduced-motion 兜底） */
.row-status.is-processing {
  background: var(--seal-color);
  animation: seal-breathe 1.6s ease-in-out infinite;
}

/* 解析失败：暗红（朱砂压深一档） */
.row-status.is-failed {
  background: color-mix(in srgb, var(--accent-danger) 72%, #000000);
}

@keyframes seal-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}

.row-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
  color: var(--text-primary);
}

.row-meta {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-tertiary);
}

/* 删除钮：悬浮行时才显影 */
.row-del {
  opacity: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
  border: none;
  border-radius: var(--border-radius-xs);
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  flex-shrink: 0;
  transition:
    opacity var(--transition-fast),
    color var(--transition-fast),
    background var(--transition-fast);
}

.doc-row:hover .row-del,
.row-del:focus-visible {
  opacity: 1;
}

.row-del:hover {
  color: var(--accent-danger);
  background: var(--accent-r-soft);
}

/* ===== 空状态：引言式引导 ===== */
.doc-empty {
  padding: 46px 20px 42px;
  text-align: center;
}

.empty-lead {
  margin: 0 0 6px;
  font-family: var(--font-display);
  font-size: 16px;
  letter-spacing: 0.1em;
  color: var(--text-secondary);
}

.empty-sub {
  margin: 0 0 18px;
  font-size: 12.5px;
  color: var(--text-muted);
}

.empty-action {
  color: var(--accent-primary);
}
</style>
