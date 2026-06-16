import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import {
    wails,
    type Config,
    DEFAULT_CONFIG,
    type ServerStatus,
    type ModelCapabilities,
    type SwitchResult,
    type SearchAPIKeys,
} from '../services/wails'
import { formatModelName } from '../utils/model'
import type { ModelSwitchState, SwitchProgressStage, SwitchProgress } from '../types/settings'
import { SWITCH_TIMING } from '../types/settings'

export type { SwitchProgressStage, SwitchProgress } from '../types/settings'

export function shouldReloadConfigOnModelChange(
    oldModel: string,
    newModel: string
): boolean {
    return !!newModel && oldModel !== newModel
}

export function matchModelRef<T>(
    modelName: string,
    refs: Record<string, T>
): T | null {
    const lower = (modelName || '').toLowerCase()
    if (!lower) return null
    const sorted = Object.entries(refs).sort((a, b) => b[0].length - a[0].length)
    for (const [key, ref] of sorted) {
        const keywords = key.split('-')
        if (keywords.every(kw => lower.includes(kw))) {
            return ref
        }
    }
    return null
}

export function shouldKeepSwitchingVisible(
    switchStartedAt: number,
    now: number,
    minDurationMs: number
): boolean {
    if (switchStartedAt === 0) return true
    return (now - switchStartedAt) < minDurationMs
}

