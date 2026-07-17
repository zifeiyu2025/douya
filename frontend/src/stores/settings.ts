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
  type ModelLoadProgressEvent
} from '../services/wails'
import { formatModelName } from '../utils/model'
import { logError } from '../utils/logger'
// 复用全局单例 discrete API，确保 message 跟随应用主题（任务 9）
import { discreteMessage } from '../utils/discrete'
import type { ModelSwitchState, SwitchProgressStage, SwitchProgress } from '../types/settings'
import { useSwitchStateMachine } from './settings/switchStateMachine'

export type { SwitchProgressStage, SwitchProgress } from '../types/settings'

export function shouldReloadConfigOnModelChange(oldModel: string, newModel: string): boolean {
  return !!newModel && oldModel !== newModel
}

export function matchModelRef<T>(modelName: string, refs: Record<string, T>): T | null {
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
  // discreteMessage 来自全局单例（utils/discrete.ts），确保主题一致（任务 9）

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
    tavily_api_key_set: false
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
    supports_preserve_reasoning: false
  })
  const currentModel = ref('')
  const modelLoadError = ref('')
  const hasEverBeenReady = ref(false)

  // ----- 模型加载进度（后端 modelLoadProgress 事件） -----
  const modelLoadProgress = ref<ModelLoadProgressEvent | null>(null)

  // ----- 模型切换状态机（单一 source of truth） -----
  const switchState = ref<ModelSwitchState>({ phase: 'idle' })

  // ----- 状态机派生的兼容 API（不破坏外部代码） -----
  const isModelSwitching = computed(
    () => switchState.value.phase === 'first_load' || switchState.value.phase === 'switching'
  )
  const switchingModelDisplay = computed(() => {
    const s = switchState.value
    if (
      s.phase === 'first_load' ||
      s.phase === 'switching' ||
      s.phase === 'ready_after_switch' ||
      s.phase === 'timeout'
    ) {
      return formatModelName(s.targetModel).display
    }
    if (s.phase === 'failed' || s.phase === 'first_load_failed') {
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
  const modelLoadFailed = computed(
    () => switchState.value.phase === 'failed' || switchState.value.phase === 'first_load_failed'
  )

  /** SwitchProgress（兼容旧接口） */
  const switchProgress = computed<SwitchProgress>(() => {
    const s = switchState.value
    const base: SwitchProgress = {
      stage: 'idle',
      targetModel: '',
      errorMessage: '',
      startTime: 0,
      endTime: 0,
      rolledBack: false
    }
    if (s.phase === 'idle') return base
    if (s.phase === 'first_load' || s.phase === 'switching') {
      return {
        ...base,
        stage: 'preparing',
        targetModel: formatModelName(s.targetModel).display,
        startTime: s.startedAt
      }
    }
    if (s.phase === 'ready_after_switch') {
      return {
        ...base,
        stage: 'done',
        targetModel: formatModelName(s.targetModel).display,
        startTime: s.startedAt,
        endTime: Date.now()
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
        rolledBack: s.rolledBack
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
        rolledBack: false
      }
    }
    if (s.phase === 'first_load_failed') {
      return {
        ...base,
        stage: 'failed',
        targetModel: formatModelName(s.targetModel).display,
        errorMessage: s.error,
        startTime: s.startedAt,
        endTime: Date.now(),
        rolledBack: false
      }
    }
    return base
  })

  const isFirstLoad = computed(() => {
    const phase = switchState.value.phase
    return (
      !hasEverBeenReady.value &&
      !serverStatus.value.model_ready &&
      !serverStatus.value.error &&
      phase !== 'first_load_failed' &&
      !isModelSwitching.value
    )
  })

  // ----- 模型切换状态机（提取到 settings/switchStateMachine.ts） -----
  // checkServerStatus 是 async function 声明，会提升到作用域顶部，此处可安全引用
  const {
    startSwitch,
    reportProgress,
    finishSuccess,
    finishFailure,
    finishFirstLoadFailure,
    reset,
    onServerReady,
    beginFirstLoad,
    startStartupPolling,
    stopStartupPolling,
    startStartupWatchdog,
    markStatusEventReceived,
    clearAllTimers
  } = useSwitchStateMachine({
    switchState,
    currentModel,
    hasEverBeenReady,
    checkServerStatus
  })

  // 状态机 watch 副作用已迁移到 switchStateMachine.ts

  // 监听模型能力变化，仅当用户未手动设置 reasoning 时自动选择最优值
  watch(
    modelCapabilities,
    newCaps => {
      if (!config.value) return
      if (!config.value.reasoning && newCaps.reasoning) {
        config.value.reasoning = 'auto'
      }
    },
    { deep: true }
  )

  // ----- 业务函数 -----

  async function loadConfig() {
    try {
      config.value = await wails.getConfig()
      searchMode.value = (config.value.search_mode as 'off' | 'auto' | 'on') ?? 'off'
    } catch (e) {
      logError('加载配置失败', e)
    }
  }

  async function updateConfig(cfg: Config) {
    try {
      await wails.updateConfig(cfg)
      await loadConfig()
    } catch (e) {
      logError('更新配置失败', e)
      throw e // rethrow 让调用方能捕获并处理
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
      logError('获取服务器状态失败', e)
    }
  }

  async function loadSearchAPIKeys() {
    try {
      const keys = await wails.getSearchAPIKeys()
      // 后端不再返回实际密钥，仅返回设置状态
      searchAPIKeys.value = {
        ...searchAPIKeys.value,
        ollama_api_key_set: keys.ollama_api_key_set ?? false,
        tavily_api_key_set: keys.tavily_api_key_set ?? false
      }
    } catch (e) {
      logError('Failed to load search API keys', e)
    }
  }

  async function saveSearchAPIKeys(keys: Partial<SearchAPIKeys>) {
    // 仅保存用户实际输入的新密钥，空字符串表示未修改
    const fullKeys: SearchAPIKeys = {
      ollama_api_key: keys.ollama_api_key ?? '',
      tavily_api_key: keys.tavily_api_key ?? '',
      ollama_api_key_set: false,
      tavily_api_key_set: false
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
      logError('Failed to check server API key', e)
      return false
    }
  }

  async function saveServerAPIKey(key: string) {
    // 不吞掉错误，让调用方 catch 后给用户视觉反馈
    await wails.setServerAPIKey(key)
  }

  // 配置写入串行化队列：所有配置写入操作排队执行，避免并发覆盖（任务 11 TOCTOU 防护）
  let configWriteQueue: Promise<void> = Promise.resolve()

  function enqueueConfigWrite(task: () => Promise<void>): Promise<void> {
    const run = configWriteQueue.then(task)
    // 即使 task 失败，队列也继续；用 catch 防止队列卡住
    configWriteQueue = run.catch(() => {})
    return run
  }

  /**
   * createCycleFn 创建一个三态循环切换函数。
   *
   * 抽取原因（基于 F-1.11+F-3.7）：cycleSearchMode 和 cycleThinkingMode 结构完全一致，
   * 仅 getCurrent/setCurrent/nextMap/configField/errorLabel 不同，
   * 提取为高阶函数消除重复，新增循环切换只需配置参数。
   *
   * 行为：
   *   1. 读取当前值 → 通过 nextMap 查找下一个值
   *   2. 乐观更新 UI（setCurrent）
   *   3. 入队写入 config（getConfig → updateConfig）
   *   4. 失败时回滚（setCurrent(oldValue)）
   *
   * 生活类比：像一个"挡位切换器"——不管切换的是搜索模式还是思考模式，
   * 都是"读当前挡位→推到下一挡→告诉引擎→引擎失败就退回原挡"的统一流程。
   */
  function createCycleFn<T extends string>(opts: {
    getCurrent: () => T // 获取当前值
    setCurrent: (v: T) => void // 设置新值（用于乐观更新和回滚）
    nextMap: Record<T, T> // 循环映射（如 { off: 'auto', auto: 'on', on: 'off' }）
    applyToConfig: (cfg: Config, v: T) => void // 写入 config 的字段
    errorLabel: string // 错误日志文案
  }) {
    return async function cycleFn() {
      const oldValue = opts.getCurrent()
      const nextMode = opts.nextMap[oldValue]
      if (!nextMode) return
      opts.setCurrent(nextMode) // 乐观更新 UI

      await enqueueConfigWrite(async () => {
        try {
          const fullConfig = await wails.getConfig()
          opts.applyToConfig(fullConfig, nextMode)
          await wails.updateConfig(fullConfig)
          config.value = fullConfig
        } catch (e) {
          logError(opts.errorLabel, e)
          opts.setCurrent(oldValue) // 回滚
        }
      })
    }
  }

  const cycleSearchMode = createCycleFn<'off' | 'auto' | 'on'>({
    getCurrent: () => searchMode.value,
    setCurrent: v => {
      searchMode.value = v
    },
    nextMap: { off: 'auto', auto: 'on', on: 'off' },
    applyToConfig: (cfg, v) => {
      cfg.search_mode = v
    },
    errorLabel: '切换搜索模式失败'
  })

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
      logError('保存搜索设置失败', e)
    }
  }

  const cycleThinkingMode = createCycleFn<'auto' | 'on' | 'off'>({
    getCurrent: () => config.value?.reasoning ?? 'off',
    setCurrent: v => {
      if (config.value) config.value.reasoning = v
    },
    nextMap: { auto: 'on', on: 'off', off: 'auto' },
    applyToConfig: (cfg, v) => {
      cfg.reasoning = v
    },
    errorLabel: '切换思考模式失败'
  })

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

  // F-1.12：事件监听统一为 registerXxxListener(): () => void 模式
  // 返回的 unsubscribe 函数用于取消监听并清理副作用，替代原来的 init/cleanup 配对调用
  // 生活类比：像安装警报器——安装（register）后拿到一个"拆卸凭证"（unsubscribe 函数），
  // 拆卸时会自动断开电源、清理相关配件，不用自己记住每一步。
  function registerStatusListener(): () => void {
    const unsubscribe = wails.subscribeServerStatus((status: ServerStatus) => {
      markStatusEventReceived()
      serverStatus.value = status
      // 后端错误事件触发 failed 状态（仅在 first_load/switching 阶段，避免 idle 时误标记失败）
      if (status.running === false && status.error) {
        const phase = switchState.value.phase
        if (phase === 'first_load') {
          // 首次启动加载失败进入终态，不自动恢复，避免无限"初始化中"
          finishFirstLoadFailure(status.error, currentModel.value)
        } else if (phase === 'switching') {
          const prev =
            'previousModel' in switchState.value
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
    return () => {
      unsubscribe()
      // 清理所有相关定时器，避免离开设置页后定时器残留
      stopStartupPolling() // 清理 startupPollingTimer 与 startupWatchdogTimer
      clearAllTimers() // 清理 pendingTransitions 中所有定时器
    }
  }

  function registerSwitchProgressListener(): () => void {
    const unsubscribe = wails.subscribeSwitchProgress(progress => {
      // 仅在 idle 接受进度事件,首次加载时记录 first_load
      if (switchState.value.phase === 'idle') {
        beginFirstLoad(progress.targetModel || currentModel.value)
      }
      reportProgress(progress.stage as SwitchProgressStage)
    })
    return unsubscribe
  }

  function registerMmprojUnavailableListener(): () => void {
    return wails.subscribeMmprojUnavailable(() => {
      discreteMessage.warning('多模态功能已降级为纯文本模式（mmproj 不兼容）', { duration: 5000 })
    })
  }

  // 监听后端 RAG 开启时自动关闭搜索的事件，同步前端状态，避免后续 autoSave 用旧值覆盖后端
  function registerSearchAutoDisabledListener(): () => void {
    return wails.subscribeSearchAutoDisabled(() => {
      searchMode.value = 'off'
      if (config.value) config.value.search_mode = 'off'
    })
  }

  function registerModelLoadProgressListener(): () => void {
    return wails.subscribeModelLoadProgress((progress: ModelLoadProgressEvent) => {
      if (progress.status === 'running') {
        // 模型加载完成，清除进度状态
        modelLoadProgress.value = null
      } else {
        modelLoadProgress.value = progress
      }
    })
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
    registerStatusListener,
    registerSwitchProgressListener,
    registerMmprojUnavailableListener,
    registerSearchAutoDisabledListener,
    registerModelLoadProgressListener,
    resetSwitchProgress,
    // 状态机内部 API（供测试 / 高级用例使用）
    switchState
  }
})
