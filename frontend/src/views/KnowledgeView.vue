<template>
  <div class="knowledge-container">
    <div class="knowledge-header">
      <button class="back-btn" type="button" aria-label="返回" @click="$router.push('/')">
        <svg width="20" height="20" viewBox="0 0 512 512" fill="none" aria-hidden="true">
          <path
            d="M244 400L100 256l144-144M120 256h292"
            stroke="currentColor"
            stroke-width="48"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
      <div class="header-title-group">
        <span class="knowledge-title">知识库管理</span>
        <span class="knowledge-subtitle">管理文档和检索设置</span>
      </div>
    </div>

    <div class="knowledge-body">
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

      <div class="kb-section">
        <div class="kb-section-header">
          <span class="section-icon">⚙️</span>
          <span class="section-title">RAG 设置</span>
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
            <n-input-number
              v-model:value="ragConfig.chunkSize"
              :min="128"
              :max="4096"
              :step="64"
              size="small"
              class="rag-input"
            />
          </n-form-item>
          <n-form-item label="分块重叠">
            <n-input-number
              v-model:value="ragConfig.chunkOverlap"
              :min="0"
              :max="512"
              :step="16"
              size="small"
              class="rag-input"
            />
          </n-form-item>
          <n-form-item label="嵌入模型">
            <div class="embedding-config">
              <n-select
                v-model:value="selectedEmbeddingModel"
                :options="embeddingModelOptions"
                placeholder="从列表选择嵌入模型"
                size="small"
                clearable
                class="embedding-select"
                @update:value="onEmbeddingModelSelect"
              />
              <n-input
                v-model:value="manualEmbeddingModel"
                placeholder="或手动输入模型名称"
                size="small"
                class="embedding-input"
                @update:value="onManualEmbeddingModelInput"
              />
              <div class="embedding-help">
                <div class="embedding-status">
                  <span v-if="ragConfig.embeddingModel" class="status-active">
                    ✓ 使用专用嵌入模型：{{ ragConfig.embeddingModel }}
                  </span>
                  <span v-else class="status-fallback">
                    ⚠ 未配置专用嵌入模型，将使用聊天模型（检索质量可能较差）
                  </span>
                </div>
                <div class="embedding-recommend">
                  <span class="recommend-label">推荐模型：</span>
                  <span class="recommend-tags">
                    <span class="recommend-tag" @click="selectRecommendedModel('nomic-embed-text')">
                      nomic-embed-text
                    </span>
                    <span class="recommend-tag" @click="selectRecommendedModel('bge-base-en-v1.5')">
                      bge-base-en-v1.5
                    </span>
                    <span
                      class="recommend-tag"
                      @click="selectRecommendedModel('bge-large-zh-v1.5')"
                    >
                      bge-large-zh-v1.5
                    </span>
                  </span>
                </div>
                <div class="embedding-tip">
                  💡 专用嵌入模型更快、更准确。留空则使用当前聊天模型。
                </div>
              </div>
            </div>
          </n-form-item>
        </n-form>

        <div class="rag-save-row">
          <n-button
            type="primary"
            size="small"
            :loading="savingRAG"
            class="save-btn"
            @click="handleSaveRAGConfig"
          >
            <template #icon>
              <n-icon size="16"><CheckmarkCircleOutline /></n-icon>
            </template>
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
  NButton,
  NIcon,
  NSelect,
  NUpload,
  NModal,
  NInput,
  NForm,
  NFormItem,
  NSwitch,
  NSlider,
  NInputNumber,
  useMessage,
  useDialog,
  type UploadCustomRequestOptions
} from 'naive-ui'
import {
  AddOutline,
  TrashOutline,
  CloseOutline,
  CloudUploadOutline,
  CheckmarkCircleOutline
} from '@vicons/ionicons5'
import { wails, type CollectionInfo, type DocumentMeta, type ModelOption } from '../services/wails'
import { useSettingsStore } from '../stores/settings'
import { showError, showSuccess, showWarning } from '../utils/showError'
import { logError } from '../utils/logger'

const message = useMessage()
const dialog = useDialog()
const settingsStore = useSettingsStore()

const knowledgeBases = ref<CollectionInfo[]>([])
const activeKB = ref('default')
const documents = ref<DocumentMeta[]>([])
const uploading = ref(false)
const savingRAG = ref(false)
const showCreateKB = ref(false)
const newKBName = ref('')

// 嵌入模型相关
const availableModels = ref<ModelOption[]>([])
const selectedEmbeddingModel = ref<string | null>(null) // 下拉选择的模型
const manualEmbeddingModel = ref('') // 手动输入的模型

const ragConfig = ref({
  enabled: false,
  topK: 3,
  minScore: 0.3,
  chunkSize: 512,
  chunkOverlap: 64,
  embeddingModel: ''
})