export const useSettingsStore = defineStore('settings', () => {
    // ----- 基础配置 -----
    const config = ref<Config>({ ...DEFAULT_CONFIG })
    const searchEnabled = ref(false)
    const thinkingEnabled = ref(true)
    const thinkingSoftSwitch = ref<'auto' | 'think' | 'no_think'>('auto')
    const searchAPIKeys = ref<SearchAPIKeys>({
        ollama_api_key: '',
        tavily_api_key: '',
        github_api_key: '',
    })
    const serverStatus = ref<ServerStatus>({ running: false })
    const modelCapabilities = ref<ModelCapabilities>({
        image_input: false,
        audio_input: false,
        video_input: false,
        text_input: true,
        reasoning: false,
        mmproj_loaded: false,
        has_mtp: false,
        thinking_mode: 'none',
        soft_switch_support: false,
        n_params: 0,
    })
    const currentModel = ref('')
    const modelLoadError = ref('')
    const hasEverBeenReady = ref(false)

    // ----- 模型切换状态机（单一 source of truth） -----
    const switchState = ref<ModelSwitchState>({ phase: 'idle' })

    // ----- 状态机派生的兼容 API（不破坏外部代码） -----
    const isModelSwitching = computed(() =>
        switchState.value.phase === 'first_load' || switchState.value.phase === 'switching'
    )
    const switchingModelDisplay = computed(() => {
        const s = switchState.value
        if (s.phase === 'first_load' || s.phase === 'switching' || s.phase === 'ready_after_switch' || s.phase === 'timeout') {
            return formatModelName(s.targetModel).display
        }
        if (s.phase === 'failed') {
            return formatModelName(s.targetModel).display
        }
        return ''
    })
    const switchStartedAt = computed(() => {
        const s = switchState.value
        if ('startedAt' in s) return s.startedAt
        return 0
    })
    const previousModelBeforeSwitch = computed(() => {
        const s = switchState.value
        return s.phase === 'switching' ? s.previousModel : ''
    })
    const modelLoadFailed = computed(() => switchState.value.phase === 'failed')

    /** SwitchProgress（兼容旧接口） */
    const switchProgress = computed<SwitchProgress>(() => {
        const s = switchState.value
        const base: SwitchProgress = {
            stage: 'idle',
            targetModel: '',
            errorMessage: '',
            startTime: 0,
            endTime: 0,
            rolledBack: false,
        }
        if (s.phase === 'idle') return base
        if (s.phase === 'first_load' || s.phase === 'switching') {
            return {
                ...base,
                stage: 'preparing',
                targetModel: formatModelName(s.targetModel).display,
                startTime: s.startedAt,
            }
        }
        if (s.phase === 'ready_after_switch') {
            return {
                ...base,
                stage: 'done',
                targetModel: formatModelName(s.targetModel).display,
                startTime: s.startedAt,
                endTime: Date.now(),
            }
        }
        if (s.phase === 'failed') {
            return {
                ...base,
                stage: s.rolledBack ? 'rolling_back' : 'failed',
                targetModel: formatModelName(s.targetModel).display,
                errorMessage: s.error,
                startTime: s.startedAt,
                endTime: Date.now(),
                rolledBack: s.rolledBack,
            }
        }
        if (s.phase === 'timeout') {
            return {
                ...base,
                stage: 'failed',
                targetModel: formatModelName(s.targetModel).display,
                errorMessage: '切换超时',
                startTime: s.startedAt,
                endTime: Date.now(),
                rolledBack: false,
            }
        }
        return base
    })

    const isFirstLoad = computed(() => {
        return !hasEverBeenReady.value && !serverStatus.value.model_ready && !serverStatus.value.error && !isModelSwitching.value
    })

    // ----- 状态机转换函数 -----

    /** 启动切换（手动） */
    function startSwitch(modelName: string) {
        switchState.value = {
            phase: 'switching',
            startedAt: Date.now(),
            targetModel: modelName,
            previousModel: currentModel.value,
        }
    }

    /** 上报后端进度 */
    function reportProgress(stage: SwitchProgressStage) {
        // 状态机单向流转：终态不接受进度事件
        if (switchState.value.phase === 'idle' || switchState.value.phase === 'ready_after_switch'
            || switchState.value.phase === 'failed' || switchState.value.phase === 'timeout') {
            return
        }
        // 此处仅保留 targetModel 不变,stage 由 store 在收到状态时实时映射
        // 当前用 switchProgress 反映 stage,但底层状态不变
        void stage
    }

    /** 切换成功 */
    function finishSuccess(model: string) {
        switchState.value = {
            phase: 'ready_after_switch',
            startedAt: switchState.value.phase !== 'idle' && 'startedAt' in switchState.value
                ? switchState.value.startedAt
                : Date.now(),
            targetModel: model,
        }
    }

    /** 切换失败 */
    function finishFailure(err: string, prev: string, rolledBack: boolean, rbSuccess: boolean) {
        const s = switchState.value
        const startedAt = 'startedAt' in s ? s.startedAt : Date.now()
        const targetModel = s.phase === 'idle' ? '' : s.targetModel
        switchState.value = {
            phase: 'failed',
            error: err,
            targetModel: targetModel || '',
            rolledBack,
            rollbackSuccess: rbSuccess,
            startedAt,
        }
        if (prev && !rolledBack) {
            currentModel.value = prev
        }
    }

    /** 切换超时 */
    function finishTimeout() {
        const s = switchState.value
        if (s.phase === 'idle') return
        const targetModel = 'targetModel' in s ? s.targetModel : ''
        const startedAt = 'startedAt' in s ? s.startedAt : Date.now()
        switchState.value = { phase: 'timeout', targetModel, startedAt }
    }

    /** 主动重置（用户主动关闭遮罩等） */
    function reset() {
        switchState.value = { phase: 'idle' }
    }

    /** 收到 server:status 事件时的状态机处理（首次加载完成） */
    function onServerReady() {
        const s = switchState.value
        if (s.phase === 'first_load') {
            switchState.value = { phase: 'idle' }
        }
    }

    /** 首次启动时记录 "first_load" 阶段 */
    function beginFirstLoad(targetModel: string) {
        if (switchState.value.phase === 'idle') {
            switchState.value = {
                phase: 'first_load',
                startedAt: Date.now(),
                targetModel,
            }
        }
    }

    // ----- 副作用：状态机驱动的定时器（集中管理,自动清理） -----
    let pendingTransitions: ReturnType<typeof setTimeout>[] = []

    function clearAllTimers() {
        for (const t of pendingTransitions) clearTimeout(t)
        pendingTransitions = []
    }

    watch(switchState, (newState) => {
        clearAllTimers()
        // 在 ready_after_switch 后 800ms 自动回到 idle
        if (newState.phase === 'ready_after_switch') {
            pendingTransitions.push(setTimeout(() => {
                if (switchState.value.phase === 'ready_after_switch') {
                    switchState.value = { phase: 'idle' }
                }
            }, 800))
        }
        // failed 后 5s 自动回到 idle
        if (newState.phase === 'failed') {
            pendingTransitions.push(setTimeout(() => {
                if (switchState.value.phase === 'failed') {
                    switchState.value = { phase: 'idle' }
                }
            }, 5000))
        }
        // first_load 长时间未完成 → 视为失败
        if (newState.phase === 'first_load') {
            pendingTransitions.push(setTimeout(() => {
                if (switchState.value.phase === 'first_load') {
                    finishTimeout()
                }
            }, SWITCH_TIMING.SERVER_POLL_TIMEOUT_MS))
        }
    })

    // ----- 业务函数 -----

    async function loadConfig() {
        try {
            config.value = await wails.getConfig()
            searchEnabled.value = config.value.search_enabled ?? false
            thinkingEnabled.value = config.value.thinking_enabled ?? true
            thinkingSoftSwitch.value = config.value.thinking_soft_switch || 'auto'
            if (!config.value.thinking_enabled && thinkingSoftSwitch.value === 'auto') {
                thinkingSoftSwitch.value = 'no_think'
            }
        } catch (e) {
            console.error('加载配置失败:', e)
        }
    }

    async function updateConfig(cfg: Config) {
        try {
            await wails.updateConfig(cfg)
            await loadConfig()
        } catch (e) {
            console.error('更新配置失败:', e)
        }
    }

    async function checkServerStatus() {
        try {
            const status = await wails.getServerStatus()
            serverStatus.value = status
            if (status.model_ready) {
                hasEverBeenReady.value = true
                onServerReady()
            }
            if (status.capabilities) {
                modelCapabilities.value = status.capabilities
            }
            if (status.current_model) {
                currentModel.value = status.current_model
            }
        } catch (e) {
            console.error('获取服务器状态失败:', e)
        }
    }

    async function loadSearchAPIKeys() {
        try {
            const keys = await wails.getSearchAPIKeys()
            searchAPIKeys.value = keys
        } catch (e) {
            console.error('Failed to load search API keys:', e)
        }
    }

    async function saveSearchAPIKeys(keys: Partial<SearchAPIKeys>) {
        try {
            if (Object.keys(keys).length === 0) return
            const fullKeys: SearchAPIKeys = {
                ollama_api_key: keys.ollama_api_key ?? '',
                tavily_api_key: keys.tavily_api_key ?? '',
                github_api_key: keys.github_api_key ?? '',
            }
            await wails.setSearchAPIKeys(fullKeys)
            searchAPIKeys.value = { ...searchAPIKeys.value, ...keys }
        } catch (e) {
            console.error('Failed to save search API keys:', e)
        }
    }

    async function hasServerAPIKey(): Promise<boolean> {
        try {
            return await wails.hasServerAPIKey()
        } catch (e) {
            console.error('Failed to check server API key:', e)
            return false
        }
    }

    async function saveServerAPIKey(key: string) {
        try {
            await wails.setServerAPIKey(key)
        } catch (e) {
            console.error('Failed to save server API key:', e)
        }
    }

    async function toggleSearch() {
        const oldValue = searchEnabled.value
        searchEnabled.value = !searchEnabled.value
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.search_enabled = searchEnabled.value
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
            searchEnabled.value = oldValue
            console.error('保存搜索设置失败:', e)
        }
    }

    async function cycleThinkingMode() {
        const next: Record<string, 'auto' | 'think' | 'no_think'> = {
            'auto': 'think',
            'think': 'no_think',
            'no_think': 'auto',
        }
        const oldValue = thinkingSoftSwitch.value
        const nextMode = next[oldValue] || 'auto'
        thinkingSoftSwitch.value = nextMode
        thinkingEnabled.value = nextMode !== 'no_think'
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.thinking_enabled = thinkingEnabled.value
            fullConfig.thinking_soft_switch = nextMode
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
            thinkingSoftSwitch.value = oldValue
            thinkingEnabled.value = oldValue !== 'no_think'
            console.error('保存思考设置失败:', e)
        }
    }

    /**
     * 处理后端 switchModel 的结果
     * 内部根据成功/失败转换状态机
     */
    function handleSwitchResult(result: SwitchResult): void {
        if (result.success) {
            if (result.current_model) {
                currentModel.value = result.current_model
            }
            if (result.capabilities) {
                modelCapabilities.value = result.capabilities
            }
            finishSuccess(result.current_model || currentModel.value)
        } else {
            finishFailure(
                result.error || '模型加载失败',
                result.previous_model || currentModel.value,
                result.rolled_back || false,
                result.rollback_success || false
            )
        }
    }

    async function switchModel(modelName: string, previousModel: string): Promise<SwitchResult> {
        startSwitch(modelName)
        try {
            const result = await wails.switchModel(modelName)
            handleSwitchResult(result)
            if (result.success) {
                await loadConfig()
            }
            return result
        } catch (e) {
            finishFailure(String(e), previousModel, false, false)
            throw e
        }
    }

    function initStatusListener() {
        wails.onServerStatus((status: ServerStatus) => {
            serverStatus.value = status
            if (status.capabilities) {
                modelCapabilities.value = status.capabilities
            }
            if (status.model_ready) {
                hasEverBeenReady.value = true
                onServerReady()
            }
            if (status.current_model && !isModelSwitching.value) {
                const oldModel = currentModel.value
                currentModel.value = status.current_model
                if (shouldReloadConfigOnModelChange(oldModel, status.current_model)) {
                    loadConfig()
                }
            }
        })
        checkServerStatus()
    }

    function cleanupStatusListener() {
        wails.offServerStatus()
    }

    function initSwitchProgressListener() {
        wails.onSwitchProgress((progress) => {
            // 仅在 idle 接受进度事件,首次加载时记录 first_load
            if (switchState.value.phase === 'idle') {
                if (progress.stage === 'loading' || progress.stage === 'preparing' || progress.stage === 'waiting') {
                    beginFirstLoad(progress.targetModel || currentModel.value)
                }
            }
            reportProgress(progress.stage as SwitchProgressStage)
        })
    }

    function cleanupSwitchProgressListener() {
        wails.offSwitchProgress()
    }

    function resetSwitchProgress() {
        clearAllTimers()
        reset()
    }

    return {
        // 基础状态
        config,
        searchEnabled,
        thinkingEnabled,
        thinkingSoftSwitch,
        searchAPIKeys,
        serverStatus,
        modelCapabilities,
        currentModel,
        modelLoadError,
        hasEverBeenReady,
        // 兼容旧 API
        modelLoadFailed,
        isModelSwitching,
        switchingModelDisplay,
        switchStartedAt,
        previousModelBeforeSwitch,
        switchProgress,
        isFirstLoad,
        // 业务
        loadConfig,
        updateConfig,
        loadSearchAPIKeys,
        saveSearchAPIKeys,
        hasServerAPIKey,
        saveServerAPIKey,
        toggleSearch,
        cycleThinkingMode,
        toggleThinking: cycleThinkingMode,
        switchModel,
        checkServerStatus,
        initStatusListener,
        cleanupStatusListener,
        initSwitchProgressListener,
        cleanupSwitchProgressListener,
        resetSwitchProgress,
        // 状态机内部 API（供测试 / 高级用例使用）
        switchState,
    }
})
