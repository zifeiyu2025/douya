<template>
  <!--
    MCP 服务器管理界面
    生活类比：像管理多家外卖平台对接——每家平台一张卡片，
    可以启停、编辑、删除、测试连接，并查看它提供的菜品（工具）清单。
  -->
  <div class="mcp-settings">
    <!-- 顶部说明 -->
    <n-alert type="info" :bordered="false" class="mcp-intro">
      <template #header>
        <span class="mcp-intro-title">MCP 服务器</span>
      </template>
      <span class="mcp-intro-desc">
        通过 stdio 连接外部 MCP 服务器，自动注册其工具供模型调用。配置保存后自动重连。
      </span>
    </n-alert>

    <!-- 服务器列表 -->
    <div v-if="servers.length === 0" class="mcp-empty">
      <n-empty description="尚未配置任何 MCP 服务器" size="small" />
    </div>

    <div v-else class="mcp-server-list">
      <div
        v-for="server in servers"
        :key="server.name"
        class="mcp-server-card"
        :class="{ 'mcp-server-disabled': !server.enabled }"
      >
        <!-- 卡片头部：名称 + 状态 + 操作按钮 -->
        <div class="mcp-server-header">
          <div class="mcp-server-info">
            <n-switch
              :value="server.enabled"
              size="small"
              @update:value="(v: boolean) => toggleEnabled(server.name, v)"
            />
            <span class="mcp-server-name" :title="server.name">{{ server.name }}</span>
            <n-tag v-if="getStatus(server.name)?.connected" type="success" size="small" round>
              已连接 · {{ getStatus(server.name)?.tool_count || 0 }} 工具
            </n-tag>
            <n-tag v-else-if="server.enabled" type="warning" size="small" round>
              {{ getStatus(server.name)?.error || '未连接' }}
            </n-tag>
            <n-tag v-else type="default" size="small" round>已禁用</n-tag>
          </div>
          <div class="mcp-server-actions">
            <n-button
              size="tiny"
              quaternary
              :loading="testingName === server.name"
              @click="handleTest(server)"
            >
              测试连接
            </n-button>
            <n-button size="tiny" quaternary @click="handleEdit(server)">编辑</n-button>
            <n-button size="tiny" quaternary type="error" @click="handleDelete(server.name)">
              删除
            </n-button>
          </div>
        </div>

        <!-- 卡片详情：command + args -->
        <div class="mcp-server-detail">
          <div class="mcp-detail-row">
            <span class="mcp-detail-label">命令：</span>
            <code class="mcp-detail-value">{{ server.command }}</code>
          </div>
          <div v-if="server.args && server.args.length > 0" class="mcp-detail-row">
            <span class="mcp-detail-label">参数：</span>
            <code class="mcp-detail-value">{{ server.args.join(' ') }}</code>
          </div>
        </div>
      </div>
    </div>

    <!-- 添加按钮 -->
    <n-button type="primary" size="small" ghost class="mcp-add-btn" @click="handleAdd">
      + 添加 MCP 服务器
    </n-button>

    <!-- 添加/编辑模态框 -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editingIndex === -1 ? '添加 MCP 服务器' : '编辑 MCP 服务器'"
      style="width: 560px; max-width: 90vw"
      :mask-closable="false"
    >
      <n-form
        ref="formRef"
        :model="formData"
        :rules="formRules"
        label-placement="left"
        label-width="80"
      >
        <n-form-item label="名称" path="name">
          <n-input
            v-model:value="formData.name"
            placeholder="唯一标识，如 filesystem"
            :disabled="editingIndex !== -1"
          />
        </n-form-item>
        <n-form-item label="命令" path="command">
          <n-input v-model:value="formData.command" placeholder="可执行文件，如 npx 或 python" />
        </n-form-item>
        <n-form-item label="参数" path="argsText">
          <n-input
            v-model:value="formData.argsText"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 5 }"
            placeholder="每行一个参数，如：&#10;-y&#10;@modelcontextprotocol/server-filesystem&#10;/path/to/dir"
          />
        </n-form-item>
        <n-form-item label="环境变量" path="envText">
          <n-input
            v-model:value="formData.envText"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 5 }"
            placeholder="每行一个，格式 KEY=VALUE，如：&#10;API_KEY=xxx&#10;DEBUG=true"
          />
        </n-form-item>
        <n-form-item label="启用">
          <n-switch v-model:value="formData.enabled" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="mcp-modal-footer">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import {
  NAlert,
  NButton,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSwitch,
  NTag,
  useMessage,
  type FormInst,
  type FormRules
} from 'naive-ui'
import { wails } from '../../services/wails'
import type { MCPServerConfig, MCPServerStatus } from '../../services/wails'
import { showError, showSuccess } from '../../utils/showError'
import { logError } from '../../utils/logger'