const kbOptions = computed(() =>
  knowledgeBases.value.map(kb => ({
    label: `${kb.name} (${kb.vector_count} 向量)`,
    value: kb.name
  }))
)

// 嵌入模型下拉选项
const embeddingModelOptions = computed(() =>
  availableModels.value.map(m => ({
    label: m.name,
    value: m.name
  }))
)

// 加载可用模型列表
async function loadModels() {
  try {
    availableModels.value = await wails.getAvailableModels()
  } catch (e) {
    logError('Failed to load models:', e)
  }
}

// 下拉选择变化时
function onEmbeddingModelSelect(value: string | null) {
  selectedEmbeddingModel.value = value
  if (value) {
    // 选择了下拉，清空手动输入
    manualEmbeddingModel.value = ''
    ragConfig.value.embeddingModel = value
  } else {
    ragConfig.value.embeddingModel = manualEmbeddingModel.value
  }
}

// 手动输入变化时
function onManualEmbeddingModelInput(value: string) {
  manualEmbeddingModel.value = value
  if (value) {
    // 手动输入时，清空下拉选择
    selectedEmbeddingModel.value = null
    ragConfig.value.embeddingModel = value
  } else {
    ragConfig.value.embeddingModel = selectedEmbeddingModel.value || ''
  }
}

// 点击推荐模型标签
function selectRecommendedModel(modelName: string) {
  manualEmbeddingModel.value = modelName
  selectedEmbeddingModel.value = null
  ragConfig.value.embeddingModel = modelName
}

async function loadData() {
  try {
    const [kbs, active, enabled, config] = await Promise.all([
      wails.listKnowledgeBases(),
      wails.getActiveKnowledgeBase(),
      wails.isRAGEnabled(),
      wails.getConfig()
    ])
    knowledgeBases.value = kbs
    activeKB.value = active || 'default'
    ragConfig.value.enabled = enabled
    if (config) {
      ragConfig.value.topK = config.rag_top_k ?? 3
      ragConfig.value.minScore = config.rag_min_score ?? 0.3
      ragConfig.value.chunkSize = config.rag_chunk_size ?? 512
      ragConfig.value.chunkOverlap = config.rag_chunk_overlap ?? 64
      ragConfig.value.embeddingModel = config.embedding_model ?? ''

      // 初始化嵌入模型状态
      const embeddingModel = config.embedding_model ?? ''
      if (embeddingModel) {
        // 检查是否在可用模型列表中
        const isInList = availableModels.value.some(m => m.name === embeddingModel)
        if (isInList) {
          selectedEmbeddingModel.value = embeddingModel
          manualEmbeddingModel.value = ''
        } else {
          selectedEmbeddingModel.value = null
          manualEmbeddingModel.value = embeddingModel
        }
      } else {
        selectedEmbeddingModel.value = null
        manualEmbeddingModel.value = ''
      }
    }
    await loadDocuments()
  } catch (e: unknown) {
    showError(message, '加载知识库数据失败', e)
  }
}

async function loadDocuments() {
  try {
    documents.value = await wails.listDocuments(activeKB.value)
  } catch {
    documents.value = []
  }
}

async function handleKBChange(name: string) {
  try {
    await wails.setActiveKnowledgeBase(name)
    activeKB.value = name
    await loadDocuments()
  } catch (e: unknown) {
    showError(message, '切换知识库失败', e)
  }
}

async function handleCreateKB() {
  if (!newKBName.value.trim()) {
    showWarning(message, '请输入知识库名称')
    return
  }
  try {
    await wails.createKnowledgeBase(newKBName.value.trim())
    showSuccess(message, '知识库创建成功')
    newKBName.value = ''
    await loadData()
  } catch (e: unknown) {
    showError(message, '创建失败', e)
  }
}

