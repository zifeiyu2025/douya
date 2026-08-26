<template>
  <n-config-provider
    :theme="isDark ? darkTheme : undefined"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-message-provider>
      <n-dialog-provider>
        <Transition name="main-fade">
          <div
            v-if="!showSplash"
            class="app-layout shell-column"
            :class="{ dark: isDark }"
            :style="mainAreaStyle"
          >
            <!-- 应用壳层 · 三件套：
                 书签式命令条（常驻顶栏 + 窗口拖拽区）
                 会话侧栏（常驻左栏，可收起，见 .shell-body）
                 命令面板（Ctrl+K 浮层） -->
            <TopCommandBar
              :switch-duration="switchDuration"
              :switch-stage-text="switchStageText"
              :is-maximized="isMaximized"
              :sidebar-visible="!sidebarCollapsed"
              @toggle-sidebar="toggleSidebar"
              @toggle-console="toggleConsole"
              @open-palette="paletteOpen = true"
              @minimize="handleMinimize"
              @toggle-maximize="handleToggleMaximize"
              @close="handleClose"
              @header-double-click="handleHeaderDoubleClick"
            />

            <!-- 壳层主体横排：常驻会话侧栏 + 主区 -->
            <div class="shell-body">
              <SessionSidebar :collapsed="sidebarCollapsed" @collapse="sidebarCollapsed = true" />

              <div class="main-area">
                <router-view v-slot="{ Component }">
                  <Transition name="route-fade" mode="out-in">
                    <component :is="Component" />
                  </Transition>
                </router-view>
              </div>
            </div>

            <CommandPalette
              :open="paletteOpen"
              @close="paletteOpen = false"
              @toggle-console="toggleConsole"
            />
          </div>
        </Transition>
        <ModelSwitchOverlay
          :show="showSwitchOverlay"
          :overlay-model-name="overlayModelName"
          :switch-stage-text="switchStageText"
          :switch-stages="switchStages"
          :get-switch-stage-index="getSwitchStageIndex"
        />
        <ExitOverlay :show="showExitOverlay" :exit-progress="exitProgress" />
        <SplashScreen
          :visible="showSplash"
          :stage="splashStage"
          :model-name="splashModelName"
          :progress="splashProgress"
        />
        <ServerConsole ref="consoleRef" />
        <StartupErrorCard
          :show="startupErrorVisible"
          :payload="startupErrorPayload"
          @exit="handleStartupErrorExit"
        />
        <BackendDownloadDialog
          :show="backendDownloadVisible"
          :payload="backendDownloadPayload"
          @download="handleBackendDownload(true)"
          @exit="handleBackendDownload(false)"
        />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { darkTheme, zhCN, dateZhCN } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import TopCommandBar from './components/layout/TopCommandBar.vue'
import SessionSidebar from './components/layout/SessionSidebar.vue'
import CommandPalette from './components/layout/CommandPalette.vue'
import SplashScreen from './components/layout/SplashScreen.vue'
import ModelSwitchOverlay from './components/ModelSwitchOverlay.vue'
import ExitOverlay from './components/ExitOverlay.vue'
import StartupErrorCard from './components/StartupErrorCard.vue'
import BackendDownloadDialog from './components/BackendDownloadDialog.vue'
// 背景双层绘制模型：同一张图按主题各存一套透明度/模糊/遮罩参数
import { buildBackgroundStyle } from './utils/backgroundStyle'
import { useSettingsStore } from './stores/settings'
import { useThemeStore } from './stores/theme'
import { useThemeOverrides } from './composables/useThemeOverrides'
// 模型切换/窗口控制/生命周期统一在 App 层调用一次，
// 子组件通过 props/emit 消费，避免 Wails 监听重复注册
import { useModelSwitch } from './composables/useModelSwitch'
import { useWindowControls } from './composables/useWindowControls'
import { useAppLifecycle } from './composables/useAppLifecycle'
// 性能强化：ServerConsole 连带 xterm 全家桶延迟到首次打开才加载，
// consoleRef.value?.toggle() 可选链调用天然兼容异步挂载时机
const ServerConsole = defineAsyncComponent(() => import('./components/ServerConsole.vue'))

