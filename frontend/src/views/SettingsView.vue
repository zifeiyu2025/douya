<template>
  <div class="settings-container">
    <div class="settings-header">
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
      <span class="settings-title">设置</span>
    </div>
    <div class="settings-content">
      <n-form label-placement="left" label-width="120" :model="formConfig">
        <n-collapse v-model:expanded-names="expandedNames" display-directive="show">
          <!-- ==================== 外观 ==================== -->
          <n-collapse-item name="appearance">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">外观</span>
                <span class="settings-group-desc">主题、背景、头像</span>
              </div>
            </template>
            <AppearanceSettings />
          </n-collapse-item>

          <!-- ==================== AI 对话 ==================== -->
          <n-collapse-item name="chat">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">AI 对话</span>
                <span class="settings-group-desc">提示词、推理、生成参数、朗读</span>
              </div>
            </template>
            <AIChatSettings />
          </n-collapse-item>

          <!-- ==================== 联网搜索 ==================== -->
          <n-collapse-item name="search">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">联网搜索</span>
                <span class="settings-group-desc">搜索引擎、搜索密钥</span>
              </div>
            </template>
            <WebSearchSettings />
          </n-collapse-item>

          <!-- ==================== 性能 ==================== -->
          <n-collapse-item name="performance">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">性能</span>
                <span class="settings-group-desc">GPU、后端、KV 缓存、推测解码</span>
              </div>
            </template>
            <PerformanceSettings />
          </n-collapse-item>

          <!-- ==================== API 服务 ==================== -->
          <n-collapse-item name="api">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">API 服务</span>
                <span class="settings-group-desc">端点、密钥、局域网访问</span>
              </div>
            </template>
            <APIServiceSettings />
          </n-collapse-item>

          <!-- ==================== 高级工具 ==================== -->
          <n-collapse-item name="advanced">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">高级</span>
                <span class="settings-group-desc">MCP 工具、RAG、LoRA、实验功能</span>
              </div>
            </template>
            <AdvancedExperimentalSettings />
          </n-collapse-item>

          <!-- ==================== 模型下载 ==================== -->
          <n-collapse-item ref="modelDownloadRef" name="model-download">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">模型下载</span>
                <span class="settings-group-desc">ModelScope / HF 镜像内置下载</span>
              </div>
            </template>
            <ModelDownloader />
          </n-collapse-item>

          <!-- ==================== 关于 ==================== -->
          <n-collapse-item name="about">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">关于</span>
                <span class="settings-group-desc">版本与更新</span>
              </div>
            </template>
            <AboutSettings />
          </n-collapse-item>
        </n-collapse>
      </n-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { defineAsyncComponent, nextTick, onMounted, provide, ref } from 'vue'
import { useRoute } from 'vue-router'
import { NForm, NCollapse, NCollapseItem, useMessage } from 'naive-ui'
import AppearanceSettings from '../components/settings/AppearanceSettings.vue'
import AIChatSettings from '../components/settings/AIChatSettings.vue'
import WebSearchSettings from '../components/settings/WebSearchSettings.vue'
import PerformanceSettings from '../components/settings/PerformanceSettings.vue'
import APIServiceSettings from '../components/settings/APIServiceSettings.vue'
import AdvancedExperimentalSettings from '../components/settings/AdvancedExperimentalSettings.vue'
import AboutSettings from '../components/settings/AboutSettings.vue'
// C-8 性能项：模型下载器体积大且非首屏必需，异步加载减小设置页首包
const ModelDownloader = defineAsyncComponent(
  () => import('../components/settings/ModelDownloader.vue')
)
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from '../components/settings/settingsContext'
import { useSettingsCore } from '../components/settings/composables/useSettingsCore'
import { useAppearanceSettings } from '../components/settings/composables/useAppearanceSettings'
import { useAIChatSettings } from '../components/settings/composables/useAIChatSettings'
import { usePerformanceSettings } from '../components/settings/composables/usePerformanceSettings'
import { useAPIServiceSettings } from '../components/settings/composables/useAPIServiceSettings'

const message = useMessage()
const route = useRoute()

