import { defineStore } from 'pinia'
import { ref } from 'vue'
import { wails, type Config, DEFAULT_CONFIG, type ServerStatus, type ModelCapabilities, type SwitchResult, type SearchAPIKeys } from '../services/wails'
import { useChatStore } from './chat'
import { formatModelName } from '../utils/model'

/**
 * Switch progress stages for UI display
 */
export type SwitchProgressStage = 'idle' | 'unloading' | 'loading' | 'waiting' | 'detecting' | 'done' | 'failed' | 'rolling_back'

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
    const searchAPIKeys = ref<SearchAPIKeys>({
        ollama_api_key: '',
        tavily_api_key: '',
        github_api_key: '',
    })
    const serverStatus = ref<ServerStatus>({ running: false })
    const modelCapabilities = ref<ModelCapabilities>({
        image_input: false,
        audio_input: false,
        text_input: true,
        reasoning: false,
        mmproj_loaded: false,
        has_mtp: false,
        thinking_mode: 'none',
        n_params: 0,
    })
    const currentModel = ref('')
    const modelLoadError = ref('')
    const isModelSwitching = ref(false)
    const switchingModelDisplay = ref('')
    const switchStartedAt = ref(0)
    const previousModelBeforeSwitch = ref('')
    const modelLoadFailed = ref(false)
    const waitingForStatusReady = ref(false)

    // Enhanced switch progress state
    const switchProgress = ref<SwitchProgress>({
        stage: 'idle',
        targetModel: '',
        errorMessage: '',
        startTime: 0,
        endTime: 0,
        rolledBack: false,
    })

    let statusPollingTimer: ReturnType<typeof setInterval> | null = null
    let switchDoneTimer: ReturnType<typeof setTimeout> | null = null
    let switchTimeoutTimer: ReturnType<typeof setTimeout> | null = null

    async function loadConfig() {
        try {
            config.value = await wails.getConfig()
            searchEnabled.value = config.value.search_enabled ?? false
            thinkingEnabled.value = config.value.thinking_enabled ?? true
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

    function startStatusPolling() {
        stopStatusPolling()
        statusPollingTimer = setInterval(async () => {
            if (!serverStatus.value.running) {
                await checkServerStatus()
            } else {
                stopStatusPolling()
            }
        }, 3000)
    }

    function stopStatusPolling() {
        if (statusPollingTimer !== null) {
            clearInterval(statusPollingTimer)
            statusPollingTimer = null
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
        waitingForStatusReady.value = false
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
        waitingForStatusReady.value = false

        // Initialize switch progress state
        switchProgress.value = {
            stage: 'unloading',
            targetModel: formatModelName(modelName).display,
            errorMessage: '',
            startTime: Date.now(),
            endTime: 0,
            rolledBack: false,
        }
        
        // Safety timeout - if nothing happens in 60 seconds, clear the switch overlay
        switchTimeoutTimer = setTimeout(() => {
            if (isModelSwitching.value) {
                console.warn('Model switch timed out, force clearing switch overlay')
                waitingForStatusReady.value = false
                isModelSwitching.value = false
                switchingModelDisplay.value = ''
                switchStartedAt.value = 0
                previousModelBeforeSwitch.value = ''
                resetSwitchProgress()
                switchTimeoutTimer = null
            }
        }, 60000)
    }

    /**
     * Handle switch result - update progress based on backend response
     */
    function handleSwitchResult(result: SwitchResult): void {
        if (result.success) {
            // Mark done
            switchProgress.value = {
                ...switchProgress.value,
                stage: 'done',
                endTime: Date.now(),
            }

            if (result.current_model) {
                currentModel.value = result.current_model
            }
            if (result.capabilities) {
                modelCapabilities.value = result.capabilities
            }
            
            // Wait for server status to show as running and ready before clearing switch overlay
            waitingForStatusReady.value = true
        } else {
            // Mark failed
            handleSwitchFailure(
                result.error || '模型加载失败',
                result.previous_model || previousModelBeforeSwitch.value,
                result.rolled_back || false,
                switchProgress.value.targetModel
            )
        }
    }

    /**
     * Enhanced handle switch failure with progress state
     */
    function handleSwitchFailure(error: string, previousModel: string, rolledBack: boolean, failedModelName?: string) {
        if (switchDoneTimer !== null) {
            clearTimeout(switchDoneTimer)
            switchDoneTimer = null
        }
        if (switchTimeoutTimer !== null) {
            clearTimeout(switchTimeoutTimer)
            switchTimeoutTimer = null
        }
        waitingForStatusReady.value = false
        modelLoadFailed.value = true
        isModelSwitching.value = false
        switchStartedAt.value = 0

        if (rolledBack) {
            // Mark as rolling back state
            switchProgress.value = {
                stage: 'rolling_back',
                targetModel: failedModelName || switchProgress.value.targetModel,
                errorMessage: error,
                startTime: switchProgress.value.startTime,
                endTime: Date.now(),
                rolledBack: true,
            }

            // After rollback completes, show failed state briefly then reset
            setTimeout(() => {
                switchProgress.value = {
                    stage: 'failed',
                    targetModel: failedModelName || '',
                    errorMessage: error,
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

            if (status.current_model && !isModelSwitching.value) {
                const oldModel = currentModel.value
                currentModel.value = status.current_model
                if (shouldReloadConfigOnModelChange(oldModel, status.current_model)) {
                    loadConfig()
                }
            }

            if (status.running) {
                stopStatusPolling()
            } else if (!status.switching && !isModelSwitching.value) {
                startStatusPolling()
            }

            // If we're waiting for status to be ready and it's now running and not switching
            if (waitingForStatusReady.value && status.running && !status.switching) {
                waitingForStatusReady.value = false
                if (switchTimeoutTimer !== null) {
                    clearTimeout(switchTimeoutTimer)
                    switchTimeoutTimer = null
                }
                // Clear switching state after a short delay for visual feedback
                switchDoneTimer = setTimeout(() => {
                    isModelSwitching.value = false
                    switchingModelDisplay.value = ''
                    switchStartedAt.value = 0
                    previousModelBeforeSwitch.value = ''
                    resetSwitchProgress()
                    switchDoneTimer = null
                }, 1200)
            }
        })
        checkServerStatus()
        startStatusPolling()
    }

    function cleanupStatusListener() {
        wails.offServerStatus()
        stopStatusPolling()
    }

    async function loadSearchAPIKeys() {
        try {
            const keys = await wails.getSearchAPIKeys()
            searchAPIKeys.value = keys
        } catch (e) {
            console.error('Failed to load search API keys:', e)
        }
    }

    async function saveSearchAPIKeys(keys: SearchAPIKeys) {
        try {
            await wails.setSearchAPIKeys(keys)
            searchAPIKeys.value = keys
        } catch (e) {
            console.error('Failed to save search API keys:', e)
        }
    }

    async function loadServerAPIKey(): Promise<string> {
        try {
            return await wails.getServerAPIKey()
        } catch (e) {
            console.error('Failed to load server API key:', e)
            return ''
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
        searchEnabled.value = !searchEnabled.value
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.search_enabled = searchEnabled.value
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
            console.error('保存搜索设置失败:', e)
        }
    }

    async function toggleThinking() {
        thinkingEnabled.value = !thinkingEnabled.value
        try {
            const fullConfig = await wails.getConfig()
            fullConfig.thinking_enabled = thinkingEnabled.value
            await wails.updateConfig(fullConfig)
            config.value = fullConfig
        } catch (e) {
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
            switchProgress.value = {
                ...switchProgress.value,
                stage: progress.stage as SwitchProgressStage,
                targetModel: progress.targetModel,
            }
        })
    }

    function cleanupSwitchProgressListener() {
        wails.offSwitchProgress()
    }

    return {
        config,
        searchEnabled,
        thinkingEnabled,
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
        waitingForStatusReady,
        // Expose new switch progress state
        switchProgress,
        loadConfig,
        updateConfig,
        loadSearchAPIKeys,
        saveSearchAPIKeys,
        loadServerAPIKey,
        saveServerAPIKey,
        toggleSearch,
        toggleThinking,
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