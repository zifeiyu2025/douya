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
        content="访问 API 所需的密钥，启用后只有提供正确 Key 才能调用 API。密钥加密存储，设置后无法再次查看"
      />
    </template>
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
    </div>
    <div v-if="formConfig.server_api_key_enabled" class="api-key-row">
      <n-input
        v-model:value="serverApiKey"
        type="password"
        show-password-on="click"
        :placeholder="hasServerApiKey ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
        :loading="savingServerApiKey"
        :disabled="savingServerApiKey"
        class="api-key-input"
        @keyup.enter="saveServerApiKey"
      />
      <n-button
        type="primary"
        ghost
        :loading="savingServerApiKey"
        :disabled="!serverApiKey"
        @click="saveServerApiKey"
      >
        保存
      </n-button>
    </div>
  </n-form-item>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
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
import { CopyOutline } from '@vicons/ionicons5'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import HelpTip from '../ui/HelpTip.vue'

defineOptions({ name: 'APIServiceSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('APIServiceSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
const {
  formConfig,
  autoSave,
  serverApiKey,
  hasServerApiKey,
  saveServerApiKey,
  savingServerApiKey,
  onServerAPIKeyToggle,
  onExposeServerToggle
} = ctx

const message = useMessage()

// API 端点 = api_base + /v1
// api_base 已包含端口（通过 port watcher 自动同步），无需再拼接
const endpoint = computed(() => {
  const base = formConfig.value.api_base || 'http://127.0.0.1:8080'
  return `${base}/v1`
})

const copyEndpoint = () => {
  navigator.clipboard.writeText(endpoint.value)
  message.success('API 端点已复制')
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
.api-key-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.api-key-input {
  flex: 1;
}
.rounded-input :deep(.n-input__input),
.rounded-input :deep(.n-input-wrapper) {
  border-radius: 10px !important;
}
.rounded-input :deep(.n-input) {
  border-radius: 10px !important;
}
</style>
