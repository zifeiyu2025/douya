import { ref, watch, onUnmounted } from 'vue'
import type { MessageApi } from 'naive-ui'
import { useSettingsStore } from '../../../stores/settings'
import { type Config, DEFAULT_CONFIG } from '../../../services/wails'
import { wails } from '../../../services/wails'
import { logError } from '../../../utils/logger' // 安全实践（#34）：用 logError 替代 console.error，避免泄漏后端细节

/**
 * 模型切换 hook：core 的 currentModel watch 触发时按注册顺序依次调用。
 * @param inCleanBlock 是否处于"无未保存修改"分支（原 SettingsView 中 if (!genParamsDirty) 块内）
 */
export type ModelSwitchHook = (inCleanBlock: boolean) => void | Promise<void>

const ALL_CONFIG_KEYS: (keyof Config)[] = [
  'model_path',
  'llama_server_path',
  'backend_type',
  'api_base',
  'port',
  'context_size',
  'temperature',
  'top_p',
  'top_k',
  'repeat_penalty',
  'mmproj_auto',
  'mmproj_offload',
  'mmproj_device',
  'kv_unified',
  'cache_idle_slots',
  'cache_reuse',
  'cache_ram',
  'image_min_tokens',
  'image_max_tokens',
  'fit_target',
  'fit_ctx',
  'system_prompt',
  'system_prompt_mode',
  'chat_background',
  'chat_background_opacity',
  // B5 每主题背景参数（v3）：同一张图，亮/暗主题各一套 opacity/blur/mask_alpha
  // 注意：这两个是嵌套对象字段，UI 层必须整体替换对象才能被下方引用比较的
  // 脏检测与 diff 合并感知（就地修改内部属性会静默丢保存）
  'background_light',
  'background_dark',
  'user_avatar',
  'ai_avatar',
  'search_mode',
  'sleep_idle_seconds',
  'models_max',
  'rag_enabled',
  'rag_active_kb',
  'rag_top_k',
  'rag_min_score',
  'rag_chunk_size',
  'rag_chunk_overlap',
  'embedding_model',
  'mmap',
  'kv_offload',
  'context_shift',
  'min_p',
  'dry_multiplier',
  'dry_base',
  'dry_allowed_length',
  'dry_sequence_breaker',
  'dry_penalty_last_n',
  'grp_attn_n',
  'grp_attn_w',
  'jinja',
  'cache_prompt',
  'metrics',
  'verbose',
  'spec_draft_threads',
  'spec_draft_threads_batch',
  'spec_default',
  'device',
  'parallel',
  'split_mode',
  'tensor_split',
  'main_gpu',
  'cache_type_k',
  'cache_type_v',
  'spec_type',
  'spec_draft_n_max',
  'spec_draft_n_min',
  'spec_ngram_mod_n_min',
  'spec_ngram_mod_n_max',
  'spec_ngram_mod_n_match',
  'spec_ngram_simple_size_n',
  'spec_ngram_simple_size_m',
  'spec_ngram_simple_min_hits',
  'spec_ngram_map_k_size_n',
  'spec_ngram_map_k_size_m',
  'spec_ngram_map_k_min_hits',
  'spec_ngram_map_k4v_size_n',
  'spec_ngram_map_k4v_size_m',
  'spec_ngram_map_k4v_min_hits',
  'lookup_cache_static',
  'lookup_cache_dynamic',
  'spec_draft_model',
  'cache_type_k_draft',
  'cache_type_v_draft',
  'chat_template_file',
  'server_api_key_enabled',
  'expose_server',
  'enable_web_ui',
  'swa_full',
  'ctx_checkpoints',
  'checkpoint_min_step',
  'tools',
  'tools_runtime',
  'prefill_assistant',
  'slot_prompt_similarity',
  'skip_chat_parsing',
  'api_prefix',
  'simple_io',
  'agent',
  'backend_sampling',
  'gpu_layers',
  'flash_attn',
  'mlock',
  'threads',
  'threads_http',
  'batch_size',
  // 原生能力：直接 I/O / MoE CPU 卸载 / 算子卸载
  'direct_io',
  'cpu_moe',
  'n_cpu_moe',
  'op_offload',
  // 推理配置
  'reasoning',
  'reasoning_budget',
  'reasoning_budget_message',
  'reasoning_format',
  'reasoning_effort',
  'reasoning_preserve',
  // RAG 重排序配置
  'reranker_model_path',
  'rerank_top_n',
  // KV 缓存持久化配置
  'slot_save_path',
  'slot_save_enabled',
  'lora_paths',
  // Draft 模型 GPU 配置
  'spec_draft_ngl',
  'spec_draft_device',
  // 请求级采样配置
  'samplers',
  'ignore_eos',
  'adaptive_target',
  'adaptive_decay',
  // TTS 朗读配置
  'tts_online',
  'tts_enabled',
  'tts_voice',
  'tts_rate',
  'tts_pitch',
  'tts_volume'
]

