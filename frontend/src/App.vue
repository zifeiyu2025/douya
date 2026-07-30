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
            class="app-layout"
            :class="{ dark: isDark, 'has-background': !!settingsStore.config.chat_background }"
            :style="mainAreaStyle"
          >
            <Sidebar :collapsed="sidebarCollapsed" @toggle="sidebarCollapsed = !sidebarCollapsed" />
            <div class="main-area" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
              <AppHeader
                :switch-duration="switchDuration"
                :switch-stage-text="switchStageText"
                :is-maximized="isMaximized"
                @toggle-sidebar="sidebarCollapsed = !sidebarCollapsed"
                @toggle-console="toggleConsole"
                @minimize="handleMinimize"
                @toggle-maximize="handleToggleMaximize"
                @close="handleClose"
                @header-double-click="handleHeaderDoubleClick"
              />
              <router-view v-slot="{ Component }">
                <Transition name="route-fade" mode="out-in">
                  <component :is="Component" />
                </Transition>
              </router-view>
            </div>
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
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { darkTheme, zhCN, dateZhCN } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import Sidebar from './components/Sidebar.vue'
import SplashScreen from './components/ui/SplashScreen.vue'
import ServerConsole from './components/ServerConsole.vue'
import AppHeader from './components/AppHeader.vue'
import ModelSwitchOverlay from './components/ModelSwitchOverlay.vue'
import ExitOverlay from './components/ExitOverlay.vue'
// Task 21：抽取 mainAreaStyle 背景图逻辑为纯函数，便于单元测试双主题支持
import { buildBackgroundStyle } from './utils/backgroundStyle'
import { useSettingsStore } from './stores/settings'
import { useThemeStore } from './stores/theme'
// Naive UI 全局主题覆盖：让所有组件使用项目 GitHub 蓝配色而非默认绿色（Task 2）
import { useThemeOverrides } from './composables/useThemeOverrides'
// 任务 9：抽取模型切换/窗口控制/生命周期到 composable
import { useModelSwitch } from './composables/useModelSwitch'
import { useWindowControls } from './composables/useWindowControls'
import { useAppLifecycle } from './composables/useAppLifecycle'

// ----- Store / 主题 -----
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()
// themeOverrides 是 ComputedRef<GlobalThemeOverrides>，会随 isDark 自动切换
// n-config-provider 接受 ref，会自动 unwrap，模板里直接传 ref 即可
const themeOverrides = useThemeOverrides()

const isDark = computed(() => themeStore.isDark)
const sidebarCollapsed = defineModel<boolean>('sidebarCollapsed', { default: false })

const mainAreaStyle = computed(() => {
  // 双主题都支持背景图：逻辑抽取到 utils/backgroundStyle.ts（Task 21）
  // isDark 不再作为限制条件，亮色与深色都会注入 --chat-background 变量
  return buildBackgroundStyle(
    settingsStore.config.chat_background ?? '',
    settingsStore.config.chat_background_opacity
  )
})

// ----- 调用三个 composable（任务 9：抽取模型切换/窗口控制/生命周期）-----
// 说明：useModelSwitch / useWindowControls / useAppLifecycle 均非单例，
//       在 App.vue 调用一次后，将子组件需要的数据通过 props 传入、事件通过 emit 传出，
//       避免子组件重复调用导致 wails 事件监听重复注册。
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

const { showSplash, splashStage, splashModelName, splashProgress, showExitOverlay, exitProgress } =
  useAppLifecycle()

// ----- 服务器控制台 -----
const consoleRef = ref()
const toggleConsole = () => {
  consoleRef.value?.toggle()
}
</script>

<style scoped>
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
