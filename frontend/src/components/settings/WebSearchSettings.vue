<!--
  WebSearchSettings: 联网搜索设置
  专业路线：保持原生参数名，HelpTip 悬停显示解释。
-->
<template>
  <!-- 联网搜索 -->
  <n-form-item>
    <template #label>
      联网搜索
      <HelpTip content="控制联网搜索行为。关闭/自动/开启" />
    </template>
    <n-select
      v-model:value="formConfig.search_mode"
      :options="[
        { label: '关闭', value: 'off' },
        { label: '自动', value: 'auto' },
        { label: '开启', value: 'on' }
      ]"
      @update:value="autoSave"
    />
  </n-form-item>

  <!-- 搜索 API Key -->
  <n-divider style="margin: 20px 0 12px" />
  <div class="section-label">搜索密钥</div>

  <n-alert
    v-if="searchKeys.ollama_api_key_set || searchKeys.tavily_api_key_set"
    type="info"
    :bordered="false"
    style="margin-bottom: 16px"
  >
    <template #icon>💡</template>
    <div class="alert-content">已配置搜索 API Key，豆芽在联网搜索时会使用这些密钥调用搜索服务</div>
  </n-alert>

  <!-- Ollama API Key -->
  <n-form-item label="Ollama API Key">
    <div class="api-key-row">
      <n-input
        v-model:value="newOllamaApiKey"
        type="password"
        show-password-on="click"
        :placeholder="searchKeys.ollama_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
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
        获取地址:
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
        :placeholder="searchKeys.tavily_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
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
        获取地址:
        <a
          href="https://app.tavily.com/"
          class="external-link"
          @click.prevent="openExternal('https://app.tavily.com/')"
        >
          https://app.tavily.com/
        </a>
        （免费 1000 次/月）
      </span>
    </template>
  </n-form-item>
</template>

<script setup lang="ts">
import { inject, watch } from 'vue'
import { NFormItem, NSelect, NInput, NDivider, NAlert, NTag, useMessage } from 'naive-ui'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import HelpTip from '../ui/HelpTip.vue'
import { openExternal } from '../../utils/externalLink'

defineOptions({ name: 'WebSearchSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('WebSearchSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
// 域切片：core 提供表单/保存，apiService 提供搜索密钥逻辑
const { core, apiService } = ctx
const { formConfig, autoSave } = core
const { searchKeys, newOllamaApiKey, newTavilyApiKey, savingSearchKeys, saveSearchKeys } =
  apiService

const message = useMessage()

// 切换搜索模式时，如果开启且没有 API Key，给出提示
watch(
  () => formConfig.value.search_mode,
  (newMode, oldMode) => {
    if (newMode !== 'off' && oldMode === 'off') {
      if (!searchKeys.value.ollama_api_key_set && !searchKeys.value.tavily_api_key_set) {
        message.warning(
          '未配置搜索 API Key，联网搜索无法生效。请在下方配置 Tavily 或 Ollama API Key',
          { duration: 5000 }
        )
      }
    }
  }
)
</script>

<style scoped>
/* 章节小标题：衬线体呼应书房目录 */
.section-label {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--text-secondary);
  margin-bottom: 8px;
}
/* 自绘格线：替代 NDivider 的纸面分隔 */
.hairline {
  height: 1px;
  background: var(--border-light);
}
.alert-content {
  font-size: 13px;
  line-height: 1.5;
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
.api-key-hint {
  font-size: 12px;
  color: var(--text-muted);
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
