<template>
  <div class="knowledge-container">
    <div class="knowledge-header">
      <n-button quaternary circle @click="$router.push('/')" class="back-btn">
        <template #icon>
          <n-icon size="20"><ArrowBackOutline /></n-icon>
        </template>
      </n-button>
      <div class="header-title-group">
        <span class="knowledge-title">知识库管理</span>
        <span class="knowledge-subtitle">管理文档和检索设置</span>
      </div>
    </div>

    <div class="knowledge-body">
      <div class="kb-card">
        <div class="card-title-row">
          <span class="card-icon">📚</span>
          <span class="card-title">知识库</span>
          <span class="kb-count" v-if="knowledgeBases.length > 0">{{ knowledgeBases.length }} 个知识库</span>
        </div>

        <div class="kb-selector-row">
          <n-select
            v-model:value="activeKB"
            :options="kbOptions"
            placeholder="选择知识库"
            class="kb-select"
            @update:value="handleKBChange"
          />
          <n-button quaternary size="small" @click="showCreateKB = true" class="action-btn">
            <template #icon><n-icon size="16"><AddOutline /></n-icon></template>
            新建
          </n-button>
          <n-button quaternary type="error" size="small" @click="handleDeleteKB" :disabled="activeKB === 'default'" class="action-btn">
            <template #icon><n-icon size="16"><TrashOutline /></n-icon></template>
            删除
          </n-button>
        </div>

        <n-modal v-model:show="showCreateKB" preset="dialog" title="新建知识库" positive-text="创建" negative-text="取消" @positive-click="handleCreateKB">
          <n-input v-model:value="newKBName" placeholder="输入知识库名称" />
        </n-modal>
      </div>

      <div class="kb-card">
        <div class="card-title-row">
          <span class="card-icon">📄</span>
          <span class="card-title">文档列表</span>
          <span class="kb-count" v-if="documents.length > 0">{{ documents.length }} 个文档</span>
        </div>

        <div class="doc-list" v-if="documents.length > 0">
          <div class="doc-item" v-for="doc in documents" :key="doc.id">
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
            <n-button quaternary type="error" size="tiny" @click="handleDeleteDoc(doc.id)" class="doc-delete-btn">
              <template #icon><n-icon size="15"><CloseOutline /></n-icon></template>
            </n-button>
          </div>
        </div>
        <div class="doc-empty" v-else>
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
              <template #icon><n-icon size="16"><CloudUploadOutline /></n-icon></template>
              上传文档
            </n-button>
          </n-upload>
        </div>
      </div>

      <div class="kb-card">
        <div class="card-title-row">
          <span class="card-icon">⚙️</span>
          <span class="card-title">RAG 设置</span>
        </div>

        <n-form label-placement="left" label-width="96" :model="ragConfig" class="rag-form">
          <n-form-item label="启用知识库">
            <n-switch v-model:value="ragConfig.enabled" @update:value="handleRAGToggle" />
          </n-form-item>
          <n-form-item label="检索数量">
            <div class="slider-wrap">
              <n-slider v-model:value="ragConfig.topK" :min="1" :max="10" :step="1" />
              <span class="slider-mark">{{ ragConfig.topK }}</span>
            </div>
          </n-form-item>
          <n-form-item label="最低相关性">
            <div class="slider-wrap">
              <n-slider v-model:value="ragConfig.minScore" :min="0" :max="1" :step="0.05" />
              <span class="slider-mark">{{ ragConfig.minScore.toFixed(2) }}</span>
            </div>
          </n-form-item>
          <n-form-item label="分块大小">
            <n-input-number v-model:value="ragConfig.chunkSize" :min="128" :max="4096" :step="64" size="small" class="rag-input" />
          </n-form-item>
          <n-form-item label="分块重叠">
            <n-input-number v-model:value="ragConfig.chunkOverlap" :min="0" :max="512" :step="16" size="small" class="rag-input" />
          </n-form-item>
        </n-form>

        <div class="rag-save-row">
          <n-button type="primary" size="small" @click="handleSaveRAGConfig" :loading="savingRAG" class="save-btn">
            <template #icon><n-icon size="16"><CheckmarkCircleOutline /></n-icon></template>
            保存设置
          </n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  NButton, NIcon, NSelect, NUpload, NModal,
  NInput, NForm, NFormItem, NSwitch, NSlider, NInputNumber,
  useMessage,
} from 'naive-ui'
import { ArrowBackOutline, AddOutline, TrashOutline, CloseOutline, CloudUploadOutline, CheckmarkCircleOutline } from '@vicons/ionicons5'
import { wails, type CollectionInfo, type DocumentMeta } from '../services/wails'

