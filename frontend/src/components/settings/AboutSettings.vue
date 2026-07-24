<template>
  <div class="about-settings">
    <!-- 应用信息 -->
    <div class="app-info">
      <img :src="appIcon" alt="豆芽" class="app-icon" />
      <div class="app-name">豆芽</div>
      <div class="app-version">v{{ currentVersion }}</div>
      <div class="app-tagline">本地 AI 助手</div>
    </div>

    <!-- 信息卡片 -->
    <div class="info-cards">
      <n-card class="info-card" hoverable>
        <div class="info-card-content">
          <n-icon size="20" class="info-card-icon"><PersonOutline /></n-icon>
          <div class="info-card-text">
            <span class="info-card-label">作者</span>
            <span class="info-card-value">zifeiyu</span>
          </div>
        </div>
      </n-card>

      <n-card class="info-card" hoverable @click="openGitHub">
        <div class="info-card-content">
          <n-icon size="20" class="info-card-icon"><LogoGithub /></n-icon>
          <div class="info-card-text">
            <span class="info-card-label">仓库</span>
            <span class="info-card-value info-card-link">GitHub</span>
          </div>
        </div>
      </n-card>

      <n-card class="info-card" hoverable>
        <div class="info-card-content">
          <n-icon size="20" class="info-card-icon"><ChatbubblesOutline /></n-icon>
          <div class="info-card-text">
            <span class="info-card-label">交流群</span>
            <span class="info-card-value">1090873033</span>
          </div>
          <n-button text size="tiny" class="copy-btn" @click="copyQQ">
            <template #icon>
              <n-icon><CopyOutline /></n-icon>
            </template>
          </n-button>
        </div>
      </n-card>

      <!-- 第四个卡片：支持（与其他卡片完全一致的水平布局） -->
      <n-card class="info-card" hoverable>
        <div class="info-card-content">
          <img :src="llamaIcon" alt="llama.cpp" class="info-card-icon-img" />
          <div class="info-card-text">
            <span class="info-card-label">支持</span>
            <span class="info-card-value">llama.cpp</span>
          </div>
        </div>
      </n-card>
    </div>

    <!-- 更新检查 -->
    <div class="update-section">
      <n-divider>版本更新</n-divider>

      <!-- 空闲状态 -->
      <div v-if="updateStatus === 'idle'" class="update-row">
        <span class="update-current">当前版本：v{{ currentVersion }}</span>
        <n-button type="primary" size="small" ghost @click="handleCheckUpdate">检查更新</n-button>
      </div>

      <!-- 检查中 -->
      <div v-else-if="updateStatus === 'checking'" class="update-row">
        <n-spin size="small" />
        <span class="update-status-text">检查中...</span>
      </div>

      <!-- 已是最新 -->
      <div v-else-if="updateStatus === 'up-to-date'" class="update-row">
        <n-icon size="18" color="var(--accent-success)"><CheckmarkCircleOutline /></n-icon>
        <span class="update-status-text update-success">已是最新版本</span>
      </div>

      <!-- 有更新 -->
      <div v-else-if="updateStatus === 'available'" class="update-available">
        <div class="update-row">
          <span class="update-info">
            新版本：
            <span class="version-highlight">v{{ updateInfo?.latest_version }}</span>
          </span>
          <n-button type="primary" size="small" ghost @click="handlePerformUpdate">
            立即更新
          </n-button>
        </div>
        <div v-if="updateInfo?.release_notes" class="release-notes">
          <n-collapse>
            <n-collapse-item title="更新日志" name="notes">
              <div class="release-notes-content">{{ updateInfo.release_notes }}</div>
            </n-collapse-item>
          </n-collapse>
        </div>
      </div>

      <!-- 下载中 -->
      <div v-else-if="updateStatus === 'downloading'" class="update-progress">
        <div class="update-row">
          <span class="update-status-text">正在下载更新...</span>
          <span class="update-percent">{{ downloadPercent }}%</span>
        </div>
        <n-progress
          type="line"
          :percentage="downloadPercent"
          :show-indicator="false"
          status="info"
        />
      </div>

      <!-- 安装中 -->
      <div v-else-if="updateStatus === 'installing'" class="update-row">
        <n-spin size="small" />
        <span class="update-status-text">正在安装更新...</span>
      </div>

      <!-- 错误 -->
      <div v-else-if="updateStatus === 'error'" class="update-row">
        <n-icon size="18" color="var(--accent-danger)"><CloseCircleOutline /></n-icon>
        <span class="update-status-text update-error">{{ errorMessage }}</span>
        <n-button type="primary" size="small" ghost @click="handleCheckUpdate">重试</n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import {
  NCard,
  NButton,
  NIcon,
  NSpin,
  NProgress,
  NDivider,
  NCollapse,
  NCollapseItem,
  useMessage
} from 'naive-ui'
import {
  PersonOutline,
  LogoGithub,
  ChatbubblesOutline,
  CopyOutline,
  CheckmarkCircleOutline,
  CloseCircleOutline
} from '@vicons/ionicons5'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import { wails, type UpdateInfo } from '../../services/wails'
import appIcon from '../../assets/images/appicon.png'
import llamaIcon from '../../assets/images/llama-icon.png'