const message = useMessage()

// ============ 响应式状态 ============
// 服务器配置列表（与后端 Config.mcp_servers 同步）
const servers = ref<MCPServerConfig[]>([])
// 服务器运行状态（name -> status）
const statusMap = ref<Record<string, MCPServerStatus>>({})
// 当前正在测试连接的服务器名（用于按钮 loading）
const testingName = ref('')
// 模态框显示
const showModal = ref(false)
// 编辑索引：-1 表示新增，>=0 表示编辑对应索引
const editingIndex = ref(-1)
// 保存中状态
const saving = ref(false)
// 表单引用
const formRef = ref<FormInst | null>(null)

// 表单数据
const formData = reactive({
  name: '',
  command: '',
  argsText: '',
  envText: '',
  enabled: true
})

// 表单校验规则
const formRules: FormRules = {
  name: {
    required: true,
    message: '请输入服务器名称',
    trigger: 'blur'
  },
  command: {
    required: true,
    message: '请输入命令',
    trigger: 'blur'
  }
}

// ============ 工具函数 ============
// 获取某个服务器的运行状态
function getStatus(name: string): MCPServerStatus | undefined {
  return statusMap.value[name]
}

// 将 args 数组转为文本（每行一个）
function argsToText(args: string[]): string {
  return (args || []).join('\n')
}