const message = useMessage()

const knowledgeBases = ref<CollectionInfo[]>([])
const activeKB = ref('default')
const documents = ref<DocumentMeta[]>([])
const uploading = ref(false)
const savingRAG = ref(false)
const showCreateKB = ref(false)
const newKBName = ref('')

const ragConfig = ref({
  enabled: false,
  topK: 3,
  minScore: 0.3,
  chunkSize: 512,
  chunkOverlap: 64,
})

const kbOptions = computed(() =>
  knowledgeBases.value.map(kb => ({
    label: `${kb.name} (${kb.vector_count} 向量)`,
    value: kb.name,
  }))
)

async function loadData() {
  try {
    const [kbs, active, enabled, config] = await Promise.all([
      wails.listKnowledgeBases(),
      wails.getActiveKnowledgeBase(),
      wails.isRAGEnabled(),
      wails.getConfig(),
    ])
    knowledgeBases.value = kbs
    activeKB.value = active || 'default'
    ragConfig.value.enabled = enabled
    if (config) {
      ragConfig.value.topK = config.rag_top_k ?? 3
      ragConfig.value.minScore = config.rag_min_score ?? 0.3
      ragConfig.value.chunkSize = config.rag_chunk_size ?? 512
      ragConfig.value.chunkOverlap = config.rag_chunk_overlap ?? 64
    }
    await loadDocuments()
  } catch (e: any) {
    message.error('加载知识库数据失败: ' + (e.message || e))
  }
}

async function loadDocuments() {
  try {
    documents.value = await wails.listDocuments(activeKB.value)
  } catch (e: any) {
    documents.value = []
  }
}

async function handleKBChange(name: string) {
  try {
    await wails.setActiveKnowledgeBase(name)
    activeKB.value = name
    await loadDocuments()
  } catch (e: any) {
    message.error('切换知识库失败: ' + (e.message || e))
  }
}

async function handleCreateKB() {
  if (!newKBName.value.trim()) {
    message.warning('请输入知识库名称')
    return
  }
  try {
    await wails.createKnowledgeBase(newKBName.value.trim())
    message.success('知识库创建成功')
    newKBName.value = ''
    await loadData()
  } catch (e: any) {
    message.error('创建失败: ' + (e.message || e))
  }
}

async function handleDeleteKB() {
  try {
    await wails.deleteKnowledgeBase(activeKB.value)
    message.success('知识库已删除')
    activeKB.value = 'default'
    await loadData()
  } catch (e: any) {
    message.error('删除失败: ' + (e.message || e))
  }
}

async function handleDeleteDoc(docID: string) {
  try {
    await wails.deleteDocument(activeKB.value, docID)
    message.success('文档已删除')
    await loadDocuments()
  } catch (e: any) {
    message.error('删除文档失败: ' + (e.message || e))
  }
}

async function handleFileUpload({ file }: any) {
  uploading.value = true
  try {
    const f = file.file as File
    const arrayBuffer = await f.arrayBuffer()
    const bytes = new Uint8Array(arrayBuffer)
    let binary = ''
    const chunkSize = 8192
    for (let i = 0; i < bytes.length; i += chunkSize) {
      const slice = bytes.subarray(i, i + chunkSize)
      binary += String.fromCharCode.apply(null, Array.from(slice))
    }
    const base64 = btoa(binary)
    await wails.uploadDocument(activeKB.value, f.name, base64, f.type || 'application/octet-stream')
    message.success(`${f.name} 上传成功`)
    await loadDocuments()
  } catch (e: any) {
    message.error('上传失败: ' + (e.message || e))
  } finally {
    uploading.value = false
  }
}

