<!--
  ModelDownloader: 内置模型下载器（来源 ModelScope / HF 镜像）
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
        :disabled="dlStore.hasActive"
        aria-label="下载源"
      />
      <n-input
        v-model:value="keyword"
        class="search-input"
        placeholder="输入模型关键词，如 Qwen3、DeepSeek-R1"
        clearable
        :disabled="dlStore.hasActive"
        @keydown.enter="search"
      />
      <n-button
        type="primary"
        :loading="searching"
        :disabled="dlStore.hasActive || keyword.trim() === ''"
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
              <!-- 主文件大小：搜索结果已按模型大小升序排序，这里展示排序依据 -->
              <span v-if="m.main_file_size">主文件 {{ formatSize(m.main_file_size) }}</span>
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
                  <n-radio :value="f.path" :disabled="dlStore.hasActive">
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
                      :disabled="dlStore.hasActive"
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
                :disabled="dlStore.hasActive || !selectedFile"
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

    <!-- 下载进度/完成/失败状态已抽出为全局悬浮卡（App.vue 挂载的 ModelDownloadStatus），
         任意页面可见，不再内嵌在本面板中 -->
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  NSelect,
  NInput,
  NButton,
  NIcon,
  NRadioGroup,
  NRadio,
  NTag,
  NSpin,
  NEmpty
} from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import { wails, type HubModel, type HubFile } from '../../services/wails'
import { useModelDownloadStore } from '../../stores/modelDownload'
import { formatSize } from '../../utils/model'
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

// 下载状态：进度/完成/重试统一收敛在全局 modelDownload store，
// 与 App.vue 挂载的悬浮状态卡（ModelDownloadStatus）共享同一份数据
const dlStore = useModelDownloadStore()
const startingDownload = ref(false)

// 仅保留 .gguf 主文件（排除 MMProj），作为单选主文件列表
const ggufFiles = computed(() => fileList.value.filter(f => f.is_gguf && !f.is_mmproj))
// MMProj 多模态投影文件列表
const mmprojFiles = computed(() => fileList.value.filter(f => f.is_mmproj))

// 当前选中主文件的大小（显示在下载按钮上）
const selectedFileSize = computed(() => {
  const f = fileList.value.find(x => x.path === selectedFile.value)
  return f ? f.size : 0
})

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

/** 发起下载：进度/完成/失败状态由全局 store 与悬浮卡接管，本面板只负责发起 */
async function startDownload() {
  if (!selectedModel.value || !selectedFile.value) return
  const attempt = {
    provider: provider.value,
    repoId: selectedModel.value.repo_id,
    mainFile: selectedFile.value,
    mmproj: selectedMmproj.value
  }
  startingDownload.value = true
  try {
    await wails.downloadHubModel(attempt.provider, attempt.repoId, attempt.mainFile, attempt.mmproj)
    // 发起成功才记录：留底参数供悬浮卡一键重试，并弹出悬浮卡展示进度
    dlStore.recordAttempt(attempt)
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
  color: var(--error-color);
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
  color: var(--error-color);
}

.files-err-msg {
  font-size: 12px;
  color: var(--error-color);
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

</style>