// 将 env 对象转为文本（每行一个 KEY=VALUE）
function envToText(env: Record<string, string>): string {
  if (!env) return ''
  return Object.entries(env)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

// 将文本解析为 args 数组（按行分割，去空行）
function textToArgs(text: string): string[] {
  return text
    .split('\n')
    .map(line => line.trim())
    .filter(line => line.length > 0)
}

// 将文本解析为 env 对象（按行分割，KEY=VALUE 格式）
function textToEnv(text: string): Record<string, string> {
  const env: Record<string, string> = {}
  text
    .split('\n')
    .map(line => line.trim())
    .filter(line => line.length > 0 && line.includes('='))
    .forEach(line => {
      const idx = line.indexOf('=')
      const key = line.slice(0, idx).trim()
      const value = line.slice(idx + 1).trim()
      if (key) env[key] = value
    })
  return env
}

// ============ 数据加载 ============
// 加载服务器列表和状态
async function loadData() {
  try {
    const [list, status] = await Promise.all([wails.getMCPServers(), wails.getMCPStatus()])
    servers.value = list || []
    statusMap.value = status || {}
  } catch (e) {
    logError('MCPSettings.loadData', e)
    showError(message, '加载 MCP 配置失败', e)
  }
}

// 刷新状态（不重新加载配置，只更新运行状态）
async function refreshStatus() {
  try {
    statusMap.value = await wails.getMCPStatus()
  } catch (e) {
    logError('MCPSettings.refreshStatus', e)
  }
}

// ============ 操作处理 ============
// 添加新服务器
function handleAdd() {
  editingIndex.value = -1
  formData.name = ''
  formData.command = ''
  formData.argsText = ''
  formData.envText = ''
  formData.enabled = true
  showModal.value = true
}

// 编辑现有服务器
function handleEdit(server: MCPServerConfig) {
  const idx = servers.value.findIndex(s => s.name === server.name)
  if (idx === -1) return
  editingIndex.value = idx
  formData.name = server.name
  formData.command = server.command
  formData.argsText = argsToText(server.args)
  formData.envText = envToText(server.env)
  formData.enabled = server.enabled
  showModal.value = true
}

// 删除服务器
async function handleDelete(name: string) {
  if (!confirm(`确定删除服务器「${name}」吗？`)) return
  try {
    const newList = servers.value.filter(s => s.name !== name)
    await wails.saveMCPServers(newList)
    servers.value = newList
    showSuccess(message, `已删除服务器「${name}」`)
    // 延迟刷新状态，等后端断开连接
    setTimeout(refreshStatus, 500)
  } catch (e) {
    logError('MCPSettings.handleDelete', e)
    showError(message, '删除失败', e)
  }
}

// 切换启停状态
async function toggleEnabled(name: string, enabled: boolean) {
  try {
    const newList = servers.value.map(s => (s.name === name ? { ...s, enabled } : s))
    await wails.saveMCPServers(newList)
    servers.value = newList
    // 延迟刷新状态，等后端重连
    setTimeout(refreshStatus, 500)
  } catch (e) {
    logError('MCPSettings.toggleEnabled', e)
    showError(message, '切换状态失败', e)
  }
}

// 测试连接
async function handleTest(server: MCPServerConfig) {
  testingName.value = server.name
  try {
    const result = await wails.testMCPConnection(server)
    if (result.success) {
      showSuccess(message, `连接成功，发现 ${result.tool_count} 个工具`)
    } else {
      showError(message, `连接失败`, result.error || '未知错误')
    }
    // 测试后刷新状态
    await refreshStatus()
  } catch (e) {
    logError('MCPSettings.handleTest', e)
    showError(message, '测试连接失败', e)
  } finally {
    testingName.value = ''
  }
}

// 保存（添加或编辑后提交）
async function handleSave() {
  // 表单校验
  try {
    await formRef.value?.validate()
  } catch {
    return // 校验失败，不继续
  }

  // 检查名称唯一性（新增时）
  if (editingIndex.value === -1) {
    const exists = servers.value.some(s => s.name === formData.name)
    if (exists) {
      showError(message, '名称冲突', `已存在名为「${formData.name}」的服务器`)
      return
    }
  }

  saving.value = true
  try {
    // 构建新的服务器配置
    const newServer: MCPServerConfig = {
      name: formData.name.trim(),
      command: formData.command.trim(),
      args: textToArgs(formData.argsText),
      env: textToEnv(formData.envText),
      enabled: formData.enabled
    }

    // 替换或追加到列表
    let newList: MCPServerConfig[]
    if (editingIndex.value === -1) {
      newList = [...servers.value, newServer]
    } else {
      newList = [...servers.value]
      newList[editingIndex.value] = newServer
    }

    // 保存到后端
    await wails.saveMCPServers(newList)
    servers.value = newList
    showModal.value = false
    showSuccess(message, editingIndex.value === -1 ? '已添加服务器' : '已更新服务器')
    // 延迟刷新状态，等后端重连
    setTimeout(refreshStatus, 800)
  } catch (e) {
    logError('MCPSettings.handleSave', e)
    showError(message, '保存失败', e)
  } finally {
    saving.value = false
  }
}

// ============ 生命周期 ============
onMounted(loadData)
</script>

<style scoped>
.mcp-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mcp-intro {
  margin-bottom: 4px;
}

.mcp-intro-title {
  font-weight: 600;
}

.mcp-intro-desc {
  font-size: 13px;
  opacity: 0.85;
}

.mcp-empty {
  padding: 24px 0;
  display: flex;
  justify-content: center;
}

.mcp-server-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mcp-server-card {
  border: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.2));
  border-radius: 6px;
  padding: 10px 12px;
  background: var(--n-color, rgba(128, 128, 128, 0.05));
  transition: opacity 0.2s;
}

.mcp-server-disabled {
  opacity: 0.55;
}

.mcp-server-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.mcp-server-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.mcp-server-name {
  font-weight: 600;
  font-size: 14px;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mcp-server-actions {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.mcp-server-detail {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--n-border-color, rgba(128, 128, 128, 0.15));
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.mcp-detail-row {
  display: flex;
  gap: 6px;
  font-size: 12px;
  align-items: flex-start;
}

.mcp-detail-label {
  color: var(--n-text-color-3, #888);
  flex-shrink: 0;
  min-width: 48px;
}

.mcp-detail-value {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  word-break: break-all;
  background: var(--n-color-target, rgba(0, 0, 0, 0.05));
  padding: 2px 6px;
  border-radius: 3px;
  flex: 1;
}

.mcp-add-btn {
  align-self: flex-start;
}

.mcp-modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