const GITHUB_URL = 'https://github.com/zifeiyu2025/douya'

const message = useMessage()
const currentVersion = ref('0.11.0')
const updateStatus = ref<
  'idle' | 'checking' | 'up-to-date' | 'available' | 'downloading' | 'installing' | 'error'
>('idle')
const updateInfo = ref<UpdateInfo | null>(null)
const downloadPercent = ref(0)
const errorMessage = ref('')

async function loadVersion() {
  try {
    currentVersion.value = await wails.getAppVersion()
  } catch {
    // 后端方法未就绪时使用 package.json 中的版本
    currentVersion.value = '0.11.0'
  }
}

function openGitHub() {
  BrowserOpenURL(GITHUB_URL)
}

async function copyQQ() {
  try {
    await navigator.clipboard.writeText('1090873033')
    message.success('已复制群号')
  } catch {
    message.error('复制失败')
  }
}

async function handleCheckUpdate() {
  updateStatus.value = 'checking'
  errorMessage.value = ''
  try {
    const info = await wails.checkUpdate()
    updateInfo.value = info
    if (info.has_update) {
      updateStatus.value = 'available'
    } else {
      updateStatus.value = 'up-to-date'
    }
  } catch (e: any) {
    errorMessage.value = e?.message || '检查更新失败'
    updateStatus.value = 'error'
  }
}

async function handlePerformUpdate() {
  if (!updateInfo.value) return
  updateStatus.value = 'downloading'
  downloadPercent.value = 0
  try {
    await wails.performUpdate(updateInfo.value.download_url, updateInfo.value.latest_version)
    updateStatus.value = 'installing'
  } catch (e: any) {
    errorMessage.value = e?.message || '更新失败'
    updateStatus.value = 'error'
  }
}

function onUpdateProgress(data: any) {
  if (data?.percent !== undefined) {
    downloadPercent.value = Math.round(data.percent)
  }
  // 下载完成后进入安装状态
  if (downloadPercent.value >= 100) {
    updateStatus.value = 'installing'
  }
}

// F-1.10：保存 subscribeUpdateProgress 返回的 unsubscribe 函数，替代原 onUpdateProgress/offUpdateProgress 配对
let unsubscribeUpdateProgress: (() => void) | null = null

onMounted(() => {
  loadVersion()
  unsubscribeUpdateProgress = wails.subscribeUpdateProgress(onUpdateProgress)
})

onUnmounted(() => {
  if (unsubscribeUpdateProgress) {
    unsubscribeUpdateProgress()
    unsubscribeUpdateProgress = null
  }
})
</script>

<style scoped>
.about-settings {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 应用信息 */
.app-info {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 16px 0 8px;
}

.app-icon {
  width: 64px;
  height: 64px;
  border-radius: 14px;
  box-shadow: var(--shadow-md);
}

.app-name {
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary);
  margin-top: 4px;
}

.app-version {
  font-size: 13px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  color: var(--text-muted);
}

.app-tagline {
  font-size: 12px;
  color: var(--text-muted);
  opacity: 0.7;
}

/* 信息卡片 */
.info-cards {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.info-card {
  cursor: default;
  transition:
    transform 0.15s ease,
    box-shadow 0.15s ease;
}

.info-card:hover {
  transform: translateY(-1px);
}

.info-card :deep(.n-card__content) {
  padding: 12px 14px;
}

.info-card-content {
  display: flex;
  align-items: center;
  gap: 10px;
}

.info-card-icon {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.info-card-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.info-card-label {
  font-size: 11px;
  color: var(--text-muted);
}

.info-card-value {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
}

.info-card-link {
  color: var(--accent-primary);
  cursor: pointer;
}

.info-card-link:hover {
  text-decoration: underline;
}

.copy-btn {
  margin-left: auto;
  flex-shrink: 0;
}

/* 第四个卡片图标：与其他 n-icon 同样大小 20x20，方形透明背景 */
.info-card-icon-img {
  width: 20px;
  height: 20px;
  flex-shrink: 0;
  display: block;
  object-fit: contain;
}

/* 更新区域 */
.update-section {
  margin-top: 4px;
}

.update-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.update-current {
  font-size: 13px;
  color: var(--text-secondary);
}

.update-status-text {
  font-size: 13px;
  color: var(--text-secondary);
}

.update-success {
  color: var(--accent-success);
}

.update-error {
  color: var(--accent-danger);
}

.update-available {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.update-info {
  font-size: 13px;
  color: var(--text-secondary);
}

.version-highlight {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-weight: 600;
  color: var(--accent-primary);
}

.update-progress {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.update-percent {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  color: var(--text-secondary);
  margin-left: auto;
}

.release-notes {
  margin-top: 4px;
}

.release-notes-content {
  font-size: 13px;
  color: var(--text-secondary);
  white-space: pre-wrap;
  line-height: 1.6;
  max-height: 200px;
  overflow-y: auto;
}
</style>
