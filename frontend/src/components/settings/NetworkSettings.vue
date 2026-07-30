<template>
  <div class="settings-group">
    <n-form ref="formRef" :model="formConfig" label-placement="top" class="settings-form">
      <div class="group-header">
        <span class="group-icon">🌐</span>
        <div class="group-text">
          <h3>网络与工具</h3>
          <p>API 服务、工具插件、联网搜索</p>
        </div>
      </div>

      <n-divider style="margin: 16px 0" />

      <!-- 联网搜索 -->
      <n-form-item>
        <template #label>
          联网搜索
          <HelpTip
            content="开启后，对话中输入 /search 可触发实时联网搜索，让豆芽获取最新信息来辅助回答"
          />
        </template>
        <n-select
          v-model:value="formConfig.search_mode"
          :options="[
            { label: '关闭', value: 'off' },
            { label: '自动搜索', value: 'auto' },
            { label: '手动搜索', value: 'on' }
          ]"
          @update:value="autoSave"
        />
      </n-form-item>

      <!-- API 配置 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">🔌</span>
        <span class="section-title">API 服务</span>
      </div>

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

      <n-form-item style="margin-top: 12px">
        <template #label>
          API 端点
          <HelpTip content="完整的 API 访问地址，可直接复制到其他工具中使用" />
        </template>
        <n-input :value="apiEndpoint" readonly class="rounded-input">
          <template #suffix>
            <n-button text @click="copyApiEndpoint">
              <n-icon :size="14"><CopyOutline /></n-icon>
            </n-button>
          </template>
        </n-input>
      </n-form-item>

      <!-- 服务 API Key -->
      <n-form-item style="margin-top: 8px">
        <template #label>
          服务 API Key
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
          <n-tag v-if="hasServerApiKey" type="success" size="small">已设置密钥</n-tag>
          <n-tag v-else type="default" size="small">未设置密钥</n-tag>
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
            size="small"
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

      <!-- 搜索 API Key 设置 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">🔍</span>
        <span class="section-title">搜索 API Key</span>
      </div>

      <n-alert
        v-if="searchKeys.ollama_api_key_set || searchKeys.tavily_api_key_set"
        type="info"
        :bordered="false"
        style="margin-bottom: 16px"
      >
        <template #icon>💡</template>
        <div class="mcp-tip-content">
          已配置搜索 API Key，豆芽在联网搜索时会使用这些密钥调用搜索服务
        </div>
      </n-alert>

      <!-- Ollama API Key -->
      <n-form-item label="Ollama API Key">
        <div class="api-key-row">
          <n-input
            v-model:value="newOllamaApiKey"
            type="password"
            show-password-on="click"
            :placeholder="
              searchKeys.ollama_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'
            "
            :loading="savingSearchKeys"
            :disabled="savingSearchKeys"
            class="api-key-input"
            @blur="saveSearchKeys"
            @keyup.enter="saveSearchKeys"
          />
          <n-tag v-if="searchKeys.ollama_api_key_set" type="success" size="small">已设置</n-tag>
        </div>
        <template #feedback>
          <span class="api-key-hint">
            获取地址：
            <a
              href="https://ollama.com/settings/keys"
              class="external-link"
              @click.prevent="openExternal('https://ollama.com/settings/keys')"
            >
              https://ollama.com/settings/keys
            </a>
          </span>
        </template>
      </n-form-item>

      <!-- Tavily API Key -->
      <n-form-item label="Tavily API Key">
        <div class="api-key-row">
          <n-input
            v-model:value="newTavilyApiKey"
            type="password"
            show-password-on="click"
            :placeholder="
              searchKeys.tavily_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'
            "
            :loading="savingSearchKeys"
            :disabled="savingSearchKeys"
            class="api-key-input"
            @blur="saveSearchKeys"
            @keyup.enter="saveSearchKeys"
          />
          <n-tag v-if="searchKeys.tavily_api_key_set" type="success" size="small">已设置</n-tag>
        </div>
        <template #feedback>
          <span class="api-key-hint">
            获取地址：
            <a
              href="https://app.tavily.com/"
              class="external-link"
              @click.prevent="openExternal('https://app.tavily.com/')"
            >
              https://app.tavily.com/
            </a>
            （免费额度 1000 次/月）
          </span>
        </template>
      </n-form-item>

      <!-- MCP 工具服务器 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">🔧</span>
        <span class="section-title">工具插件（MCP）</span>
      </div>

      <MCPSettings />
    </n-form>
  </div>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import {
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NButton,
  NIcon,
  NDivider,
  NAlert,
  NTag,
  NSwitch,
  useMessage
} from 'naive-ui'
import { CopyOutline } from '@vicons/ionicons5'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import MCPSettings from './MCPSettings.vue'
import HelpTip from '../ui/HelpTip.vue'

defineOptions({ name: 'NetworkSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('NetworkSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
const {
  formConfig,
  autoSave,
  searchKeys,
  newOllamaApiKey,
  newTavilyApiKey,
  savingSearchKeys,
  saveSearchKeys,
  serverApiKey,
  hasServerApiKey,
  saveServerApiKey,
  savingServerApiKey,
  onServerAPIKeyToggle
} = ctx

const message = useMessage()

const apiEndpoint = computed(() => {
  const base = formConfig.value.api_base || 'http://127.0.0.1'
  const port = formConfig.value.port || 8080
  return `${base}:${port}/v1`
})

const copyApiEndpoint = () => {
  navigator.clipboard.writeText(apiEndpoint.value)
  message.success('API 端点已复制')
}

const openExternal = (url: string) => {
  window.open(url, '_blank')
}
</script>

<style scoped>
.settings-group {
  margin-bottom: 32px;
}

.group-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
}

.group-icon {
  font-size: 28px;
}

.group-text h3 {
  font-size: 18px;
  font-weight: 600;
  margin: 0;
  color: var(--n-text-color);
}

.group-text p {
  font-size: 13px;
  color: var(--n-text-color-3);
  margin: 2px 0 0;
}

.settings-form {
  max-width: 720px;
  margin: 0 auto;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.section-icon {
  font-size: 18px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--n-text-color);
}

.rounded-input :deep(.n-input__input),
.rounded-input :deep(.n-input-wrapper) {
  border-radius: 10px !important;
}

.rounded-input :deep(.n-input) {
  border-radius: 10px !important;
}

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
  color: var(--n-text-color-2);
}

.api-colon {
  font-size: 18px;
  font-weight: 600;
  color: var(--n-text-color-3);
  padding-bottom: 8px;
}

.api-keys-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.api-key-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: var(--n-color-2);
  border-radius: 16px;
  font-size: 13px;
  font-family: 'Consolas', 'Monaco', monospace;
}

.key-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--n-color-target);
  background: #22c55e;
}

.key-text {
  color: var(--n-text-color);
  letter-spacing: 0.5px;
}

.key-delete-btn {
  color: var(--n-text-color-3) !important;
}

.key-delete-btn:hover {
  color: #ef4444 !important;
}

.api-keys-empty {
  font-size: 13px;
  color: var(--n-text-color-3);
  padding: 4px 0;
}

.add-key-btn {
  border-radius: 10px !important;
}

.mcp-tip-content {
  font-size: 13px;
  line-height: 1.5;
}

.mcp-tip-content code {
  background: var(--n-color-2);
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 12px;
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

.api-key-input :deep(.n-input__textarea-el),
.api-key-input :deep(.n-input__input-el) {
  background: transparent;
}

.api-key-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.external-link {
  color: var(--accent-primary);
  text-decoration: none;
  cursor: pointer;
}

.external-link:hover {
  text-decoration: underline;
}
</style>
