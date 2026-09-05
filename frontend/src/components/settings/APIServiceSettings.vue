<!--
  APIServiceSettings: API 服务设置
  配置豆芽对外暴露的 API 接口地址和密钥。
  专业路线：保持原生参数名，HelpTip 悬停显示解释。
-->
<template>
  <!-- API 地址 + 端口 -->
  <div class="api-endpoint-row">
    <div class="api-field api-field-host">
      <label class="api-field-label">API 地址</label>
      <n-input
        v-model:value="formConfig.api_base"
        placeholder="http://127.0.0.1"
        class="rounded-input"
        @blur="autoSave"
      />
    </div>
    <span class="api-colon">:</span>
    <div class="api-field api-field-port">
      <label class="api-field-label">端口</label>
      <n-input-number
        v-model:value="formConfig.port"
        :min="1"
        :max="65535"
        placeholder="8080"
        class="rounded-input"
        :show-arrow-buttons="false"
        @blur="autoSave"
      />
    </div>
  </div>

  <!-- API 端点（只读 + 复制） -->
  <n-form-item style="margin-top: 12px">
    <template #label>
      API 端点
      <HelpTip content="完整的 API 访问地址，可直接复制到其他工具中使用" />
    </template>
    <n-input :value="endpoint" readonly class="rounded-input">
      <template #suffix>
        <n-button text @click="copyEndpoint">
          <n-icon :size="14"><CopyOutline /></n-icon>
        </n-button>
      </template>
    </n-input>
  </n-form-item>

  <!-- 模型 ID（供外部工具填写 model 参数） -->
  <n-form-item>
    <template #label>
      模型 ID
      <HelpTip
        content="外部工具（Claude Code、Codex 等）调用 API 时需填写的 model 参数。default 始终指向当前默认模型；其余 ID 对应 models 目录中的各个模型"
      />
    </template>
    <div class="model-id-list">
      <div class="model-id-item">
        <span class="model-id-value">default</span>
        <n-tag type="info" size="small">当前默认模型</n-tag>
        <n-button text class="model-id-copy" @click="copyModelId('default')">
          <n-icon :size="14"><CopyOutline /></n-icon>
        </n-button>
      </div>
      <div v-for="m in modelOptions" :key="m.name" class="model-id-item">
        <span class="model-id-value">{{ m.name }}</span>
        <n-tag v-if="m.is_loaded" type="success" size="small">已加载</n-tag>
        <n-tag v-else-if="m.is_default" type="info" size="small">默认</n-tag>
        <n-button text class="model-id-copy" @click="copyModelId(m.name)">
          <n-icon :size="14"><CopyOutline /></n-icon>
        </n-button>
      </div>
      <div v-if="modelsLoadFailed" class="model-id-empty">
        模型列表加载失败，可稍后重新进入设置页重试
      </div>
      <div v-else-if="modelOptions.length === 0" class="model-id-empty">
        暂无可用模型，请先在模型管理中下载或导入 .gguf 模型
      </div>
    </div>
  </n-form-item>

  <!-- Expose LAN toggle -->
  <n-form-item>
    <template #label>
      开放局域网访问
      <HelpTip
        content="开启后同一局域网内的其他设备可以访问豆芽 API。请确保已设置 API Key 防止未授权访问"
      />
    </template>
    <n-switch v-model:value="formConfig.expose_server" @update:value="onExposeServerToggle" />
  </n-form-item>

  <!-- Server API Key -->
  <n-form-item>
    <template #label>
      服务端 API Key
      <HelpTip
        content="访问 API 所需的密钥，由系统一键生成通用格式（sk-douya- 开头），无需手动设置。密钥加密存储，生成后仅显示一次"
      />
    </template>
    <!-- 启用开关 + 状态标签 + 一键生成（同一行紧凑排版，字段上下文已含"API Key"语义） -->
    <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 8px">
      <n-switch
        v-model:value="formConfig.server_api_key_enabled"
        @update:value="onServerAPIKeyToggle"
      >
        <template #checked>已启用</template>
        <template #unchecked>未启用</template>
      </n-switch>
      <n-tag v-if="hasServerApiKey" type="success" size="small">已设置</n-tag>
      <n-tag v-else type="default" size="small">未设置</n-tag>
      <n-button
        v-if="!generatedServerApiKey"
        size="small"
        type="primary"
        ghost
        :loading="savingServerApiKey"
        @click="generateServerApiKey"
      >
        <template #icon>
          <n-icon :size="13"><KeyOutline /></n-icon>
        </template>
        {{ hasServerApiKey ? '重新生成' : '一键生成' }}
      </n-button>
    </div>
    <!-- 生成后：一次性展示明文（关闭即丢弃，后端不再提供查看接口） -->
    <div v-if="generatedServerApiKey" class="generated-key-box">
      <div class="generated-key-value">{{ generatedServerApiKey }}</div>
      <div class="generated-key-actions">
        <n-button text type="primary" @click="copyGeneratedApiKey">复制</n-button>
        <n-button text @click="dismissGeneratedApiKey">关闭</n-button>
      </div>
      <div class="generated-key-note">请立即复制保存，此 Key 仅显示一次，重启应用后生效</div>
    </div>
  </n-form-item>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import {
  NFormItem,
  NInput,
  NInputNumber,
  NSwitch,
  NButton,
  NIcon,
  NTag,
  useMessage
} from 'naive-ui'
import { CopyOutline, KeyOutline } from '@vicons/ionicons5'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import HelpTip from '../ui/HelpTip.vue'
import { wails, type ModelOption } from '../../services/wails'
import { copyText } from '../../utils/clipboard'
import { logError } from '../../utils/logger'

