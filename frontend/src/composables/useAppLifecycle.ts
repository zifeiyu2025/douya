/**
 * useAppLifecycle - 应用生命周期 composable
 *
 * 从 App.vue 抽取的启动初始化、异常清理、退出进度相关状态与逻辑。
 * 负责应用从"启动屏 → 就绪 → 退出"全过程的 UI 状态与事件监听。
 *
 * 设计原则：
 *  - 纯搬迁，不优化不重构，逻辑与原 App.vue 完全一致
 *  - 依赖注入：通过 useSettingsStore / useChatStore 获取底层状态，不在 composable 内部维护副本
 *  - 事件监听在 onMounted 注册、onUnmounted 清理，避免内存泄漏
 *  - 与 useModelSwitch / useWindowControls 职责分离：
 *      · 本 composable 只负责启动/异常/退出
 *      · 模型切换（server:switchProgress、modelLoadProgress）由 useModelSwitch 负责
 *      · 窗口控制（resize、maximize）由 useWindowControls 负责
 *
 * 生活类比：就像一家店铺的"营业流程管家"——负责开门迎客（启动屏），
 * 处理突发状况（异常清理提示），到点打烊（退出动效）。
 * 而换菜谱（模型切换）和调窗户（窗口控制）由其他专人负责。
 */
import { computed, onMounted, onUnmounted, readonly, ref, watch } from 'vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { wails } from '../services/wails'
import { discreteDialog, discreteMessage } from '../utils/discrete'
import { classifyError } from '../utils/errorGuidance'
import { formatModelName } from '../utils/model'

