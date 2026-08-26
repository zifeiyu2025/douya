import { ref, watch } from 'vue'
import type { MessageApi } from 'naive-ui'
import { useSettingsStore } from '../../../stores/settings'
import { type SearchAPIKeys } from '../../../services/wails'
import { showSuccess } from '../../../utils/showError'
import { logError } from '../../../utils/logger'
import type { SettingsCore } from './useSettingsCore'

/**
 * API 服务域：搜索 API Key、服务端 API Key、局域网访问、原生 Web UI、port→api_base 同步。
 * 自 SettingsView 迁出，方法体逐字保留。
 */
export function useAPIServiceSettings(core: SettingsCore, message: MessageApi) {
  const settingsStore = useSettingsStore()
  const { formConfig, autoSave } = core

  // 搜索 API Key 设置状态（后端不再返回实际密钥，仅返回是否已设置）
  const searchKeys = ref<SearchAPIKeys>({
    ollama_api_key: '',
    tavily_api_key: '',
    ollama_api_key_set: false,
    tavily_api_key_set: false
  })

  // 用户输入的新 API Key（不在状态中保存真实密钥）
  const newOllamaApiKey = ref('')
  const newTavilyApiKey = ref('')
  const savingSearchKeys = ref(false)

  async function saveSearchKeys() {
    // 只发送非空的 key，空值表示不更新
    const keysToUpdate: Partial<SearchAPIKeys> = {}
    if (newOllamaApiKey.value) {
      keysToUpdate.ollama_api_key = newOllamaApiKey.value
    }
    if (newTavilyApiKey.value) {
      keysToUpdate.tavily_api_key = newTavilyApiKey.value
    }
    if (Object.keys(keysToUpdate).length === 0) return

    // 构建提示文案：区分保存了哪些 key
    const savedNames: string[] = []
    if (keysToUpdate.ollama_api_key) savedNames.push('Ollama')
    if (keysToUpdate.tavily_api_key) savedNames.push('Tavily')

    savingSearchKeys.value = true
    try {
      await settingsStore.saveSearchAPIKeys(keysToUpdate)
      // 保存成功后清空输入框
      newOllamaApiKey.value = ''
      newTavilyApiKey.value = ''
      showSuccess(message, `${savedNames.join(' + ')} API Key 已保存`)
    } catch (e) {
      logError('Failed to save search API keys', e)
      message.destroyAll()
      message.error('API Key 保存失败，请重试', { duration: 4000 })
    } finally {
      savingSearchKeys.value = false
    }
  }

  const hasServerApiKey = ref(false)
  const savingServerApiKey = ref(false)
  // 一次性展示的生成结果（仅生成时由后端返回，关闭展示区即丢弃）
  const generatedServerApiKey = ref('')

  async function generateServerApiKey() {
    savingServerApiKey.value = true
    try {
      const key = await settingsStore.generateServerAPIKey()
      hasServerApiKey.value = true
      generatedServerApiKey.value = key
      // 后端生成即启用开关，同步表单状态避免 UI 与实际配置不一致
      formConfig.value.server_api_key_enabled = true
      showSuccess(message, 'API Key 已生成（重启应用后生效）')
    } catch (e) {
      logError('Failed to generate server API key', e)
      message.destroyAll()
      message.error('API Key 生成失败，请重试', { duration: 4000 })
    } finally {
      savingServerApiKey.value = false
    }
  }

  function copyGeneratedApiKey() {
    navigator.clipboard.writeText(generatedServerApiKey.value)
    showSuccess(message, 'API Key 已复制')
  }

  function dismissGeneratedApiKey() {
    generatedServerApiKey.value = ''
  }

  async function onServerAPIKeyToggle() {
    await autoSave()
    // 切换开关后需要重新创建 client 以更新 API Key 设置
    if (formConfig.value.server_api_key_enabled) {
      hasServerApiKey.value = await settingsStore.hasServerAPIKey()
    }
  }

  async function onExposeServerToggle() {
    message.destroyAll()
    if (formConfig.value.expose_server) {
      // 开启局域网访问前检查：必须已启用 API Key 并设置密钥
      if (!formConfig.value.server_api_key_enabled || !hasServerApiKey.value) {
        formConfig.value.expose_server = false
        message.error(
          '开启局域网访问前，必须先启用服务端 API Key 并设置密钥。请先在下方设置 API Key 后再开启局域网访问。',
          { duration: 6000 }
        )
        return
      }
      await autoSave()
      message.warning(
        '已开启局域网访问，重启服务后生效。同一局域网内的设备可通过本机 IP 访问 API。',
        {
          duration: 5000
        }
      )
    } else {
      await autoSave()
      message.info('已关闭局域网访问，重启服务后仅本机可访问。', { duration: 3000 })
    }
  }

  async function onEnableWebUIToggle() {
    await autoSave()
    message.destroyAll()
    if (formConfig.value.enable_web_ui) {
      message.warning(
        '已启用原生 Web UI，重启服务后可通过浏览器访问 http://127.0.0.1:' +
          formConfig.value.port +
          ' 。该 UI 与豆芽前端独立，仅供高级用户调试。',
        { duration: 5000 }
      )
    } else {
      message.info('已关闭原生 Web UI，重启服务后生效。', { duration: 3000 })
    }
  }

  // port 变化时自动同步 api_base 中的端口，保持两者一致
  watch(
    () => formConfig.value.port,
    (newPort, oldPort) => {
      if (newPort == null || newPort === oldPort) return
      const base = formConfig.value.api_base || ''
      // 用正则匹配 URL 末尾的 :端口，替换为新端口
      // 匹配 http://host:port 或 http://host:port/path 中的端口部分
      const portPattern = /^(https?:\/\/[^/:]+):\d+/
      if (portPattern.test(base)) {
        formConfig.value.api_base = base.replace(portPattern, `$1:${newPort}`)
      } else if (/^https?:\/\/[^/:]+$/.test(base)) {
        // 地址不含端口（如 http://127.0.0.1），追加端口
        formConfig.value.api_base = `${base}:${newPort}`
      }
      // 如果 api_base 格式异常（不以 http 开头），不做自动修改，让后端校验拦截
    }
  )

  /** 初始化：加载搜索密钥状态与服务端密钥存在性（原 onMounted 中段） */
  async function init() {
    await settingsStore.loadSearchAPIKeys()
    searchKeys.value = { ...settingsStore.searchAPIKeys }
    hasServerApiKey.value = await settingsStore.hasServerAPIKey()
  }

  return {
    newOllamaApiKey,
    newTavilyApiKey,
    searchKeys,
    saveSearchKeys,
    savingSearchKeys,
    hasServerApiKey,
    generateServerApiKey,
    savingServerApiKey,
    generatedServerApiKey,
    copyGeneratedApiKey,
    dismissGeneratedApiKey,
    onServerAPIKeyToggle,
    onExposeServerToggle,
    onEnableWebUIToggle,
    init
  }
}
