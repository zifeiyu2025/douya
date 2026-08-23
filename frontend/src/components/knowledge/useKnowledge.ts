/*
 * useKnowledge: 知识库域单一状态源（C-6 自 KnowledgeView.vue 逐字搬移）
 * 三个子组件（KBSelector / DocumentManager / RagSettings）经 provide/inject 共享此上下文。
 */
import { ref, computed } from 'vue'
import { useMessage, useDialog, type UploadCustomRequestOptions } from 'naive-ui'
import {
  wails,
  type CollectionInfo,
  type DocumentMeta,
  type ModelOption
} from '../../services/wails'
import { useSettingsStore } from '../../stores/settings'
import { showError, showSuccess, showWarning } from '../../utils/showError'
import { logError } from '../../utils/logger'

export function useKnowledge() {
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
      await wails.uploadDocument(
        activeKB.value,
        f.name,
        base64,
        f.type || 'application/octet-stream'
      )
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

  // 原 onMounted 编排改为显式 init，由视图壳在挂载时调用
  async function init() {
    await loadModels()
    await loadData()
  }

  return {
    // 状态
    knowledgeBases,
    activeKB,
    documents,
    uploading,
    savingRAG,
    showCreateKB,
    newKBName,
    availableModels,
    selectedEmbeddingModel,
    manualEmbeddingModel,
    ragConfig,
    // computed
    kbOptions,
    embeddingModelOptions,
    // 方法
    loadModels,
    loadDocuments,
    loadData,
    handleKBChange,
    handleCreateKB,
    handleDeleteKB,
    handleDeleteDoc,
    handleFileUpload,
    handleRAGToggle,
    handleSaveRAGConfig,
    onEmbeddingModelSelect,
    onManualEmbeddingModelInput,
    selectRecommendedModel,
    formatFileSize,
    getFileIcon,
    formatTime,
    init
  }
}

export type KnowledgeContext = ReturnType<typeof useKnowledge>