export function useAppLifecycle() {
    const chatStore = useChatStore()
    const settingsStore = useSettingsStore()

    // ===== SubTask 8.2: 启动屏与退出动效状态 =====

    /**
     * 退出动效是否显示（对应原 App.vue 中的 isExiting）
     * 由 wails.onShutdownProgress 事件触发为 true
     */
    const isExiting = ref(false)

    /**
     * 退出进度消息文本（对应原 App.vue 中的 exitMessage）
     * 由 wails.onShutdownProgress 事件的 message 字段更新
     */
    const exitMessage = ref('')

    /**
     * 模型加载是否超时（启动屏相关，原 App.vue 中的 modelLoadTimeout）
     */
    const modelLoadTimeout = computed(() => settingsStore.switchState.phase === 'timeout')

    /**
     * 是否显示启动屏（对应原 App.vue 中的 showSplash）
     * - 首次启动彻底失败：仍显示启动屏展示错误（不转圈）
     * - 首次加载未就绪：显示 splash（失败/超时也仍显示，由 SplashScreen 组件决定是否转圈）
     * - 已就绪后：无论是否超时都不显示 splash
     */
    const showSplash = computed(() => {
        // 首次启动已彻底失败，仍显示启动屏展示错误（但不转圈）
        if (settingsStore.switchState.phase === 'first_load_failed') return true
        // 首次加载未就绪时显示 splash（无论 failed 还是 timeout 都仍显示）
        // SplashScreen 组件会根据 stage 决定是否转圈（timeout/failed 均映射为 'failed' stage，停止转圈）
        if (!settingsStore.hasEverBeenReady) return true
        // 已就绪后无论是否 timeout 都不显示 splash
        if (modelLoadTimeout.value) return false
        return false
    })

    /**
     * 启动屏阶段（对应原 App.vue 中的 splashStage）
     * - first_load_failed → 'failed'（停止转圈，展示错误）
     * - 其他 → 透传 store.switchProgress.stage
     */
    const splashStage = computed(() => {
        if (settingsStore.switchState.phase === 'first_load_failed') return 'failed'
        return settingsStore.switchProgress.stage
    })

    /**
     * 启动屏上显示的模型名（对应原 App.vue 中的 splashModelName）
     * 优先用 switchProgress.targetModel，其次用 store.currentModel
     */
    const splashModelName = computed(() => {
        const name = settingsStore.switchProgress.targetModel || settingsStore.currentModel
        if (!name) return ''
        return formatModelName(name).display
    })

    /**
     * 启动屏进度百分比（对应原 App.vue 中的 splashProgress）
     * 优先用后端推送的真实加载进度，无进度时用阶段映射兜底
     */
    const splashProgress = computed(() => {
        // 优先使用后端推送的真实加载进度
        const modelLoadProgress = settingsStore.modelLoadProgress
        if (modelLoadProgress && modelLoadProgress.status === 'loading') {
            return Math.max(5, Math.min(99, Math.round(modelLoadProgress.progress)))
        }
        // 无真实进度时使用粗略阶段映射（仅作为兜底）
        const stageMap: Record<string, number> = {
            idle: 0, preparing: 5, loading: 10,
            waiting: 10, detecting: 90, done: 100,
            failed: 100, rolling_back: 50,
        }
        return stageMap[settingsStore.switchProgress.stage] ?? 0
    })

    // ===== SubTask 8.1: 启动与异常清理事件监听 =====
    // 生活类比：开店前先把对讲机（事件监听）全部打开，再开始营业（await），
    // 这样开门瞬间的任何消息（后端事件）都不会漏接。
    onMounted(async () => {
        // 1. 最早注册所有事件监听器，确保不遗漏后端推送的早期事件
        //    注：模型切换相关监听（initSwitchProgressListener / initModelLoadProgressListener）
        //    由 useModelSwitch 负责，此处不重复注册。
        chatStore.initStreamListener()
        settingsStore.initStatusListener()
        settingsStore.initMmprojUnavailableListener()
        settingsStore.initSearchAutoDisabledListener()

        // 2. 所有 watch 必须在 await 之前注册
        //    原因：await 期间后端可能已推送状态变化，延迟注册会错过首次事件导致无限转圈或会话列表不加载

        // 模型加载成功（model_ready=true）后加载会话列表
        // - 用 model_ready 而非 running：running 在 LoadModel 之前就为 true，但模型尚未就绪
        // - model_ready 只在模型真正加载完成后置 true，失败时永远为 false（符合"失败后不加载"）
        // - immediate: 捕获 watch 注册时已有的状态（await 期间可能已变化）
        let hasLoadedOnReady = false
        watch(() => settingsStore.serverStatus.model_ready, (ready) => {
            if (ready && !hasLoadedOnReady) {
                hasLoadedOnReady = true
                chatStore.loadConversations()
            }
        }, { immediate: true })

        // 首次启动失败时弹出修复建议对话框（而非仅在状态栏显示文字）
        // 生活类比：就像开店时设备出故障，不只挂个"暂停营业"牌子，还要告诉顾客具体出了什么问题、怎么修
        let hasShownStartupError = false
        let hasShownPermanentFailure = false
        watch(() => settingsStore.serverStatus.error, (errorVal) => {
            if (!errorVal) return
            // 永久失败是严重状态，跳过阶段限制独立弹窗，确保用户立即感知
            const isPermanentFailure = /永久失败/.test(errorVal)
            if (isPermanentFailure) {
                if (hasShownPermanentFailure) return
                hasShownPermanentFailure = true
            } else {
                if (hasShownStartupError) return
                // 仅在首次加载阶段（从未就绪过）弹出 dialog，避免与手动切换模型的提示重复
                if (settingsStore.hasEverBeenReady) return
                // 仅在 first_load/switching 阶段弹窗，避免 idle 阶段（后端引擎尚未启动）
                // 的 "server not initialized" 等早期错误触发误弹窗
                const phase = settingsStore.switchState.phase
                if (phase !== 'first_load' && phase !== 'switching') return
                hasShownStartupError = true
            }
            const guidance = classifyError(errorVal)
            if (guidance) {
                const suggestions = guidance.suggestions.map((s, i) => `${i + 1}. ${s}`).join('\n')
                discreteDialog.error({
                    title: guidance.title,
                    content: `${guidance.description}\n\n错误详情：${errorVal}\n\n修复建议：\n${suggestions}`,
                    positiveText: '知道了',
                    style: { whiteSpace: 'pre-wrap' },
                })
            } else {
                // 未匹配到已知错误分类时，也弹窗显示原始错误信息
                discreteDialog.error({
                    title: '模型加载失败',
                    content: `启动引擎时发生错误，请根据以下信息排查：\n\n错误详情：${errorVal}\n\n可尝试：\n1. 查看设置中的模型路径和参数配置是否正确\n2. 检查 runtime/ 和 models/ 目录文件是否完整\n3. 查看控制台日志获取更多详细信息`,
                    positiveText: '知道了',
                    style: { whiteSpace: 'pre-wrap' },
                })
            }
        }, { immediate: true })

        // 3. 加载配置（await 可能耗时，但 watch 已注册，不会错过期间的事件）
        await settingsStore.loadConfig()

        // 异常清理事件监听：后端检测到无有效消息的会话时主动推送
        wails.onAbnormalCleanup((data) => {
            chatStore.loadConversations()
            discreteMessage.info(`已自动清理 ${data.count} 个异常会话（无有效消息）`, { duration: 5000 })
        })

        // 启动时检查是否有清理结果（后端在应用启动前可能已清理过异常会话）
        try {
            const result = await wails.getCleanupResult()
            if (result && result.length > 0) {
                chatStore.loadConversations()
                discreteMessage.info(`已自动清理 ${result.length} 个异常会话（无有效消息）`, { duration: 5000 })
            }
        } catch (e) {
            console.error('检查清理结果失败:', e)
        }

        // 退出进度事件监听：后端在优雅退出过程中推送进度，触发退出动效
        wails.onShutdownProgress((progress: { stage: string, message: string }) => {
            isExiting.value = true
            exitMessage.value = progress.message
        })

        // 注：原 App.vue onMounted 中还有以下逻辑，由其他 composable 负责：
        //   - await Promise.all([loadAvailableModels(), updateMaximizedState()])
        //     · loadAvailableModels 属于模型相关，保留在 App.vue
        //     · updateMaximizedState 由 useWindowControls 负责
        //   - window.addEventListener('resize', handleResize) 由 useWindowControls 负责
    })

    // ===== SubTask 8.3: 组件卸载时统一取消监听与计时器 =====
    // 生活类比：店铺打烊时，要把所有对讲机（事件监听）关掉，
    // 避免关店后还有消息进来却没人处理（内存泄漏）。
    onUnmounted(() => {
        // 清理本 composable 注册的事件监听
        chatStore.cleanupStreamListener()
        settingsStore.cleanupStatusListener()
        settingsStore.cleanupMmprojUnavailableListener()
        settingsStore.cleanupSearchAutoDisabledListener()
        wails.offAbnormalCleanup()
        wails.offShutdownProgress()

        // 注：以下清理由其他 composable 负责，此处不重复：
        //   - stopSwitchDurationTimer() / settingsStore.cleanupSwitchProgressListener()
        //     / settingsStore.cleanupModelLoadProgressListener() / wails.offSwitchProgress()
        //     → 由 useModelSwitch 负责
        //   - window.removeEventListener('resize', handleResize) / clearTimeout(resizeTimer)
        //     → 由 useWindowControls 负责
    })

    return {
        // 启动屏状态
        showSplash,        // ComputedRef<boolean>：是否显示启动屏
        splashStage,       // ComputedRef<string>：启动屏阶段
        splashModelName,   // ComputedRef<string>：启动屏模型名
        splashProgress,    // ComputedRef<number>：启动屏进度百分比
        // 退出动效状态（外部只读）
        showExitOverlay: readonly(isExiting),  // Readonly<Ref<boolean>>：是否显示退出遮罩
        exitProgress: readonly(exitMessage),   // Readonly<Ref<string>>：退出进度消息
    }
}
