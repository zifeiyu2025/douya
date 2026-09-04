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

      <n-card class="info-card" hoverable>
        <div class="info-card-content">
          <n-icon size="20" class="info-card-icon"><InformationCircleOutline /></n-icon>
          <div class="info-card-text">
            <span class="info-card-label">当前版本</span>
            <span class="info-card-value">v{{ currentVersion }}</span>
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

    <!-- 帮助与反馈：一键复制诊断信息（仿 VS Code / 微信「帮助与反馈」） -->
    <n-card class="diagnostics-card" hoverable>
      <div class="diagnostics-content">
        <n-icon size="20" class="diagnostics-icon"><BugOutline /></n-icon>
        <div class="diagnostics-text">
          <span class="diagnostics-title">遇到问题？</span>
          <span class="diagnostics-desc">
            点击复制诊断信息，连同问题描述一起发给开发者，能更快定位原因。信息已自动脱敏，不含任何密钥。
          </span>
        </div>
        <n-button
          type="primary"
          tertiary
          size="small"
          class="diagnostics-btn"
          :loading="diagnosticsLoading"
          @click="copyDiagnostics"
        >
          <template #icon>
            <n-icon><CopyOutline /></n-icon>
          </template>
          复制诊断信息
        </n-button>
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NIcon, NButton, useMessage } from 'naive-ui'
import {
  PersonOutline,
  InformationCircleOutline,
  ChatbubblesOutline,
  CopyOutline,
  BugOutline
} from '@vicons/ionicons5'
import { wails } from '../../services/wails'
import appIcon from '../../assets/images/appicon.png'
import llamaIcon from '../../assets/images/llama-icon.png'
import pkg from '../../../package.json'

const message = useMessage()
const currentVersion = ref(pkg.version)
const diagnosticsLoading = ref(false)

async function loadVersion() {
  try {
    currentVersion.value = await wails.getAppVersion()
  } catch {
    // 后端方法未就绪时使用 package.json 中的版本（构建时注入，无需手动维护）
    currentVersion.value = pkg.version
  }
}

async function copyQQ() {
  try {
    await navigator.clipboard.writeText('1090873033')
    message.success('已复制群号')
  } catch {
    message.error('复制失败')
  }
}

// 一键复制诊断信息：后端生成脱敏环境快照 → 写入剪贴板。
// 用户反馈问题时连同描述一起粘贴，开发者即可获得可复现问题所需的全部上下文。
async function copyDiagnostics() {
  diagnosticsLoading.value = true
  try {
    const text = await wails.getDiagnostics()
    await navigator.clipboard.writeText(text)
    message.success('诊断信息已复制，可直接粘贴反馈')
  } catch (err) {
    console.error('复制诊断信息失败:', err)
    message.error('复制诊断信息失败，请打开日志目录查看')
  } finally {
    diagnosticsLoading.value = false
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

/* 应用 LOGO：透明原图直接呈现——无边框、无底色、无阴影（与欢迎页扉页小印风格一致） */
.app-icon {
  width: 64px;
  height: 64px;
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

/* 帮助与反馈卡片 */
.diagnostics-card {
  border-color: var(--border-color);
}

.diagnostics-card :deep(.n-card__content) {
  padding: 14px 16px;
}

.diagnostics-content {
  display: flex;
  align-items: center;
  gap: 12px;
}

.diagnostics-icon {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.diagnostics-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.diagnostics-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.diagnostics-desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-muted);
}

.diagnostics-btn {
  flex-shrink: 0;
}
</style>
