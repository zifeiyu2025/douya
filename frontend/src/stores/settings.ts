import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { wails, type Config, DEFAULT_CONFIG, type ServerStatus, type ModelCapabilities, type SwitchResult, type SearchAPIKeys } from '../services/wails'
import { formatModelName } from '../utils/model'

/**
 * Switch progress stages for UI display
 */
export type SwitchProgressStage = 'idle' | 'preparing' | 'loading' | 'waiting' | 'detecting' | 'done' | 'failed' | 'rolling_back'

/**
 * Switch progress state for detailed UI feedback
 */
export interface SwitchProgress {
    stage: SwitchProgressStage
    targetModel: string
    errorMessage: string
    startTime: number
    endTime: number
    rolledBack: boolean
}

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
    const config = ref<Config>({ ...DEFAULT_CONFIG })
    const searchEnabled = ref(false)
    const thinkingEnabled = ref(true)
    const thinkingSoftSwitch = ref<'auto' | 'think' | 'no_think'>('auto')
    const searchAPIKeys = ref<SearchAPIKeys>({
        ollama_api_key: '',
        tavily_api_key: '',
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
    const isModelSwitching = ref(false)
    const switchingModelDisplay = ref('')
    const switchStartedAt = ref(0)
    const previousModelBeforeSwitch = ref('')
    const modelLoadFailed = ref(false)
    const hasEverBeenReady = ref(false)


    // Enhanced switch progress state
    const switchProgress = ref<SwitchProgress>({
        stage: 'idle',
        targetModel: '',
        errorMessage: '',
        startTime: 0,
        endTime: 0,
        rolledBack: false,
    })

    let switchDoneTimer: ReturnType<typeof setTimeout> | null = null
    let switchTimeoutTimer: ReturnType<typeof setTimeout> | null = null


    async function loadConfig() {
        try {
            config.value = await wails.getConfig()
            searchEnabled.value = config.value.search_enabled ?? false
            thinkingEnabled.value = config.value.thinking_enabled ?? true
            thinkingSoftSwitch.value = config.value.thinking_soft_switch || 'auto'
            // 向后兼容：旧版 thinking_enabled=false 等效于 no_think
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
            // 使用 model_ready（模型真正加载完成）而非 running（服务器进程在运行）
            if (status.model_ready) {
                hasEverBeenReady.value = true
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

    /**
     * Reset switch progress to idle state
     */
    function resetSwitchProgress() {
        if (switchDoneTimer !== null) {
            clearTimeout(switchDoneTimer)
            switchDoneTimer = null
        }
        if (switchTimeoutTimer !== null) {
            clearTimeout(switchTimeoutTimer)
            switchTimeoutTimer = null
        }
        switchProgress.value = {
            stage: 'idle',
            targetModel: '',
            errorMessage: '',
            startTime: 0,
            endTime: 0,
            rolledBack: false,
        }
    }

    /**
     * Start model switch - initialize progress state
     */
    function startSwitch(modelName: string) {
        if (switchDoneTimer !== null) {
            clearTimeout(switchDoneTimer)
            switchDoneTimer = null
        }
        if (switchTimeoutTimer !== null) {
            clearTimeout(switchTimeoutTimer)
            switchTimeoutTimer = null
        }
        isModelSwitching.value = true
        switchingModelDisplay.value = formatModelName(modelName).display
        switchStartedAt.value = Date.now()
        previousModelBeforeSwitch.value = currentModel.value

        // Initialize switch progress state
        switchProgress.value = {
            stage: 'preparing',
            targetModel: formatModelName(modelName).display,
            errorMessage: '',
            startTime: Date.now(),
            endTime: 0,
            rolledBack: false,
        }
        
        // Safety timeout - if nothing happens in 300 seconds, clear the switch overlay
        switchTimeoutTimer = setTimeout(() => {
            if (isModelSwitching.value) {
                console.warn('Model switch timed out, force clearing switch overlay')
                isModelSwitching.value = false
                switchingModelDisplay.value = ''
                switchStartedAt.value = 0
                previousModelBeforeSwitch.value = ''
                resetSwitchProgress()
                switchTimeoutTimer = null
            }
        }, 120000)
    }

    /**
     * Handle switch result - update progress based on backend response
     */
    function handleSwitchResult(result: SwitchResult): void {
        if (result.success) {
            switchProgress.value = {
                ...switchProgress.value,
                stage: 'done',
                endTime: Date.now(),
            }

            // 清除安全超时计时器
            if (switchTimeoutTimer !== null) {
                clearTimeout(switchTimeoutTimer)
                switchTimeoutTimer = null
            }
            // 启动完成定时器，延迟后清除切换状态
            if (switchDoneTimer !== null) {
                clearTimeout(switchDoneTimer)
                switchDoneTimer = null
            }
            switchDoneTimer = setTimeout(() => {
                isModelSwitching.value = false
                switchingModelDisplay.value = ''
                switchStartedAt.value = 0
                previousModelBeforeSwitch.value = ''
                resetSwitchProgress()
                switchDoneTimer = null
            }, 800)

            if (result.current_model) {
                currentModel.value = result.current_model
            }
            if (result.capabilities) {
                modelCapabilities.value = result.capabilities
            }
        } else {
            handleSwitchFailure(
                result.error || '模型加载失败',
                result.previous_model || previousModelBeforeSwitch.value,
                result.rolled_back || false,
                switchProgress.value.targetModel,
                result.rollback_success
            )
        }
    }

    /**
     * Enhanced handle switch failure with progress state
     */
    function handleSwitchFailure(error: string, previousModel: string, rolledBack: boolean, failedModelName?: string, rollbackSuccess?: boolean) {
        if (switchDoneTimer !== null) {
            clearTimeout(switchDoneTimer)
            switchDoneTimer = null
        }
        if (switchTimeoutTimer !== null) {
            clearTimeout(switchTimeoutTimer)
            switchTimeoutTimer = null
        }
        modelLoadFailed.value = true
        isModelSwitching.value = false
        switchStartedAt.value = 0

        if (rolledBack) {
            // 根据 rollbackSuccess 显示不同提示
            const rollbackMsg = rollbackSuccess ? '已恢复旧模型' : '恢复旧模型也失败'

            // Mark as rolling back state
            switchProgress.value = {
                stage: 'rolling_back',
                targetModel: failedModelName || switchProgress.value.targetModel,
                errorMessage: `${error}（${rollbackMsg}）`,
                startTime: switchProgress.value.startTime,
                endTime: Date.now(),
                rolledBack: true,
            }

            // After rollback completes, show failed state briefly then reset
            setTimeout(() => {
                switchProgress.value = {
                    stage: 'failed',
                    targetModel: failedModelName || '',
                    errorMessage: `${error}（${rollbackMsg}）`,
                    startTime: switchProgress.value.startTime,
                    endTime: Date.now(),
                    rolledBack: true,
                }
            }, 500)

            // Reset after display
            setTimeout(() => {
                modelLoadFailed.value = false
                switchingModelDisplay.value = ''
                resetSwitchProgress()
            }, 5000)
        } else {
            // Failed without rollback
            switchProgress.value = {
                stage: 'failed',
                targetModel: failedModelName || switchProgress.value.targetModel,
                errorMessage: error,
                startTime: switchProgress.value.startTime,
                endTime: Date.now(),
                rolledBack: false,
            }

            currentModel.value = previousModel

            setTimeout(() => {
                modelLoadFailed.value = false
                switchingModelDisplay.value = ''
                resetSwitchProgress()
            }, 5000)
        }
    }

    function initStatusListener() {
        wails.onServerStatus((status: ServerStatus) => {
            serverStatus.value = status
            if (status.capabilities) {
                modelCapabilities.value = status.capabilities
            }

            // 使用 model_ready（模型真正加载完成）而非 running（服务器进程在运行）
            if (status.model_ready) {
                hasEverBeenReady.value = true
            }

            if (status.current_model && !isModelSwitching.value) {
                const oldModel = currentModel.value
                currentModel.value = status.current_model
                if (shouldReloadConfigOnModelChange(oldModel, status.current_model)) {
                    loadConfig()
                }
            }

            // 首次启动加载完成：收到 model_ready 状态（手动切换由 handleSwitchResult 处理）
            if (switchProgress.value.stage !== 'idle' && status.model_ready && !status.switching && !isModelSwitching.value) {
                if (switchTimeoutTimer !== null) {
                    clearTimeout(switchTimeoutTimer)
                    switchTimeoutTimer = null
                }
                switchProgress.value = {
                    ...switchProgress.value,
                    stage: 'done',
                    endTime: Date.now(),
                }
                switchDoneTimer = setTimeout(() => {
                    resetSwitchProgress()
                    switchDoneTimer = null
                }, 800)
            }

            // 模型加载/切换失败：收到 error 且非 running
            if (switchProgress.value.stage !== 'idle' && !status.running && status.error) {
                switchProgress.value = {
                    ...switchProgress.value,
                    stage: 'failed',
                    errorMessage: status.error,
                    endTime: Date.now(),
                }
                isModelSwitching.value = false
                switchStartedAt.value = 0
                modelLoadFailed.value = true
                switchingModelDisplay.value = switchProgress.value.targetModel
                setTimeout(() => {
                    switchingModelDisplay.value = ''
                    modelLoadFailed.value = false
                    resetSwitchProgress()
                }, 5000)
            }
        })
        checkServerStatus()
    }

    function cleanupStatusListener() {
        wails.offServerStatus()
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
            // 如果没有需要更新的 key，直接返回
            if (Object.keys(keys).length === 0) return
            // 补全空字符串给未提供的字段（后端空值表示不更新）
            const fullKeys: SearchAPIKeys = {
                ollama_api_key: keys.ollama_api_key ?? '',
                tavily_api_key: keys.tavily_api_key ?? '',
            }
            await wails.setSearchAPIKeys(fullKeys)
            // 更新本地状态：非空的覆盖，空值保留原值
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
            // 回滚状态
            searchEnabled.value = oldValue
            console.error('保存搜索设置失败:', e)
        }
    }

    async function cycleThinkingMode() {
        // 统一三态循环：auto → think → no_think → auto
        const next: Record<string, 'auto' | 'think' | 'no_think'> = {
            'auto': 'think',
            'think': 'no_think',
            'no_think': 'auto',
        }
        const oldValue = thinkingSoftSwitch.value
        const nextMode = next[oldValue] || 'auto'
        thinkingSoftSwitch.value = nextMode
        // 同步 thinkingEnabled：auto/think 为 true，no_think 为 false
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

    async function switchModel(modelName: string, previousModel: string): Promise<SwitchResult> {
        // Initialize switch progress
        startSwitch(modelName)

        try {
            const result = await wails.switchModel(modelName)
            handleSwitchResult(result)

            if (result.success) {
                await loadConfig()
            }

            return result
        } catch (e) {
            handleSwitchFailure(String(e), previousModel, false)
            throw e
        }
    }


    function initSwitchProgressListener() {
        wails.onSwitchProgress((progress) => {
            // 状态机单向流转保护：仅在 idle 状态接受进度事件
            // 终态（done/failed/rolling_back）由 handleSwitchResult / handleSwitchFailure 负责收尾，不应被进度事件回滚
            if (switchProgress.value.stage !== 'idle') {
                return
            }
            switchProgress.value = {
                ...switchProgress.value,
                stage: progress.stage as SwitchProgressStage,
                targetModel: progress.targetModel,
            }
            // 收到 loading 事件但不是手动切换 → 首次启动加载，记录开始时间
            if (progress.stage === 'loading' && !isModelSwitching.value) {
                switchProgress.value = {
                    ...switchProgress.value,
                    startTime: switchProgress.value.startTime || Date.now(),
                }
            }
        })
    }

    function cleanupSwitchProgressListener() {
        wails.offSwitchProgress()
    }

    // 首次加载状态：服务器正在运行但模型尚未就绪，且从未就绪过
    const isFirstLoad = computed(() => {
        return !hasEverBeenReady.value && !serverStatus.value.model_ready && !serverStatus.value.error && !isModelSwitching.value
    })

    return {
        config,
        searchEnabled,
        thinkingEnabled,
        thinkingSoftSwitch,
        searchAPIKeys,
        serverStatus,
        modelCapabilities,
        currentModel,
        modelLoadError,
        modelLoadFailed,
        isModelSwitching,
        switchingModelDisplay,
        switchStartedAt,
        previousModelBeforeSwitch,
        isFirstLoad,
        hasEverBeenReady,
        // Expose new switch progress state
        switchProgress,
        loadConfig,
        updateConfig,
        loadSearchAPIKeys,
        saveSearchAPIKeys,
        hasServerAPIKey,
        saveServerAPIKey,
        toggleSearch,
        cycleThinkingMode,
        toggleThinking: cycleThinkingMode, // 兼容旧调用
        switchModel,
        checkServerStatus,
        initStatusListener,
        cleanupStatusListener,
        initSwitchProgressListener,
        cleanupSwitchProgressListener,
        // Export additional helpers
        resetSwitchProgress,
    }
})