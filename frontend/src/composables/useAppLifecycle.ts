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
import type { ServerWarningEvent } from '../services/wails'
import { discreteDialog, discreteMessage } from '../utils/discrete'
import { logError } from '../utils/logger'
import { classifyError } from '../utils/errorGuidance'
import { formatModelName } from '../utils/model'
// F-1.14：stageMap 抽取为常量，与 useModelSwitch.ts 共享
import { STAGE_PERCENT_MAP } from './stageMap'
import { useTTS } from './useTTS'

export function useAppLifecycle() {
  const chatStore = useChatStore()
  const settingsStore = useSettingsStore()
  // TTS 播音员调度台：用于切换会话时停止上一条朗读
  const tts = useTTS()

  // 切换会话时停止当前朗读，避免声音跨会话残留
  watch(
    () => chatStore.currentConversationId,
    () => {
      if (tts.isAnySpeaking()) {
        tts.stop()
      }
    }
  )

  // ===== 灰色地带 GPU 类型选择对话框状态 =====
  // 后端检测到 auto + GPUType=unknown 时触发，让用户选择推理后端
  const gpuTypeChoiceVisible = ref(false)
  const gpuTypeChoicePayload = ref<{
    gpu_name: string
    gpu_vendor: string
    gpu_vram_mb: number
    gpu_type: string
    timeout_seconds: number
  } | null>(null)

  /**
   * 弹出灰色地带 GPU 类型选择对话框。
   * 用户选择后调用 resolveGpuTypeChoice 通知后端，后端解除 startup 阻塞继续启动。
   */
  function showGpuTypeChoiceDialog(payload: {
    gpu_name: string
    gpu_vendor: string
    gpu_vram_mb: number
    gpu_type: string
    timeout_seconds: number
  }) {
    gpuTypeChoicePayload.value = payload
    gpuTypeChoiceVisible.value = true
  }

  /**
   * 用户在对话框中选择后端后调用。
   * "我不清楚" 映射为 cpu，直接继续启动无需读秒等待。
   */
  async function handleGpuTypeChoice(backend: string) {
    gpuTypeChoiceVisible.value = false
    gpuTypeChoicePayload.value = null
    try {
      await wails.resolveGpuTypeChoice(backend)
    } catch (e) {
      logError('resolveGpuTypeChoice failed', e)
      discreteMessage.error('后端选择失败，将使用默认配置启动')
    }
  }

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

  // ===== 启动阶段后端下载状态 =====
  // 当 runtime 缺失时，用户同意下载后后端会推送 downloadStart / downloadProgress / downloadComplete 事件，
  // 前端据此在启动动效中展示下载进度，而非无限转圈。
  // 生活类比：启动屏原本只是"加载中"转圈，下载阶段改为显示"正在下载 xxx 45%"的进度条。
  const isDownloading = ref(false)
  const downloadInfo = ref({
    name: '',
    percent: 0,
    status: '',
    error: '',
    assetName: '',
    label: ''
  })

  /**
   * 是否显示启动屏（对应原 App.vue 中的 showSplash）
   * - 下载阶段：强制显示（展示下载进度）
   * - 首次启动彻底失败：仍显示启动屏展示错误（不转圈）
   * - 首次加载未就绪：显示 splash（失败/超时也仍显示，由 SplashScreen 组件决定是否转圈）
   * - 已就绪后：无论是否超时都不显示 splash
   */
  const showSplash = computed(() => {
    // 下载阶段强制显示启动屏
    if (isDownloading.value) return true
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
   * - 下载阶段 → 'downloading'（展示下载进度条）
   * - 安装中 → 'downloading'（进度100%，展示"安装中"文字）
   * - 下载失败 → 'failed'
   * - first_load_failed → 'failed'（停止转圈，展示错误）
   * - 其他 → 透传 store.switchProgress.stage
   */
  const splashStage = computed(() => {
    // 下载阶段优先：展示下载进度条而非模型加载转圈
    if (isDownloading.value) {
      if (downloadInfo.value.status === 'failed') return 'failed'
      return 'downloading'
    }
    if (settingsStore.switchState.phase === 'first_load_failed') return 'failed'
    return settingsStore.switchProgress.stage
  })

  /**
   * 启动屏上显示的模型名（对应原 App.vue 中的 splashModelName）
   * 下载阶段显示当前下载内容的描述（如"推理后端"、"cudart 依赖包"），其他阶段显示模型名
   */
  const splashModelName = computed(() => {
    // 下载阶段显示当前下载内容的 label（如"推理后端"、"cudart 依赖包"、"安装中"）
    if (isDownloading.value) {
      if (downloadInfo.value.label) return downloadInfo.value.label
      if (downloadInfo.value.name) return downloadInfo.value.name
      return ''
    }
    const name = settingsStore.switchProgress.targetModel || settingsStore.currentModel
    if (!name) return ''
    return formatModelName(name).display
  })

  /**
   * 启动屏进度百分比（对应原 App.vue 中的 splashProgress）
   * - 下载阶段：使用下载百分比
   * - 其他：优先用后端推送的真实加载进度，无进度时用阶段映射兜底
   */
  const splashProgress = computed(() => {
    // 下载阶段使用下载百分比
    if (isDownloading.value) {
      return Math.max(0, Math.min(100, Math.round(downloadInfo.value.percent)))
    }
    // 优先使用后端推送的真实加载进度
    const modelLoadProgress = settingsStore.modelLoadProgress
    if (modelLoadProgress && modelLoadProgress.status === 'loading') {
      return Math.max(5, Math.min(99, Math.round(modelLoadProgress.progress)))
    }
    // 无真实进度时使用粗略阶段映射（仅作为兜底）
    // F-1.14：STAGE_PERCENT_MAP 抽取到 ./stageMap，与 useModelSwitch 共享
    return STAGE_PERCENT_MAP[settingsStore.switchProgress.stage] ?? 0
  })

  // ===== SubTask 8.1: 启动与异常清理事件监听 =====
  // 生活类比：开店前先把对讲机（事件监听）全部打开，再开始营业（await），
  // 这样开门瞬间的任何消息（后端事件）都不会漏接。
  // F-1.12：所有 register 函数返回 unsubscribe，统一收集到 unsubscribers 数组，
  // onUnmounted 中批量调用，替代原来的 init/cleanup 配对调用。
  const unsubscribers: Array<() => void> = []

  onMounted(async () => {
    // 1. 最早注册所有事件监听器，确保不遗漏后端推送的早期事件
    //    注：模型切换相关监听（registerSwitchProgressListener / registerModelLoadProgressListener）
    //    由 useModelSwitch 负责，此处不重复注册。
    unsubscribers.push(chatStore.registerStreamListener())
    unsubscribers.push(settingsStore.registerStatusListener())
    unsubscribers.push(settingsStore.registerMmprojUnavailableListener())
    unsubscribers.push(settingsStore.registerSearchAutoDisabledListener())

    // 启动阶段后端下载事件监听：runtime 缺失时用户同意下载后，后端推送进度到启动屏
    // 三个事件：downloadStart（开始）→ downloadProgress（进度，多次）→ downloadComplete（完成/失败）
    unsubscribers.push(
      wails.subscribeBackendDownloadStart((info: { backend: string; name: string }) => {
        isDownloading.value = true
        downloadInfo.value = {
          name: info.name,
          percent: 0,
          status: 'downloading',
          error: '',
          assetName: '',
          label: '推理后端'
        }
      })
    )
    unsubscribers.push(
      wails.subscribeBackendDownloadProgress(progress => {
        // 下载进度事件：更新百分比、状态和标签
        // status 可能是 downloading / completed / failed / installing / retrying
        // label 可能是"推理后端"、"cudart 依赖包"、"安装中"
        if (!isDownloading.value) isDownloading.value = true
        downloadInfo.value = {
          name: downloadInfo.value.name,
          percent: progress.Percent,
          status: progress.Status,
          error: progress.Error,
          assetName: progress.AssetName,
          label: progress.Label || downloadInfo.value.label
        }
      })
    )
    unsubscribers.push(
      wails.subscribeBackendDownloadComplete(
        (result: { backend: string; success: boolean; error?: string }) => {
          if (result.success) {
            // 下载+解压完成：后端会自动推送"重启中"状态并触发重启
            // 前端不弹窗，直接显示"重启中"状态，等待后端自动重启
            downloadInfo.value = {
              ...downloadInfo.value,
              status: 'completed',
              percent: 100,
              label: '重启中'
            }
          } else {
            // 下载失败：保持 isDownloading=true，状态改为 failed，展示错误
            downloadInfo.value = {
              ...downloadInfo.value,
              status: 'failed',
              error: result.error || '未知错误'
            }
            discreteDialog.error({
              title: '下载失败',
              content: `推理后端下载失败：${result.error || '未知错误'}\n\n请检查网络连接后重启应用重试，或在「设置 → 后端」中手动下载。`,
              positiveText: '知道了'
            })
          }
        }
      )
    )

    // 灰色地带 GPU 类型选择监听：auto 模式 + GPUType=unknown 时后端检测到未知的显卡状态，
    // 弹出对话框让用户选择推理后端，避免错误启用 GPU 导致 OOM。
    // 生活类比：车检员无法判断是跑车还是电瓶车时，让车主自己选驾驶模式。
    unsubscribers.push(
      wails.subscribeHardwareGpuTypeUnknown(payload => {
        showGpuTypeChoiceDialog(payload)
      })
    )

    // 2. 所有 watch 必须在 await 之前注册
    //    原因：await 期间后端可能已推送状态变化，延迟注册会错过首次事件导致无限转圈或会话列表不加载

    // 模型加载成功（model_ready=true）后加载会话列表
    // - 用 model_ready 而非 running：running 在 LoadModel 之前就为 true，但模型尚未就绪
    // - model_ready 只在模型真正加载完成后置 true，失败时永远为 false（符合"失败后不加载"）
    // - immediate: 捕获 watch 注册时已有的状态（await 期间可能已变化）
    let hasLoadedOnReady = false
    watch(
      () => settingsStore.serverStatus.model_ready,
      ready => {
        if (ready && !hasLoadedOnReady) {
          hasLoadedOnReady = true
          chatStore.loadConversations()
        }
      },
      { immediate: true }
    )

    // 首次启动失败时弹出修复建议对话框（而非仅在状态栏显示文字）
    // 生活类比：就像开店时设备出故障，不只挂个"暂停营业"牌子，还要告诉顾客具体出了什么问题、怎么修
    let hasShownStartupError = false
    let hasShownPermanentFailure = false
    watch(
      () => settingsStore.serverStatus.error,
      errorVal => {
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
            style: { whiteSpace: 'pre-wrap' }
          })
        } else {
          // 未匹配到已知错误分类时，也弹窗显示原始错误信息
          discreteDialog.error({
            title: '模型加载失败',
            content: `启动引擎时发生错误，请根据以下信息排查：\n\n错误详情：${errorVal}\n\n可尝试：\n1. 查看设置中的模型路径和参数配置是否正确\n2. 检查 runtime/ 和 models/ 目录文件是否完整\n3. 查看控制台日志获取更多详细信息`,
            positiveText: '知道了',
            style: { whiteSpace: 'pre-wrap' }
          })
        }
      },
      { immediate: true }
    )

    // 3. 加载配置（await 可能耗时，但 watch 已注册，不会错过期间的事件）
    await settingsStore.loadConfig()

    // 同步 TTS 配置到 useTTS（让朗读用用户配置的发音人/语速/音调/音量）
    tts.updateConfig({
      voice: settingsStore.config.tts_voice,
      rate: settingsStore.config.tts_rate,
      pitch: settingsStore.config.tts_pitch,
      volume: settingsStore.config.tts_volume
    })

    // 异常清理事件监听：后端检测到无有效消息的会话时主动推送
    // M4 修复：用标志位记录是否已显示清理提示，避免事件 + getCleanupResult 轮询重复弹窗
    let abnormalCleanupShown = false
    unsubscribers.push(
      wails.subscribeAbnormalCleanup(data => {
        if (abnormalCleanupShown) return
        abnormalCleanupShown = true
        chatStore.loadConversations()
        discreteMessage.info(`已自动清理 ${data.count} 个异常会话（无有效消息）`, {
          duration: 5000
        })
      })
    )

    // 启动时检查是否有清理结果（后端在应用启动前可能已清理过异常会话）
    // M4 修复：如果事件监听已显示过提示，跳过此处轮询，避免重复弹窗
    try {
      const result = await wails.getCleanupResult()
      if (result && result.length > 0 && !abnormalCleanupShown) {
        abnormalCleanupShown = true
        chatStore.loadConversations()
        discreteMessage.info(`已自动清理 ${result.length} 个异常会话（无有效消息）`, {
          duration: 5000
        })
      }
    } catch (e) {
      logError('检查清理结果失败:', e)
    }

    // 退出进度事件监听：后端在优雅退出过程中推送进度，触发退出动效
    unsubscribers.push(
      wails.subscribeShutdownProgress((progress: { stage: string; message: string }) => {
        isExiting.value = true
        exitMessage.value = progress.message
      })
    )

    // 服务器警告事件监听（M1 修复）：
    // 后端在 preset 文件生成失败等非致命问题发生时推送 server:warning 事件，
    // 前端显示 warning 提示让用户知道模型加载可能使用默认参数。
    unsubscribers.push(
      wails.subscribeServerWarning((data: ServerWarningEvent) => {
        discreteMessage.warning(data.message, { duration: 8000 })
      })
    )

    // 注：原 App.vue onMounted 中还有以下逻辑，由其他 composable 负责：
    //   - await Promise.all([loadAvailableModels(), updateMaximizedState()])
    //     · loadAvailableModels 属于模型相关，保留在 App.vue
    //     · updateMaximizedState 由 useWindowControls 负责
    //   - window.addEventListener('resize', handleResize) 由 useWindowControls 负责
  })

  // ===== SubTask 8.3: 组件卸载时统一取消监听与计时器 =====
  // 生活类比：店铺打烊时，要把所有对讲机（事件监听）关掉，
  // 避免关店后还有消息进来却没人处理（内存泄漏）。
  // F-1.12：所有监听器在注册时已返回 unsubscribe，此处批量调用即可。
  onUnmounted(() => {
    // 批量清理本 composable 注册的事件监听（含相关定时器）
    while (unsubscribers.length > 0) {
      const unsubscribe = unsubscribers.pop()
      try {
        unsubscribe?.()
      } catch (e) {
        logError('[useAppLifecycle] unsubscribe failed:', e)
      }
    }

    // 注：以下清理由其他 composable 负责，此处不重复：
    //   - stopSwitchDurationTimer() / settingsStore.registerSwitchProgressListener()() / settingsStore.registerModelLoadProgressListener()()
    //     → 由 useModelSwitch 负责
    //   - window.removeEventListener('resize', handleResize) / clearTimeout(resizeTimer)
    //     → 由 useWindowControls 负责
  })

  return {
    // 启动屏状态
    showSplash, // ComputedRef<boolean>：是否显示启动屏
    splashStage, // ComputedRef<string>：启动屏阶段
    splashModelName, // ComputedRef<string>：启动屏模型名
    splashProgress, // ComputedRef<number>：启动屏进度百分比
    // 退出动效状态（外部只读）
    showExitOverlay: readonly(isExiting), // Readonly<Ref<boolean>>：是否显示退出遮罩
    exitProgress: readonly(exitMessage), // Readonly<Ref<string>>：退出进度消息
    // 灰色地带 GPU 类型选择对话框状态（外部只读 + 操作回调）
    gpuTypeChoiceVisible: readonly(gpuTypeChoiceVisible),
    gpuTypeChoicePayload: readonly(gpuTypeChoicePayload),
    handleGpuTypeChoice
  }
}
