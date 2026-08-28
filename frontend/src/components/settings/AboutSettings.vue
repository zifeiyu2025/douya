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

    <!-- 版本信息 -->
    <div class="update-section">
      <!-- 书房风：双侧格线夹衬线小节标题，替代 NDivider -->
      <div class="update-divider">
        <span class="update-divider-title">版本信息</span>
      </div>

      <div class="update-row">
        <span class="update-current">当前版本：v{{ currentVersion }}</span>
        <span class="update-status-text">更新由 Microsoft Store 接管</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NIcon, NButton, useMessage } from 'naive-ui'
import { PersonOutline, LogoGithub, ChatbubblesOutline, CopyOutline } from '@vicons/ionicons5'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import { GetGitHubURL } from '../../../wailsjs/go/main/App'
import { wails } from '../../services/wails'
import appIcon from '../../assets/images/appicon.png'
import llamaIcon from '../../assets/images/llama-icon.png'
import pkg from '../../../package.json'

const message = useMessage()
const currentVersion = ref(pkg.version)
const githubUrl = ref('https://github.com/zifeiyu2025/douya')

async function loadVersion() {
  try {
    currentVersion.value = await wails.getAppVersion()
  } catch {
    // 后端方法未就绪时使用 package.json 中的版本（构建时注入，无需手动维护）
    currentVersion.value = pkg.version
  }
  try {
    githubUrl.value = await GetGitHubURL()
  } catch {
    // 后端方法未就绪时使用默认 URL，保证"访问主页"按钮始终可用
  }
}

function openGitHub() {
  BrowserOpenURL(githubUrl.value)
}

async function copyQQ() {
  try {
    await navigator.clipboard.writeText('1090873033')
    message.success('已复制群号')
  } catch {
    message.error('复制失败')
  }
}

onMounted(() => {
  loadVersion()
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

/* 版本信息 */
.update-section {
  margin-top: 4px;
}

/* 双侧格线夹衬线标题：替代带文字的 NDivider */
.update-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 8px 0 16px;
}
.update-divider::before,
.update-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border-light);
}
.update-divider-title {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--text-secondary);
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
</style>
