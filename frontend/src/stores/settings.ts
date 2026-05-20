import { defineStore } from 'pinia'
import { ref } from 'vue'
import { wails, type Config, type ServerStatus, type ModelCapabilities, type SwitchResult } from '../services/wails'
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
    const config = ref<Config>({
        model_path: '',
        llama_server_path: '',
        api_base: '',
        port: 8080,
        context_size: 32768,
        temperature: 0.8,
        top_p: 0.95,
        top_k: 20,
        repeat_penalty: 1.0,
        system_prompt: '',
        chat_background: '',
        user_avatar: '',
        ai_avatar: '',
        search_enabled: false,
        sleep_idle_seconds: 120,
        models_max: 1,
    })
    const searchEnabled = ref(false)
    const serverStatus = ref<ServerStatus>({ running: false })
    const modelCapabilities = ref<ModelCapabilities>({
        image_input: false,
        audio_input: false,
        text_input: true,
        reasoning: false,
    })
    const currentModel = ref('')
    const modelLoadError = ref('')
    const isModelSwitching = ref(false)
    const switchingModelDisplay = ref('')
    const switchStartedAt = ref(0)
    const previousModelBeforeSwitch = ref('')
    const modelLoadFailed = ref(false)

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

    async function loadConfig() {
        try {
            config.value = await wails.getConfig()
            searchEnabled.value = config.value.search_enabled ?? false
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
        isModelSwitching.value = true
        switchingModelDisplay.value = formatModelName(modelName).display
        switchStartedAt.value = Date.now()
        previousModelBeforeSwitch.value = currentModel.value

        // Initialize switch progress state
        switchProgress.value = {
            stage: 'unloading',
            targetModel: formatModelName(modelName).display,
            errorMessage: '',
            startTime: Date.now(),
            endTime: 0,
            rolledBack: false,
        }
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

            // Clear switching state after a short delay for visual feedback
            setTimeout(() => {
                isModelSwitching.value = false
                switchingModelDisplay.value = ''
                switchStartedAt.value = 0
                previousModelBeforeSwitch.value = ''
                resetSwitchProgress()
            }, 1500)

            if (result.current_model) {
                currentModel.value = result.current_model
            }
            if (result.capabilities) {
                modelCapabilities.value = result.capabilities
            }
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

            // Handle switching progress from server status updates
            if (status.switching && isModelSwitching.value) {
                // Server indicates switching in progress
                const currentStage = switchProgress.value.stage

                // Update stage based on current progress
                if (currentStage === 'unloading') {
                    switchProgress.value = {
                        ...switchProgress.value,
                        stage: 'loading',
                    }
                } else if (currentStage === 'loading') {
                    switchProgress.value = {
                        ...switchProgress.value,
                        stage: 'waiting',
                    }
                } else if (currentStage === 'waiting') {
                    switchProgress.value = {
                        ...switchProgress.value,
                        stage: 'detecting',
                    }
                }

                const chatStore = useChatStore()
                chatStore.forceResetGenerating()
                modelLoadError.value = ''
            }

            // Handle model change completion
            if (status.current_model && !isModelSwitching.value) {
                const oldModel = currentModel.value
                currentModel.value = status.current_model
                if (shouldReloadConfigOnModelChange(oldModel, status.current_model)) {
                    loadConfig()
                }
            }

            // Handle running state - may indicate switch completed
            if (status.running && isModelSwitching.value && switchProgress.value.stage !== 'idle') {
                // Server is running - check if switch is complete
                if (status.current_model && status.current_model !== previousModelBeforeSwitch.value) {
                    // Model changed successfully
                    switchProgress.value = {
                        ...switchProgress.value,
                        stage: 'done',
                        endTime: Date.now(),
                    }

                    setTimeout(() => {
                        isModelSwitching.value = false
                        switchingModelDisplay.value = ''
                        switchStartedAt.value = 0
                        previousModelBeforeSwitch.value = ''
                        resetSwitchProgress()
                    }, 1500)
                }
            }

            if (status.running) {
                stopStatusPolling()
            } else if (!status.switching) {
                startStatusPolling()
            }
        })
        checkServerStatus()
        startStatusPolling()
    }

    function cleanupStatusListener() {
        wails.offServerStatus()
        stopStatusPolling()
    }

    async function toggleSearch() {
        searchEnabled.value = !searchEnabled.value
        config.value.search_enabled = searchEnabled.value
        try {
            await wails.updateConfig(config.value)
        } catch (e) {
            console.error('保存搜索设置失败:', e)
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


    return {
        config,
        searchEnabled,
        serverStatus,
        modelCapabilities,
        currentModel,
        modelLoadError,
        modelLoadFailed,
        isModelSwitching,
        switchingModelDisplay,
        switchStartedAt,
        previousModelBeforeSwitch,
        // Expose new switch progress state
        switchProgress,
        loadConfig,
        updateConfig,
        toggleSearch,
        switchModel,
        checkServerStatus,
        initStatusListener,
        cleanupStatusListener,
        // Export additional helpers
        resetSwitchProgress,
    }
})