/**
 * 设置域核心：formConfig、自动保存（重入保护 + 300ms 防抖）、dirty 跟踪、
 * 模型切换事件分发、卸载前 flush。自 SettingsView 迁出，方法体逐字保留。
 */
export function useSettingsCore(message: MessageApi) {
  const settingsStore = useSettingsStore()
  const saving = ref(false)
  const genParamsDirty = ref(false)
  let genParamsSaveTimer: ReturnType<typeof setTimeout> | null = null

  // 初始值复用 DEFAULT_CONFIG，避免 80+ 字段硬编码
  // init() 时会从 settingsStore 加载实际配置覆盖此默认值
  const formConfig = ref<Config>({ ...DEFAULT_CONFIG })

  let savingPromise: Promise<void> | null = null // 进行中的保存 Promise，防止重入

  async function doAutoSave() {
    if (genParamsSaveTimer) {
      clearTimeout(genParamsSaveTimer)
      genParamsSaveTimer = null
    }
    saving.value = true
    try {
      // 先拉取后端最新配置，避免覆盖其他路径（如 RAG 开启、模型切换）的修改
      const latest = await wails.getConfig()
      // 仅合并用户实际修改过的字段（formConfig 相对于 settingsStore.config 的 diff）
      // 用户未改的字段保留 latest 的值（后端最新），避免用过期值覆盖后端改动
      const merged: Config = { ...latest }
      for (const k of ALL_CONFIG_KEYS) {
        if (formConfig.value[k] !== settingsStore.config[k]) {
          // 字段名和值均来自同一 Config 对象，类型必然匹配
          ;(merged as any)[k] = formConfig.value[k]
        }
      }
      await settingsStore.updateConfig(merged)
      // 浅拷贝替代 JSON.parse(JSON.stringify)，Config 字段均为原始类型
      formConfig.value = { ...settingsStore.config }
      genParamsDirty.value = false
    } catch (e) {
      logError('自动保存配置失败', e)
      message.destroyAll()
      message.error('保存失败')
    } finally {
      saving.value = false
    }
  }

  async function autoSave() {
    // 重入保护：进行中的保存复用同一 Promise，避免并发覆盖
    if (savingPromise) return savingPromise
    savingPromise = doAutoSave()
    try {
      await savingPromise
    } finally {
      savingPromise = null
    }
  }

  const modelSwitchHooks: ModelSwitchHook[] = []

  /** 各域 composable 在 setup 阶段调用，注册自己的模型切换响应逻辑 */
  function onModelSwitch(hook: ModelSwitchHook) {
    modelSwitchHooks.push(hook)
  }

  watch(
    () => settingsStore.currentModel,
    async () => {
      const inCleanBlock = !genParamsDirty.value
      if (inCleanBlock) {
        await settingsStore.loadConfig()
        // 浅拷贝替代 JSON.parse(JSON.stringify)，Config 字段均为原始类型
        formConfig.value = { ...settingsStore.config }
      }
      for (const hook of modelSwitchHooks) {
        await hook(inCleanBlock)
      }
    }
  )

  watch(
    () => ALL_CONFIG_KEYS.map(k => formConfig.value[k]),
    () => {
      const savedConfig = settingsStore.config
      const dirty = ALL_CONFIG_KEYS.some(k => formConfig.value[k] !== savedConfig[k])
      genParamsDirty.value = dirty
      if (dirty) {
        scheduleAutoSave()
      }
    }
  )

  function scheduleAutoSave() {
    if (genParamsSaveTimer) {
      clearTimeout(genParamsSaveTimer)
    }
    // 300ms 防抖：用户感知为即时保存，同时避免快速输入时频繁写盘
    genParamsSaveTimer = setTimeout(() => {
      autoSave()
    }, 300)
  }

  onUnmounted(() => {
    // 离开设置页时如有未保存的修改，立即同步保存，避免丢失
    // 注意：组件已卸载，message 实例可能失效，静默保存并 catch 错误
    if (genParamsSaveTimer) {
      clearTimeout(genParamsSaveTimer)
      genParamsSaveTimer = null
      if (genParamsDirty.value) {
        autoSave().catch(e => {
          // 静默处理：组件已卸载，无法弹 UI 提示，仅控制台记录
          logError('[settings] onUnmounted autoSave failed:', e)
        })
      }
    }
  })

  /** 初始化：加载实际配置并刷新表单（原 onMounted 前半部分） */
  async function init() {
    await settingsStore.loadConfig()
    // 浅拷贝替代 JSON.parse(JSON.stringify)，Config 字段均为原始类型
    formConfig.value = { ...settingsStore.config }
    genParamsDirty.value = false
  }

  return {
    settingsStore,
    formConfig,
    autoSave,
    genParamsDirty,
    saving,
    onModelSwitch,
    init
  }
}

export type SettingsCore = ReturnType<typeof useSettingsCore>