defineOptions({ name: 'APIServiceSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('APIServiceSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
// 域切片：core 提供表单/保存，apiService 提供服务端密钥与开关逻辑
const { core, apiService } = ctx
const { formConfig, autoSave } = core
const {
  hasServerApiKey,
  generateServerApiKey,
  savingServerApiKey,
  generatedServerApiKey,
  copyGeneratedApiKey,
  dismissGeneratedApiKey,
  onServerAPIKeyToggle,
  onExposeServerToggle
} = apiService

const message = useMessage()

// API 端点 = api_base + /v1
// api_base 已包含端口（通过 port watcher 自动同步），无需再拼接
const endpoint = computed(() => {
  const base = formConfig.value.api_base || 'http://127.0.0.1:8080'
  return `${base}/v1`
})

const copyEndpoint = async () => {
  const ok = await copyText(endpoint.value)
  if (ok) message.success('API 端点已复制')
  else message.error('复制失败，请手动选择文本复制')
}

// 模型 ID 列表：外部工具调用 API 时需填写的 model 参数
const modelOptions = ref<ModelOption[]>([])
const modelsLoadFailed = ref(false)

onMounted(async () => {
  try {
    modelOptions.value = await wails.getAvailableModels()
  } catch (e) {
    logError('Failed to load available models', e)
    modelsLoadFailed.value = true
  }
})

const copyModelId = async (id: string) => {
  const ok = await copyText(id)
  if (ok) message.success(`模型 ID「${id}」已复制`)
  else message.error('复制失败，请手动选择文本复制')
}
</script>

<style scoped>
.api-endpoint-row {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.api-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.api-field-host {
  flex: 3;
}
.api-field-port {
  flex: 1;
}
.api-field-label {
  font-size: 13px;
  color: var(--text-secondary);
}
.api-colon {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-muted);
  padding-bottom: 8px;
}
/* 生成结果一次性展示卡片 */
.generated-key-box {
  width: 100%;
  padding: 12px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  background: var(--bg-secondary);
}
.generated-key-value {
  font-family: monospace;
  font-size: 13px;
  word-break: break-all;
  color: var(--text-primary);
  user-select: all;
}
.generated-key-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.generated-key-note {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary);
}
/* 模型 ID 列表 */
.model-id-list {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.model-id-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-secondary);
}
.model-id-value {
  font-family: monospace;
  font-size: 13px;
  color: var(--text-primary);
  word-break: break-all;
  user-select: all;
}
.model-id-copy {
  margin-left: auto;
  flex-shrink: 0;
}
.model-id-empty {
  font-size: 12px;
  color: var(--text-secondary);
}
.rounded-input :deep(.n-input__input),
.rounded-input :deep(.n-input-wrapper) {
  border-radius: 10px !important;
}
.rounded-input :deep(.n-input) {
  border-radius: 10px !important;
}
</style>
