<!--
  ModelDownloader: 内置模型下载器（来源 ModelScope / HF 镜像）
  生活类比：像"网购模型"——选平台、搜商品、挑款式、下单，然后等"快递"送到 models 目录。
  流程：
    1. 选择下载源（ModelScope 魔搭社区 / HF 国内镜像）
    2. 输入关键词搜索出候选模型仓库
    3. 点选一个仓库，展开其文件列表
    4. 选择 .gguf 主文件，可选配 MMProj 多模态投影文件
    5. 点击"开始下载"，实时显示进度；支持断点续传、重复文件自动跳过
-->
<template>
  <div class="model-downloader">
    <!-- 顶部：下载源选择 + 搜索框 -->
    <div class="downloader-toolbar">
      <n-select
        v-model:value="provider"
        class="provider-select"
        :options="providerOptions"
        :disabled="downloading"
        aria-label="下载源"
      />
      <n-input
        v-model:value="keyword"
        class="search-input"
        placeholder="输入模型关键词，如 Qwen3、DeepSeek-R1"
        clearable
        :disabled="downloading"
        @keydown.enter="search"
      />
      <n-button
        type="primary"
        :loading="searching"
        :disabled="downloading || keyword.trim() === ''"
        @click="search"
      >
        <template #icon>
          <n-icon :component="SearchOutline" />
        </template>
        搜索
      </n-button>
    </div>

    <!-- 搜索提示 / 空状态 -->
    <n-empty
      v-if="!searched && !searching"
      description="搜索想要下载的模型"
      class="state-empty"
      size="small"
    >
      <template #extra>
        <span class="state-hint">
          支持 ModelScope 魔搭社区 与 HF 国内镜像，下载断点续传、断网可重来
        </span>
      </template>
    </n-empty>

    <!-- 模型搜索结果列表 -->
    <div v-if="searched" class="model-list">
      <div v-if="searching" class="list-loading">
        <n-spin size="small" />
        正在搜索并校验是否有可下载的 .gguf…
      </div>
      <template v-else>
        <n-empty
          v-if="models.length === 0"
          description="没有找到含 .gguf 文件的模型，换个关键词试试"
          size="small"
        />
        <div
          v-for="m in models"
          :key="m.provider + '/' + m.repo_id"
          class="model-item"
          :class="{ 'model-item--active': selectedModel?.repo_id === m.repo_id }"
          @click="selectModel(m)"
        >
          <div class="model-item__main">
            <div class="model-item__name">{{ m.name }}</div>
            <div class="model-item__meta">
              <span>仓库：{{ m.repo_id }}</span>
              <span v-if="m.downloads > 0">下载 {{ formatCount(m.downloads) }}</span>
              <span v-if="m.likes > 0">点赞 {{ formatCount(m.likes) }}</span>
            </div>
            <div
              v-if="filesLoading && selectedModel?.repo_id === m.repo_id"
              class="model-item__files-loading"
            >
              <n-spin size="small" />
              加载文件列表…
            </div>
            <!-- 加载失败：明确展示原因，避免误报为"无可下载" -->
            <div
              v-else-if="filesError && selectedModel?.repo_id === m.repo_id"
              class="model-item__files model-item__files--error"
            >
              <div class="files-title">加载文件列表失败</div>
              <div class="files-err-msg">{{ filesError }}</div>
            </div>
            <!-- 已选模型的文件选择区（含可选主 .gguf） -->
            <!-- @click.stop：文件选择/勾选MMProj/开始下载这些内部操作不再冒泡到条目，
                 避免点击内部控件时误触发条目的展开/收起逻辑 -->
            <div
              v-else-if="selectedModel?.repo_id === m.repo_id && ggufFiles.length > 0"
              class="model-item__files"
              @click.stop
            >
              <div class="files-title">请选择要下载的文件：</div>
              <n-radio-group v-model:value="selectedFile" class="files-group">
                <div v-for="f in ggufFiles" :key="f.path" class="file-option">
                  <n-radio :value="f.path" :disabled="downloading">
                    <span class="file-option__name">{{ f.path }}</span>
                  </n-radio>
                  <span class="file-option__size">{{ formatSize(f.size) }}</span>
                  <n-tag v-if="f.is_mmproj" size="tiny" type="warning" :bordered="false">
                    MMProj
                  </n-tag>
                </div>
              </n-radio-group>
              <template v-if="mmprojFiles.length > 0">
                <div class="files-title files-title--sub">可选：多模态投影文件（MMProj）</div>
                <div class="files-group">
                  <div v-for="f in mmprojFiles" :key="f.path" class="file-option">
                    <n-checkbox
                      :checked="selectedMmproj === f.path"
                      :disabled="downloading"
                      :aria-label="'额外勾选 MMProj 文件 ' + f.path"
                      @update:checked="(val: boolean) => onToggleMmproj(f.path, val)"
                    >
                      <span class="file-option__name file-option__name--mmproj">{{ f.path }}</span>
                    </n-checkbox>
                    <span class="file-option__size">{{ formatSize(f.size) }}</span>
                    <n-tag size="tiny" type="warning" :bordered="false">MMProj</n-tag>
                  </div>
                </div>
              </template>
              <div class="mmproj-note">
                主文件必须是 .gguf；若为多模态模型，可额外勾选上方的 MMProj 文件以启用图片理解
              </div>
              <n-button
                type="primary"
                size="small"
                :loading="startingDownload"
                :disabled="downloading || !selectedFile"
                @click="startDownload"
              >
                开始下载 {{ selectedFile ? formatSize(selectedFileSize) : '' }}
              </n-button>
            </div>
            <!-- 没有任何主 .gguf，但可能有 MMProj：给出更准确提示 -->
            <div v-else-if="selectedModel?.repo_id === m.repo_id" class="model-item__files">
              <div class="files-title">
                {{
                  fileList.length > 0
                    ? '该仓库只有 MMProj 文件，缺少主 .gguf 模型文件'
                    : '该仓库暂无可下载的 .gguf 文件'
                }}
              </div>
            </div>
          </div>
        </div>

        <!-- 加载更多：按钮式触发，性能友好 -->
        <div v-if="models.length > 0 && (hasMore || loadingMore)" class="load-more">
          <n-button
            v-if="!loadingMore"
            size="small"
            class="load-more-btn"
            :disabled="searching"
            @click="loadMore"
          >
            加载更多
          </n-button>
          <div v-else class="list-loading">
            <n-spin size="small" />
            正在加载下一页…
          </div>
          <div v-if="loadingMoreError" class="load-more-err">{{ loadingMoreError }}</div>
        </div>
      </template>
    </div>

    <!-- 下载进度列表 -->
    <div v-if="Object.keys(progressMap).length > 0" class="download-progress">
      <div class="progress-title">下载进度</div>
      <div v-for="(p, file) in progressMap" :key="file" class="progress-item">
        <div class="progress-item__head">
          <span class="progress-item__file" :title="file">{{ file }}</span>
          <span class="progress-item__status" :class="'status--' + p.status">
            {{ statusText(p) }}
          </span>
        </div>
        <n-progress
          v-if="p.status === 'downloading' || p.status === 'paused'"
          type="line"
          :percentage="Math.round(p.percent)"
          :indicator-placement="'inside'"
          status="default"
          :height="8"
        />
        <n-progress
          v-else
          type="line"
          :percentage="p.status === 'completed' ? 100 : 0"
          :indicator-placement="'inside'"
          :status="p.status === 'failed' ? 'error' : 'success'"
          :height="8"
        />
        <div v-if="p.status === 'failed' && p.error" class="progress-item__error">
          {{ p.error }}
        </div>
        <div v-if="p.status === 'downloading'" class="progress-item__bytes">
          {{ formatSize(p.downloaded) }} / {{ formatSize(p.total_bytes) }}
        </div>
      </div>
    </div>

    <!-- 下载完成：移除进度条，提示重启应用以加载新模型 -->
    <div v-if="downloadDone" class="download-done">
      <div class="download-done__text">模型下载完成，重启应用后即可加载使用</div>
      <button
        class="download-done__restart"
        :disabled="restarting"
        :aria-label="'重启应用'"
        @click="restartApp"
      >
        {{ restarting ? '正在重启…' : '重启应用' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  NSelect,
  NInput,
  NButton,
  NIcon,
  NRadioGroup,
  NRadio,
  NTag,
  NProgress,
  NSpin,
  NEmpty
} from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import {
  wails,
  type HubModel,
  type HubFile,
  type ModelDownloadProgress
} from '../../services/wails'
import { logError } from '../../utils/logger'

defineOptions({ name: 'ModelDownloader' })

// 下载源选项：默认 ModelScope（国内直连快，中文友好），HF 镜像作为备选
const providerOptions = [
  { label: 'ModelScope 魔搭社区', value: 'modelscope' },
  { label: 'HF 国内镜像 (hf-mirror.com)', value: 'hfmirror' }
]

const provider = ref('modelscope')
const keyword = ref('')
const searching = ref(false)
const searched = ref(false)
const models = ref<HubModel[]>([])
const selectedModel = ref<HubModel | null>(null)

// 分页/加载更多状态
const currentPage = ref(1) // 当前已加载的最大页码
const loadingMore = ref(false) // 是否正在加载下一页
const hasMore = ref(false) // 是否还有下一页可加载
const loadingMoreError = ref('') // 加载更多失败时的提示

// 文件列表状态
const filesLoading = ref(false)
const filesError = ref('')
const fileList = ref<HubFile[]>([])
const selectedFile = ref('')
const selectedMmproj = ref('') // 用户额外勾选的 MMProj 文件

// 下载状态
const downloading = ref(false)
const startingDownload = ref(false)
const progressMap = ref<Record<string, ModelDownloadProgress>>({})

// 存储事件退订函数，组件卸载时清理
let unsubProgress: (() => void) | null = null
let unsubComplete: (() => void) | null = null

// 仅保留 .gguf 主文件（排除 MMProj），作为单选主文件列表
const ggufFiles = computed(() => fileList.value.filter(f => f.is_gguf && !f.is_mmproj))
// MMProj 多模态投影文件列表
const mmprojFiles = computed(() => fileList.value.filter(f => f.is_mmproj))

// 当前选中主文件的大小（显示在下载按钮上）
const selectedFileSize = computed(() => {
  const f = fileList.value.find(x => x.path === selectedFile.value)
  return f ? f.size : 0
})

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '未知大小'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

function formatCount(n: number): string {
  if (n >= 10000) return `${(n / 10000).toFixed(1)}w`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}

async function search() {
  const q = keyword.value.trim()
  if (!q) return
  searching.value = true
  searched.value = true
  filesError.value = ''
  selectedModel.value = null
  fileList.value = []
  selectedFile.value = ''
  loadingMoreError.value = ''
  hasMore.value = false
  downloadDone.value = false
  downloadFailed.value = false
  try {
    models.value = await wails.searchHubModels(provider.value, q, 1)
    currentPage.value = 1
    // 还有没有下一页：本页是满页（约30条）时不确定是否到底，先按"可能还有"处理，
    // 用户点"加载更多"拿到空页自然就停（API 会返回空，hasMore 据此判断）。
    hasMore.value = true
  } catch (e) {
    models.value = []
    hasMore.value = false
    logError('搜索模型失败', e)
  } finally {
    searching.value = false
  }
}

/** 加载下一页并追加到列表尾部。按钮式触发，避免无限滚动带来的意外多页过滤请求。 */
async function loadMore() {
  // 防重入：搜索中/正在加载/换源/换词后都不可点
  if (searching.value || loadingMore.value) return
  const q = keyword.value.trim()
  if (!q) return
  loadingMore.value = true
  loadingMoreError.value = ''
  try {
    const next = await wails.searchHubModels(provider.value, q, currentPage.value + 1)
    if (next.length === 0) {
      // 下一页为空：说明没有更多仓库了，关闭入口
      hasMore.value = false
      return
    }
    // 跨页可能重复（下载源排序变化），按 repo_id 去重
    const known = new Set(models.value.map(m => m.repo_id))
    const fresh = next.filter(m => !known.has(m.repo_id))
    models.value = models.value.concat(fresh)
    currentPage.value += 1
    // 本页少于一页容量，说明已到底；否则保留入口继续尝试
    if (next.length < 30) {
      hasMore.value = false
    }
  } catch (e) {
    loadingMoreError.value = '加载更多失败：' + String(e)
    logError('加载更多失败', e)
  } finally {
    loadingMore.value = false
  }
}

async function selectModel(m: HubModel) {
  // 再次点击已展开的仓库：收缩（收起文件选择区）。点击其他仓库则切换展开。
  if (selectedModel.value?.repo_id === m.repo_id) {
    selectedModel.value = null
    fileList.value = []
    selectedFile.value = ''
    selectedMmproj.value = ''
    filesError.value = ''
    return
  }
  selectedModel.value = m
  filesLoading.value = true
  filesError.value = ''
  fileList.value = []
  selectedFile.value = ''
  selectedMmproj.value = ''
  try {
    const files = await wails.listHubModelFiles(provider.value, m.repo_id)
    fileList.value = files
    // 默认选中第一个主 .gguf 文件
    if (ggufFiles.value.length > 0) {
      selectedFile.value = ggufFiles.value[0].path
    }
  } catch (e) {
    filesError.value = '加载文件列表失败：' + String(e)
    logError('加载文件列表失败', e)
  } finally {
    filesLoading.value = false
  }
}

async function startDownload() {
  if (!selectedModel.value || !selectedFile.value) return
  const mainFile = selectedFile.value
  // 使用用户勾选的 MMProj（未勾选则传空，不下载 MMProj）
  const mmproj = selectedMmproj.value
  startingDownload.value = true
  try {
    await wails.downloadHubModel(provider.value, selectedModel.value.repo_id, mainFile, mmproj)
    downloading.value = true
    // 初始化进度占位
    progressMap.value[mainFile] = {
      provider: selectedModel.value.provider,
      repo_id: selectedModel.value.repo_id,
      file_path: mainFile,
      total_bytes: 0,
      downloaded: 0,
      percent: 0,
      status: 'downloading',
      error: ''
    }
    if (mmproj) {
      progressMap.value[mmproj] = {
        provider: selectedModel.value.provider,
        repo_id: selectedModel.value.repo_id,
        file_path: mmproj,
        total_bytes: 0,
        downloaded: 0,
        percent: 0,
        status: 'waiting',
        error: ''
      }
    }
  } catch (e) {
    logError('发起下载失败', e)
  } finally {
    startingDownload.value = false
  }
}

/** 勾选/取消 MMProj：多选二时互斥，同一时刻至多勾选一个 */
function onToggleMmproj(path: string, checked: boolean) {
  selectedMmproj.value = checked ? path : ''
}

/** 重启应用：启动新进程后退出当前进程，用于让下载完成的模型立即生效 */
async function restartApp() {
  restarting.value = true
  try {
    await wails.restartApp()
  } catch (e) {
    restarting.value = false
    logError('重启应用失败', e)
  }
}

function statusText(p: ModelDownloadProgress): string {
  switch (p.status) {
    case 'downloading':
      return '下载中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '失败'
    case 'paused':
      return '已暂停（保留断点）'
    case 'waiting':
      return '等待中'
    default:
      return p.status
  }
}

// 是否显示"下载完成，重启应用"提示
const downloadDone = ref(false)
// 是否有下载失败（失败时不自动隐藏，便于查看错误）
const downloadFailed = ref(false)
// 正在重启应用（防重复点击）
const restarting = ref(false)

onMounted(() => {
  unsubProgress = wails.subscribeModelDownloadProgress(p => {
    progressMap.value[p.file_path] = p
  })
  unsubComplete = wails.subscribeModelDownloadComplete(result => {
    // 全部主文件+MMProj完成后关闭"下载中"状态
    downloading.value = false
    if (result.success) {
      // 下载完成：移除下载进度，提示重启应用以加载新模型
      downloadDone.value = true
      downloadFailed.value = false
      progressMap.value = {}
    } else {
      downloadFailed.value = true
      // 失败保留进度项，供用户查看错误详情；不放重启按钮
    }
  })
})

onUnmounted(() => {
  unsubProgress?.()
  unsubComplete?.()
})
</script>

<style scoped>
.model-downloader {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.downloader-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.provider-select {
  width: 180px;
  flex-shrink: 0;
}

.search-input {
  flex: 1;
}

.state-empty {
  padding: 28px 0;
}

.state-hint {
  font-size: 12px;
  color: var(--text-muted);
}

.model-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* 加载更多：按钮式触发，避免无限滚动带来的高开销自动请求 */
.load-more {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
}

.load-more-btn {
  min-width: 160px;
}

.load-more-err {
  color: var(--error-color, #e5484d);
  font-size: 12px;
}

.list-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px 0;
  color: var(--text-muted);
  font-size: 13px;
}

.model-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  padding: 12px 14px;
  cursor: pointer;
  transition:
    border-color 0.2s,
    background 0.2s;
}

.model-item:hover {
  border-color: var(--color-primary);
}

.model-item--active {
  border-color: var(--color-primary);
  background: var(--bg-hover);
}

/* 右侧"直接下载"按钮已移除：下载入口统一放在展开条目后的"开始下载" */
.model-item__main {
  flex: 1;
  min-width: 0;
}

.model-item__name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  word-break: break-all;
}