async function handleRAGToggle(enabled: boolean) {
  try {
    await wails.setRAGEnabled(enabled)
    ragConfig.value.enabled = enabled
  } catch (e: any) {
    message.error('切换 RAG 失败: ' + (e.message || e))
  }
}

async function handleSaveRAGConfig() {
  savingRAG.value = true
  try {
    const config = await wails.getConfig() as any
    config.rag_top_k = ragConfig.value.topK
    config.rag_min_score = ragConfig.value.minScore
    config.rag_chunk_size = ragConfig.value.chunkSize
    config.rag_chunk_overlap = ragConfig.value.chunkOverlap
    await wails.updateConfig(config)
    message.success('RAG 设置已保存')
  } catch (e: any) {
    message.error('保存失败: ' + (e.message || e))
  } finally {
    savingRAG.value = false
  }
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function getFileIcon(fileName: string): string {
  const ext = fileName.split('.').pop()?.toLowerCase() || ''
  const iconMap: Record<string, string> = {
    pdf: '📕', doc: '📘', docx: '📘', md: '📝', txt: '📃',
    csv: '📊', json: '⚙️', xml: '📋', html: '🌐', yaml: '📑', yml: '📑', toml: '📑',
    go: '🔷', py: '🐍', js: '💛', ts: '💙', java: '☕', c: '🔧', cpp: '🔧',
    h: '📌', rs: '🦀', sh: '💻', rb: '💎', php: '🐘', swift: '🍎', kt: '🟣',
    sql: '🗄️', log: '📜', ini: '📑', cfg: '📑',
  }
  return iconMap[ext] || '📄'
}

function formatTime(ts: string): string {
  if (!ts) return ''
  try {
    const d = new Date(ts)
    return d.toLocaleString('zh-CN')
  } catch {
    return ts
  }
}

onMounted(loadData)
</script>

<style scoped>
.knowledge-container {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
}

.knowledge-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 20px;
  background: var(--bg-primary);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  height: var(--header-height);
  box-sizing: border-box;
}

.back-btn {
  flex-shrink: 0;
}

.header-title-group {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.knowledge-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.2px;
}

.knowledge-subtitle {
  font-size: 12px;
  color: var(--text-muted);
  font-weight: 400;
}

.knowledge-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 20px 20px 32px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.kb-card {
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-lg);
  padding: 20px;
  flex-shrink: 0;
  transition: border-color var(--transition-fast);
}

.kb-card:hover {
  border-color: var(--accent-primary);
}

.card-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.card-icon {
  font-size: 18px;
  line-height: 1;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

.kb-count {
  font-size: 12px;
  color: var(--text-muted);
  margin-left: auto;
  padding: 2px 8px;
  background: var(--bg-tertiary);
  border-radius: 10px;
}

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
  padding: 12px 14px;
  border-radius: var(--border-radius);
  background: var(--bg-tertiary);
  border: 1px solid transparent;
  transition: all var(--transition-fast);
}

.doc-item:hover {
  border-color: var(--border-color);
  background: var(--bg-hover);
}

.doc-icon-box {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
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

.doc-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px 20px;
  border: 2px dashed var(--border-color);
  border-radius: var(--border-radius);
  margin-bottom: 14px;
  background: var(--bg-tertiary);
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

.rag-form {
  margin-bottom: 4px;
}

.slider-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
}

.slider-wrap .n-slider {
  flex: 1;
}

.slider-mark {
  min-width: 36px;
  text-align: center;
  font-size: 13px;
  font-weight: 600;
  color: var(--accent-primary);
  background: var(--accent-tertiary);
  padding: 2px 8px;
  border-radius: 8px;
}

.rag-input {
  width: 120px;
}

.rag-save-row {
  display: flex;
  justify-content: flex-end;
  padding-top: 4px;
  border-top: 1px solid var(--border-light);
}

.save-btn {
  font-weight: 600;
}
</style>