// 折叠面板受控展开：默认展开"外观"；欢迎页"前往模型下载"跳转时带
// open=model-download 路由参数，此处自动展开"模型下载"面板并滚动到可见，
// 避免用户落地设置页后找不到下载入口（无模型首启"死路"修复）
const expandedNames = ref<string[]>(['appearance'])
const modelDownloadRef = ref<InstanceType<typeof NCollapseItem> | null>(null)

// C-5 设置域重建：SettingsView 退化为编排壳，原 805 行 script 拆分到 composables/
// 组装顺序即依赖链：core ← performance ← aiChat；appearance / apiService 仅依赖 core
const core = useSettingsCore(message)
const appearance = useAppearanceSettings(core, message)
const performance = usePerformanceSettings(core)
const aiChat = useAIChatSettings(core, performance, message)
const apiService = useAPIServiceSettings(core, message)

// 模板 n-form 的 :model 绑定需要表单对象
const { formConfig } = core

// 通过 provide 向子组件注入共享状态
const settingsContext: SettingsContext = { core, appearance, aiChat, performance, apiService }
provide(SETTINGS_CONTEXT_KEY, settingsContext)

// 显式按序初始化（原 onMounted 逻辑等价拆分）：配置 → 性能/GPU → 密钥状态 → 模型预设
onMounted(async () => {
  // 接受路由参数展开指定面板（如 model-download），并滚动到对应区域
  const openName = route.query.open
  if (openName && typeof openName === 'string' && !expandedNames.value.includes(openName)) {
    expandedNames.value = [...expandedNames.value, openName]
    await nextTick()
    modelDownloadRef.value?.$el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
  await core.init()
  await performance.init()
  await apiService.init()
  await aiChat.init()
})
</script>

<style scoped>
.settings-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
}

.settings-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 24px;
  border-bottom: 1px solid var(--border-color);
}

/* .back-btn 样式已抽取到 style.css 全局（F-1.15），此处不再重复 */

.settings-title {
  font-size: 22px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.2px;
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px 24px 80px;
  max-width: 640px;
  width: 100%;
  margin: 0 auto;
  /* 自定义滚动条，避免占用内容空间 */
  scrollbar-width: thin;
  scrollbar-color: var(--border-color) transparent;
}

/* Q10: 设置面板的滚动条与全局 ::-webkit-scrollbar 有意不同
 * 全局用 --scrollbar-thumb + 4px 圆角 + hover 时变主题色（显眼）
 * 此处用 --border-color + 3px 圆角 + hover 时变 --text-tertiary（低调，不抢设置内容焦点）
 * 不要强行统一，避免设置面板滚动条变得过于显眼影响阅读 */
.settings-content::-webkit-scrollbar {
  width: 6px;
}

.settings-content::-webkit-scrollbar-track {
  background: transparent;
}

.settings-content::-webkit-scrollbar-thumb {
  background-color: var(--border-color);
  border-radius: 3px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background-color: var(--text-tertiary);
}

/* 分隔线间距优化，避免配置区域过于拥挤 */
.settings-content :deep(.n-divider) {
  margin-top: 24px;
  margin-bottom: 16px;
}

.settings-content :deep(.n-divider:first-child) {
  margin-top: 0;
}

/* 表单项间距优化 */
.settings-content :deep(.n-form-item) {
  margin-bottom: 16px;
}

/* 折叠面板密度优化：项之间间距适中，避免过于拥挤 */
.settings-content :deep(.n-collapse-item) {
  margin-bottom: 4px;
}

.settings-content :deep(.n-collapse-item__header) {
  padding: 12px 8px;
  border-radius: var(--border-radius-sm);
  transition: background-color var(--transition-fast);
}

.settings-content :deep(.n-collapse-item__header:hover) {
  background: var(--bg-hover);
}

.settings-content :deep(.n-collapse-item__content-inner) {
  padding-top: 8px;
}

.settings-group-header {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.settings-group-title {
  font-size: 16px;
  font-weight: 600;
}

.settings-group-desc {
  font-size: 12px;
  color: var(--n-text-color-3);
}
</style>