.model-item__meta {
  margin-top: 4px;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 12px;
  color: var(--text-muted);
}

.model-item__files-loading {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-muted);
}

.model-item__files {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed var(--border-color);
}

.model-item__files--error .files-title {
  color: var(--error-color, #d03050);
}

.files-err-msg {
  font-size: 12px;
  color: var(--error-color, #d03050);
  word-break: break-all;
}

.files-title {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 8px;
}

.files-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 10px;
}

.file-option {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.file-option__name {
  color: var(--text-primary);
  word-break: break-all;
}

.file-option__size {
  color: var(--text-muted);
  font-size: 12px;
  flex-shrink: 0;
}

.mmproj-note {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 10px;
}

.download-progress {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.progress-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.progress-item {
  background: var(--bg-tertiary);
  border-radius: 8px;
  padding: 10px 12px;
}

.progress-item__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}

.progress-item__file {
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-item__status {
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
}

.progress-item__status.status--downloading {
  color: var(--color-primary);
}

.progress-item__status.status--completed {
  color: var(--success-color);
}

.progress-item__status.status--failed {
  color: var(--color-error);
}

.progress-item__status.status--paused {
  color: var(--color-warning);
}

.progress-item__bytes {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 4px;
}

.progress-item__error {
  font-size: 12px;
  color: var(--error-color, #d03050);
  margin-top: 4px;
  word-break: break-all;
}

/* 下载完成后的重启提示：区分于普通进度，用高对比重点提示（成功绿色） */
.download-done {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 16px;
  border: 1px solid var(--success-color);
  border-radius: 10px;
  background: color-mix(in srgb, var(--success-color) 8%, transparent);
}

.download-done__text {
  color: var(--success-color);
  font-size: 13px;
  font-weight: 600;
}

.download-done__restart {
  padding: 6px 22px;
  font-size: 13px;
  font-weight: 600;
  color: var(--success-color);
  background: transparent;
  border: 1px solid var(--success-color);
  border-radius: 8px;
  cursor: pointer;
  transition:
    color 0.2s,
    background 0.2s,
    border-color 0.2s;
}

.download-done__restart:hover:not(:disabled) {
  color: #fff;
  background: var(--success-color);
  border-color: var(--success-color);
}

.download-done__restart:focus-visible {
  outline: 2px solid var(--success-color);
  outline-offset: 2px;
}

.download-done__restart:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
