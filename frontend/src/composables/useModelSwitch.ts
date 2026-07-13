/**
 * useModelSwitch - 模型切换 overlay UI 状态 composable
 *
 * 从 App.vue 抽取的模型切换 overlay 相关状态、computed 与事件监听。
 * 仅负责 UI 展示层状态（进度阶段、耗时、阶段文字、是否显示遮罩等），
 * 底层状态机（switchState）仍由 settingsStore 统一管理。
 *
 * 设计原则：
 *  - 纯搬迁，不优化不重构，逻辑与原 App.vue 完全一致
 *  - 依赖注入：通过 useSettingsStore 获取底层状态，不在 composable 内部维护副本
 *  - 事件监听在 onMounted 注册、onUnmounted 清理，避免内存泄漏
 *
 * 生活类比：就像把厨房里散放的调料（状态、计时器、事件监听）收进一个统一的
 * 调料盒（composable）里，原来菜谱（App.vue）的步骤不变，只是取用更方便。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useSettingsStore } from '../stores/settings'
import { wails } from '../services/wails'
import { formatModelName } from '../utils/model'
import type { SwitchProgressStage } from '../types/settings'
// F-1.14：stageMap 抽取为常量，与 useAppLifecycle.ts 共享
import { STAGE_PERCENT_MAP } from './stageMap'

export function useModelSwitch() {
    const settingsStore = useSettingsStore()

    // ----- 从 store 派生的兼容 computed（原 App.vue 中的 thin wrapper）-----
    // 这些是原 App.vue 中直接转发 store 的 computed，保留以维持依赖链一致
    const isModelSwitching = computed(() => settingsStore.isModelSwitching)
    const switchingModelDisplay = computed(() => settingsStore.switchingModelDisplay)
    const switchStartedAt = computed(() => settingsStore.switchStartedAt)
    const previousModelBeforeSwitch = computed(() => settingsStore.previousModelBeforeSwitch)

    // ----- 模型切换 overlay 状态 -----

    /**
     * 切换进度阶段（idle/preparing/loading/waiting/detecting/done/failed/rolling_back）
     * 直接映射 store.switchProgress.stage
     */
    const switchProgressStage = computed<SwitchProgressStage>(
        () => settingsStore.switchProgress.stage
    )

    /**
     * 正在切换的模型显示名（格式化后的友好名称）
     * 优先用 store 的 switchingModelDisplay，其次用 serverStatus.switching_to
     */
    const switchingModelName = computed(() => {
        if (switchingModelDisplay.value) return switchingModelDisplay.value
        if (settingsStore.serverStatus.switching_to) {
            return formatModelName(settingsStore.serverStatus.switching_to).display
        }
        return ''
    })

    /**
     * 正在切换的模型原始 ID（路径/名称，未格式化）
     * 注：原 App.vue 中无此独立变量，这里按 SubTask 6.3 返回值要求补充派生，
     * 数据完全来自 store.switchState.targetModel，不引入新逻辑。
     */
    const switchingModelId = computed(() => {
        const s = settingsStore.switchState
        if ('targetModel' in s) return s.targetModel
        if (settingsStore.serverStatus.switching_to) {
            return settingsStore.serverStatus.switching_to
        }
        return ''
    })

    /**
     * 切换耗时显示文本（如 " · 已等待 5s"）
     * 由 switchDurationTimer 每秒更新
     */
    const switchDuration = ref('')

    /**
     * 切换阶段文字（用于 overlay 与状态栏展示）
     * - 切换进行中但后端未推送 stage 时显示 "正在切换模型..."
     * - 否则按 stage 映射到具体文字
     */
    const switchStageText = computed(() => {
        // 切换进行中（前端发起，后端还未推送 stage）
        if (isModelSwitching.value && settingsStore.switchProgress.stage === 'idle') {
            return '正在切换模型...'
        }
        const stage = settingsStore.switchProgress.stage
        const texts: Record<string, string> = {
            'preparing': '准备切换模型...',
            'loading': '加载模型中...',
            'waiting': '初始化模型...',
            'detecting': '检测模型能力...',
            'done': '加载完成',
            'failed': '模型加载失败',
            'vram-warning': 'VRAM 不足警告，可能影响性能...',
            'spec-warning': '推测解码兼容性警告...',
        }
        return texts[stage] || '加载中...'
    })

    /**
     * 是否显示切换 overlay
     * 合并触发条件：切换进行中（isModelSwitching）或切换后反馈（stage 非 idle）都显示
     * 这样 MessageList.vue 不再需要自己的切换 overlay，避免重复
     */
    const showSwitchOverlay = computed(
        () =>
            isModelSwitching.value ||
            (settingsStore.switchProgress.stage !== 'idle' && settingsStore.hasEverBeenReady)
    )

    /**
     * overlay 上显示的模型名（优先 switchProgress.targetModel，其次 switchingModelDisplay）
     */
    const overlayModelName = computed(() => {
        if (settingsStore.switchProgress.targetModel) {
            return settingsStore.switchProgress.targetModel
        }
        if (switchingModelDisplay.value) return switchingModelDisplay.value
        return ''
    })

    // ----- 切换阶段指示器（3 阶段，与原 MessageList 逻辑一致）-----
    const switchStages = ['准备切换', '加载新模型', '初始化完成'] as const

    /**
     * 获取当前切换阶段索引（0/1/2）
     * - 切换进行中但后端未推送 stage 时返回 0
     * - preparing → 0, loading → 1, done → 2
     */
    function getSwitchStageIndex(): number {
        // 切换进行中但后端未推送 stage 时，显示第一阶段
        if (isModelSwitching.value && settingsStore.switchProgress.stage === 'idle') return 0
        const stage = settingsStore.switchProgress.stage
        switch (stage) {
            case 'preparing': return 0
            case 'loading': return 1
            case 'done': return 2
            default: return 0
        }
    }

    /**
     * overlay 进度条百分比（复用 splashProgress 逻辑）
     * 优先用后端推送的真实加载进度，无进度时用阶段映射兜底
     */
    const switchProgressPercent = computed(() => {
        const modelLoadProgress = settingsStore.modelLoadProgress
        if (modelLoadProgress && modelLoadProgress.status === 'loading') {
            return Math.max(5, Math.min(99, Math.round(modelLoadProgress.progress)))
        }
        // 无真实进度时使用粗略阶段映射（仅作为兜底）
        // F-1.14：STAGE_PERCENT_MAP 抽取到 ./stageMap，与 useAppLifecycle 共享
        return STAGE_PERCENT_MAP[settingsStore.switchProgress.stage] ?? 0
    })

    // ----- 切换耗时计时器（每秒更新 switchDuration）-----
    let switchDurationTimer: ReturnType<typeof setInterval> | null = null

    /**
     * 停止耗时计时器并清空 switchDuration
     */
    function stopSwitchDurationTimer() {
        if (switchDurationTimer) {
            clearInterval(switchDurationTimer)
            switchDurationTimer = null
        }
        switchDuration.value = ''
    }

    // 阶段变化时启动/停止耗时计时器
    watch(switchProgressStage, (stage) => {
        if (stage !== 'idle') {
            stopSwitchDurationTimer()
            switchDurationTimer = setInterval(() => {
                const startTime = settingsStore.switchStartedAt || settingsStore.switchProgress.startTime
                if (startTime > 0) {
                    const elapsed = Math.floor((Date.now() - startTime) / 1000)
                    if (elapsed > 0) {
                        switchDuration.value = ` · 已等待 ${elapsed}s`
                    }
                }
            }, 1000)
        } else {
            stopSwitchDurationTimer()
        }
    })

    // ----- 事件监听（onMounted 注册，onUnmounted 清理）-----
    // 监听 server:switchProgress 事件（切换进度）与 modelLoadProgress 事件（模型加载进度）
    // F-1.12：register 函数返回 unsubscribe，收集到 unsubscribers 数组批量清理
    const unsubscribers: Array<() => void> = []
    onMounted(() => {
        unsubscribers.push(settingsStore.registerSwitchProgressListener())
        unsubscribers.push(settingsStore.registerModelLoadProgressListener())
    })

    onUnmounted(() => {
        // 清理计时器
        stopSwitchDurationTimer()
        // 批量清理事件监听（含 wails.offSwitchProgress 等价逻辑）
        while (unsubscribers.length > 0) {
            const unsubscribe = unsubscribers.pop()
            try {
                unsubscribe?.()
            } catch (e) {
                console.error('[useModelSwitch] unsubscribe failed:', e)
            }
        }
    })

    return {
        // 状态 / computed
        switchProgressStage,          // ComputedRef<SwitchProgressStage>
        switchingModelName,           // ComputedRef<string>
        switchingModelId,             // ComputedRef<string>
        switchDuration,               // Ref<string>
        switchStageText,              // ComputedRef<string>
        showSwitchOverlay,            // ComputedRef<boolean>
        overlayModelName,             // ComputedRef<string>
        switchProgressPercent,        // ComputedRef<number>
        switchStages,                 // readonly tuple
        // store 派生（供调用方按需使用）
        isModelSwitching,             // ComputedRef<boolean>
        switchingModelDisplay,        // ComputedRef<string>
        switchStartedAt,              // ComputedRef<number>
        previousModelBeforeSwitch,    // ComputedRef<string>
        // 方法
        getSwitchStageIndex,          // () => number
        stopSwitchDurationTimer,      // () => void
    }
}
