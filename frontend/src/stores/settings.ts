import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { createDiscreteApi } from 'naive-ui'
import {
    wails,
    type Config,
    DEFAULT_CONFIG,
    type ServerStatus,
    type ModelCapabilities,
    type SwitchResult,
    type SearchAPIKeys,
    type ModelLoadProgressEvent,
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

export const useSettingsStore = defineStore('settings', () => {
    // 用于在 store 中显示 naive-ui 消息（store 不在 NMessageProvider 内部，需用 discrete API）
    const { message: discreteMessage } = createDiscreteApi(['message'])

    // ----- 基础配置 -----
    const config = ref<Config>({ ...DEFAULT_CONFIG })
    const searchMode = ref<'off' | 'auto' | 'on'>('off')
    const thinkingEnabled = computed(() => config.value?.reasoning !== 'off')
    const thinkingSoftSwitch = computed<'auto' | 'think' | 'no_think'>(() => {
        const r = config.value?.reasoning
        if (r === 'on') return 'think'
        if (r === 'off') return 'no_think'
        return 'auto'
    })
    const searchAPIKeys = ref<SearchAPIKeys>({
        ollama_api_key: '',
        tavily_api_key: '',
        ollama_api_key_set: false,
        tavily_api_key_set: false,
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
        tool_call_support: false,
    })
    const currentModel = ref('')
    const modelLoadError = ref('')
    const hasEverBeenReady = ref(false)

    // ----- 模型加载进度（后端 modelLoadProgress 事件） -----
    const modelLoadProgress = ref<ModelLoadProgressEvent | null>(null)

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

    // ----- 启动兜底：周期性状态轮询 + 看门狗（防止事件监听器注册晚于后端事件发射导致无限转圈） -----
    let startupPollingTimer: ReturnType<typeof setInterval> | null = null
    let startupWatchdogTimer: ReturnType<typeof setTimeout> | null = null
    let receivedAnyStatusEvent = false

    /** 启动周期性状态轮询（每 3s 兜底检查服务器状态） */
    function startStartupPolling() {
        // 监听器重复初始化时先清除旧定时器
        stopStartupPolling()
        startupPollingTimer = setInterval(() => {
            checkServerStatus()
        }, 3000)
    }

    /** 停止周期性状态轮询 + 清除看门狗定时器 */
    function stopStartupPolling() {
        if (startupPollingTimer) {
            clearInterval(startupPollingTimer)
            startupPollingTimer = null
        }
        if (startupWatchdogTimer) {
            clearTimeout(startupWatchdogTimer)
            startupWatchdogTimer = null
        }
    }

    /** 启动看门狗：60s 内未收到任何状态事件则主动轮询，轮询失败则标记 failed */
    function startStartupWatchdog() {
        // 重复初始化时先清除旧定时器
        if (startupWatchdogTimer) {
            clearTimeout(startupWatchdogTimer)
            startupWatchdogTimer = null
        }
        startupWatchdogTimer = setTimeout(async () => {
            if (!receivedAnyStatusEvent && !hasEverBeenReady.value) {
                console.warn('[startup] watchdog: no status event received in 60s, polling manually')
                try {
                    await checkServerStatus()
                } catch (e) {
                    console.error('[startup] watchdog: manual polling failed', e)
                    finishFailure(String(e) || '启动看门狗触发失败', currentModel.value, false, false)
                }
            }
        }, 60000)
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
        // switching 阶段超时保护，避免后端卡死导致前端无限等待
        if (newState.phase === 'switching') {
            pendingTransitions.push(setTimeout(() => {
                if (switchState.value.phase === 'switching') {
                    finishTimeout()
                }
            }, SWITCH_TIMING.SERVER_POLL_TIMEOUT_MS))
        }
    })

    // 启动兜底：当已就绪或进入终态时停止周期性轮询
    watch(hasEverBeenReady, (ready) => {
        if (ready) {
            stopStartupPolling()
        }
    })
    watch(() => switchState.value.phase, (phase) => {
        // phase 进入 done(ready_after_switch)/failed/timeout 时停止轮询
        if (phase === 'ready_after_switch' || phase === 'failed' || phase === 'timeout') {
            stopStartupPolling()
        }
    })

    // 监听模型能力变化，仅当用户未手动设置 reasoning 时自动选择最优值
    watch(modelCapabilities, (newCaps) => {
        if (!config.value) return
        if (!config.value.reasoning && newCaps.reasoning) {
            config.value.reasoning = 'auto'
        }
    }, { deep: true })

    // ----- 业务函数 -----

    async function loadConfig() {
        try {
            config.value = await wails.getConfig()
            searchMode.value = (config.value.search_mode as 'off' | 'auto' | 'on') ?? 'off'
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
            // 后端不再返回实际密钥，仅返回设置状态
            searchAPIKeys.value = {
                ...searchAPIKeys.value,
                ollama_api_key_set: keys.ollama_api_key_set ?? false,
                tavily_api_key_set: keys.tavily_api_key_set ?? false,
            }
        } catch (e) {
            console.error('Failed to load search API keys:', e)
        }
    }

    async function saveSearchAPIKeys(keys: Partial<SearchAPIKeys>) {
        // 仅保存用户实际输入的新密钥，空字符串表示未修改
        const fullKeys: SearchAPIKeys = {
            ollama_api_key: keys.ollama_api_key ?? '',
            tavily_api_key: keys.tavily_api_key ?? '',
            ollama_api_key_set: false,
            tavily_api_key_set: false,
        }
        if (!fullKeys.ollama_api_key && !fullKeys.tavily_api_key) return
        // 不吞掉错误，让调用方 catch 后给用户视觉反馈
        await wails.setSearchAPIKeys(fullKeys)
        // 保存成功后刷新设置状态
        await loadSearchAPIKeys()
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
        // 不吞掉错误，让调用方 catch 后给用户视觉反馈
        await wails.setServerAPIKey(key)
    }

    async function cycleSearchMode() {
        const next: Record<string, 'off' | 'auto' | 'on'> = {
            'off': 'auto',
            'auto': 'on',
            'on': 'off',
        }
        const oldValue = searchMode.value
        const nextMode = next[oldValue] || 'off'
        searchMode.value = nextMode
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.search_mode = nextMode
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
            searchMode.value = oldValue
            console.error('保存搜索设置失败:', e)
        }
    }

    async function setSearchMode(mode: 'off' | 'auto' | 'on') {
        const oldValue = searchMode.value
        searchMode.value = mode
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.search_mode = mode
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
            searchMode.value = oldValue
            console.error('保存搜索设置失败:', e)
        }
    }

    async function cycleThinkingMode() {
        const next: Record<string, 'auto' | 'on' | 'off'> = {
            'auto': 'on',
            'on': 'off',
            'off': 'auto',
        }
        const oldValue = config.value?.reasoning ?? 'off'
        const nextMode = next[oldValue] || 'auto'
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.reasoning = nextMode
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
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
            receivedAnyStatusEvent = true
            serverStatus.value = status
            // 后端错误事件触发 failed 状态（仅在 first_load/switching 阶段，避免 idle 时误标记失败）
            if (status.running === false && status.error) {
                const phase = switchState.value.phase
                if (phase === 'first_load' || phase === 'switching') {
                    const prev = phase === 'switching' && 'previousModel' in switchState.value
                        ? switchState.value.previousModel
                        : currentModel.value
                    finishFailure(status.error, prev, false, false)
                }
            }
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
        // 启动周期性轮询兜底，防止事件竞态导致无限转圈
        startStartupPolling()
        // 启动看门狗：60s 兜底保护，防止事件监听完全失效
        startStartupWatchdog()
    }

    function cleanupStatusListener() {
        wails.offServerStatus()
        // 清理所有相关定时器，避免离开设置页后定时器残留
        stopStartupPolling()       // 清理 startupPollingTimer 与 startupWatchdogTimer
        clearAllTimers()           // 清理 pendingTransitions 中所有定时器
    }

    function initSwitchProgressListener() {
        wails.onSwitchProgress((progress) => {
            // 仅在 idle 接受进度事件,首次加载时记录 first_load
            if (switchState.value.phase === 'idle') {
                beginFirstLoad(progress.targetModel || currentModel.value)
            }
            reportProgress(progress.stage as SwitchProgressStage)
        })
    }

    function cleanupSwitchProgressListener() {
        wails.offSwitchProgress()
    }

    function initMmprojUnavailableListener() {
        wails.onMmprojUnavailable(() => {
            discreteMessage.warning('多模态功能已降级为纯文本模式（mmproj 不兼容）', { duration: 5000 })
        })
    }

    function cleanupMmprojUnavailableListener() {
        wails.offMmprojUnavailable()
    }

    function initModelLoadProgressListener() {
        wails.onModelLoadProgress((progress: ModelLoadProgressEvent) => {
            if (progress.status === 'running') {
                // 模型加载完成，清除进度状态
                modelLoadProgress.value = null
            } else {
                modelLoadProgress.value = progress
            }
        })
    }

    function cleanupModelLoadProgressListener() {
        wails.offModelLoadProgress()
    }

    function resetSwitchProgress() {
        clearAllTimers()
        reset()
    }

    return {
        // 基础状态
        config,
        searchMode,
        thinkingEnabled,
        thinkingSoftSwitch,
        searchAPIKeys,
        serverStatus,
        modelCapabilities,
        currentModel,
        modelLoadError,
        hasEverBeenReady,
        modelLoadProgress,
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
        toggleSearch: cycleSearchMode,
        cycleSearchMode,
        setSearchMode,
        cycleThinkingMode,
        toggleThinking: cycleThinkingMode,
        switchModel,
        checkServerStatus,
        initStatusListener,
        cleanupStatusListener,
        initSwitchProgressListener,
        cleanupSwitchProgressListener,
        initMmprojUnavailableListener,
        cleanupMmprojUnavailableListener,
        initModelLoadProgressListener,
        cleanupModelLoadProgressListener,
        resetSwitchProgress,
        // 状态机内部 API（供测试 / 高级用例使用）
        switchState,
    }
})