// ----- Store / 主题 -----
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()
const themeOverrides = useThemeOverrides()

const isDark = computed(() => themeStore.isDark)

const mainAreaStyle = computed(() => {
  // 每主题独立背景参数：同一张图，亮/暗主题各存一套透明度、模糊与遮罩强度
  const params = isDark.value
    ? settingsStore.config.background_dark
    : settingsStore.config.background_light
  return buildBackgroundStyle(settingsStore.config.chat_background ?? '', params)
})

// ----- 壳层开关 -----
const paletteOpen = ref(false)

// ----- 会话侧栏收起状态（localStorage 记忆，重启后保持上次选择） -----
const SIDEBAR_COLLAPSED_KEY = 'douya.sidebar.collapsed'
const sidebarCollapsed = ref(localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === '1')

watch(sidebarCollapsed, collapsed => {
  if (collapsed) {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, '1')
  } else {
    localStorage.removeItem(SIDEBAR_COLLAPSED_KEY)
  }
})

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

// ----- 全局快捷键：Ctrl+K 唤起命令面板；Esc 收起面板 -----
function handleGlobalKeydown(e: KeyboardEvent) {
  if (e.ctrlKey && (e.key === 'k' || e.key === 'K')) {
    e.preventDefault()
    paletteOpen.value = !paletteOpen.value
    return
  }
  if (e.key === 'Escape' && paletteOpen.value) {
    paletteOpen.value = false
  }
}

onMounted(() => window.addEventListener('keydown', handleGlobalKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', handleGlobalKeydown))

// ----- 调用三个 composable -----
const {
  switchDuration,
  switchStageText,
  showSwitchOverlay,
  overlayModelName,
  switchStages,
  getSwitchStageIndex
} = useModelSwitch()

const { isMaximized, handleMinimize, handleToggleMaximize, handleHeaderDoubleClick, handleClose } =
  useWindowControls()

const {
  showSplash,
  splashStage,
  splashModelName,
  splashProgress,
  showExitOverlay,
  exitProgress,
  startupErrorVisible,
  startupErrorPayload,
  handleStartupErrorExit,
  backendDownloadVisible,
  backendDownloadPayload,
  handleBackendDownload
} = useAppLifecycle()

// ----- 服务器控制台 -----
const consoleRef = ref()
const toggleConsole = () => {
  consoleRef.value?.toggle()
}
</script>

<style scoped>
/* ===== 壳层纵向骨架：命令条在上、主区吃满剩余高度 =====
 * .app-layout 的定位/尺寸/底色/isolation 由 style.css 全局层负责 */
.shell-column {
  display: flex;
  flex-direction: column;
}

/* 壳层主体横排：侧栏定宽、主区吃满剩余宽度 */
.shell-body {
  flex: 1;
  display: flex;
  min-height: 0;
}

.main-area {
  position: relative;
  flex: 1;
  min-width: 0; /* 横排上下文必需：防止长内容把主区撑破 */
  min-height: 0;
  overflow: hidden;
}

.main-fade-enter-active {
  transition:
    opacity 0.5s ease 0.3s,
    transform 0.5s cubic-bezier(0.4, 0, 0.2, 1) 0.3s;
  will-change: transform, opacity;
}

.main-fade-leave-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: transform, opacity;
}

.main-fade-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.main-fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

/* ===== 路由切换过渡 =====
 * out-in 模式：先淡出当前，再淡入下一个，避免重叠
 * translateY(6px) 轻微上滑，配合 opacity 营造层次感
 */
.route-fade-enter-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.route-fade-leave-active {
  transition:
    opacity 0.2s ease,
    transform 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.route-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.route-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