async function handleDeleteKB() {
  dialog.warning({
    title: '删除知识库',
    content: `确定要删除知识库「${activeKB.value}」吗？此操作不可撤销，所有文档和向量数据将被永久删除。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await wails.deleteKnowledgeBase(activeKB.value)
        showSuccess(message, '知识库已删除')
        activeKB.value = 'default'
        await loadData()
      } catch (e: unknown) {
        showError(message, '删除失败', e)
      }
    }
  })
}

async function handleDeleteDoc(docID: string) {
  try {
    await wails.deleteDocument(activeKB.value, docID)
    showSuccess(message, '文档已删除')
    await loadDocuments()
  } catch (e: unknown) {
    showError(message, '删除文档失败', e)
  }
}

async function handleFileUpload({ file }: UploadCustomRequestOptions) {
  uploading.value = true
  try {
    const f = file.file as File
    if (f.size > 200 * 1024 * 1024) {
      message.destroyAll()
      message.error('文件大小不能超过 200MB')
      return
    }
    // 安全修复（S3）：原实现 arrayBuffer → chunks 数组 → join → btoa，
    // 同一份数据在内存中存在 3~4 份副本（bytes + chunks + joined + base64），
    // 200MB 文件峰值约 867MB，易导致 OOM。
    // 改用 FileReader.readAsDataURL 让浏览器原生实现读取+编码，
    // 消除 chunks 数组和 join 的中间字符串副本，降低内存峰值。
    // dataURL 形如 "data:application/pdf;base64,XXXX"，剥离前缀得到纯 base64。
    const base64 = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => {
        const result = reader.result as string
        const commaIdx = result.indexOf(',')
        if (commaIdx < 0) {
          reject(new Error('文件读取失败：dataURL 格式异常'))
          return
        }
        resolve(result.slice(commaIdx + 1))
      }
      reader.onerror = () => reject(reader.error || new Error('文件读取失败'))
      reader.readAsDataURL(f)
    })
    await wails.uploadDocument(activeKB.value, f.name, base64, f.type || 'application/octet-stream')
    showSuccess(message, `${f.name} 上传成功`)
    await loadDocuments()
  } catch (e: unknown) {
    showError(message, '上传失败', e)
  } finally {
    uploading.value = false
  }
}

async function handleRAGToggle(enabled: boolean) {
  try {
    await wails.setRAGEnabled(enabled)
    ragConfig.value.enabled = enabled
    message.destroyAll()
    if (enabled) {
      // RAG 开启时自动关闭联网搜索（两者互斥）
      settingsStore.setSearchMode('off')
      message.success('RAG 已开启，已自动关闭联网搜索', { duration: 3500 })
    } else {
      message.info('RAG 已关闭', { duration: 2000 })
    }
  } catch (e: unknown) {
    showError(message, '切换 RAG 失败', e)
  }
}

async function handleSaveRAGConfig() {
  savingRAG.value = true
  try {
    const config = await wails.getConfig()
    config.rag_top_k = ragConfig.value.topK
    config.rag_min_score = ragConfig.value.minScore
    config.rag_chunk_size = ragConfig.value.chunkSize
    config.rag_chunk_overlap = ragConfig.value.chunkOverlap
    config.embedding_model = ragConfig.value.embeddingModel
    await wails.updateConfig(config)
    showSuccess(message, 'RAG 设置已保存')
  } catch (e: unknown) {
    showError(message, '保存失败', e)
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
    pdf: '📕',
    doc: '📘',
    docx: '📘',
    md: '📝',
    txt: '📃',
    csv: '📊',
    json: '⚙️',
    xml: '📋',
    html: '🌐',
    yaml: '📑',
    yml: '📑',
    toml: '📑',
    go: '🔷',
    py: '🐍',
    js: '💛',
    ts: '💙',
    java: '☕',
    c: '🔧',
    cpp: '🔧',
    h: '📌',
    rs: '🦀',
    sh: '💻',
    rb: '💎',
    php: '🐘',
    swift: '🍎',
    kt: '🟣',
    sql: '🗄️',
    log: '📜',
    ini: '📑',
    cfg: '📑'
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

onMounted(async () => {
  await loadModels()
  await loadData()
})
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
  padding: 14px 24px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  height: var(--header-height);
  box-sizing: border-box;
}

.knowledge-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 28px 32px;
  display: flex;
  flex-direction: column;
}

/* 三段式布局：段落之间用间距分隔，不用 card 边框 */
.kb-section {
  flex-shrink: 0;
  padding-bottom: 24px;
}

.kb-section + .kb-section {
  margin-top: 8px;
}

/* .back-btn 样式已抽取到 style.css 全局（F-1.15），此处不再重复 */

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

.kb-section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.section-icon {
  font-size: 18px;
  line-height: 1;
}

/* 强排版标题：每段标题字号 16-18px，字重 600 */
.section-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.kb-count {
  font-size: 12px;
  color: var(--text-muted);
  margin-left: auto;
  padding: 2px 8px;
  background: var(--bg-tertiary);
  border-radius: var(--border-radius-sm);
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
  border-radius: var(--border-radius-sm);
}

.rag-input {
  width: 120px;
}

.embedding-config {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.embedding-select,
.embedding-input {
  width: 100%;
}

.embedding-help {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: var(--bg-tertiary);
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.6;
}

.embedding-status {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* RAG 设置状态色：启用 → --accent-g-primary */
.status-active {
  color: var(--accent-g-primary);
  font-weight: 500;
}

/* RAG 设置状态色：禁用/未配置 → --text-muted */
.status-fallback {
  color: var(--text-muted);
  font-weight: 500;
}

.embedding-recommend {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.recommend-label {
  color: var(--text-secondary);
  font-weight: 500;
}

.recommend-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.recommend-tag {
  padding: 2px 8px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
  font-family: monospace;
  font-size: 11px;
}

.recommend-tag:hover {
  background: var(--accent-primary);
  color: white;
  border-color: var(--accent-primary);
}

.embedding-tip {
  color: var(--text-tertiary);
  font-size: 11px;
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
