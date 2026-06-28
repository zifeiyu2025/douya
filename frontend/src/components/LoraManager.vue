<template>
  <div class="lora-manager">
    <!-- LoRA 路径配置（需要重启生效） -->
    <div class="lora-section">
      <div class="section-header">
        <span class="section-title">适配器路径</span>
        <n-text depth="3" style="font-size: 12px;">修改后需重启生效</n-text>
      </div>

      <div class="lora-path-list">
        <div v-for="(path, index) in loraPathList" :key="index" class="lora-path-item">
          <div class="lora-path-text" :title="path" @click="handleReplacePath(index)">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0;opacity:0.5">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
              <polyline points="14 2 14 8 20 8"/>
            </svg>
            <span class="path-value">{{ path || '点击选择文件...' }}</span>
          </div>
          <n-button quaternary circle size="tiny" class="lora-path-remove" @click="removePath(index)" title="移除">
            <template #icon>
              <n-icon size="14">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </n-icon>
            </template>
          </n-button>
        </div>
      </div>

      <n-button type="primary" size="small" ghost class="lora-add-btn" @click="handleAddPath">
        <template #icon>
          <n-icon size="14">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
            </svg>
          </n-icon>
        </template>
        添加 LoRA 适配器
      </n-button>

      <!-- 重启提示 -->
      <Transition name="lora-hint">
        <div v-if="pathChanged" class="lora-hint warning">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
            <line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
          <span>路径已修改，重启应用后生效</span>
        </div>
      </Transition>
    </div>

    <!-- 运行时 LoRA 状态（热切换，无需重启） -->
    <div class="lora-section" style="margin-top: 16px;">
      <div class="section-header">
        <span class="section-title">运行时状态</span>
        <n-button quaternary circle size="tiny" class="lora-refresh-btn" :class="{ spinning: loading }" @click="loadAdapters" title="刷新">
          <template #icon>
            <n-icon size="14">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/>
                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
              </svg>
            </n-icon>
          </template>
        </n-button>
      </div>

      <!-- 错误提示 -->
      <div v-if="errorMsg" class="lora-hint error">
        <span>{{ errorMsg }}</span>
      </div>

      <!-- 空状态 -->
      <div v-if="!loading && adapters.length === 0 && !errorMsg" class="lora-empty">
        <span>当前没有加载任何 LoRA 适配器</span>
        <span class="lora-empty-hint">请先在上方添加路径并重启应用</span>
      </div>

      <!-- 适配器列表 -->
      <div v-else-if="adapters.length > 0" class="lora-list">
        <div v-for="adapter in adapters" :key="adapter.id" class="lora-item" :class="{ active: adapter.scale > 0 }">
          <div class="lora-info">
            <div class="lora-id-row">
              <span class="lora-badge" :class="adapter.scale > 0 ? 'on' : 'off'">#{{ adapter.id }}</span>
              <span class="lora-path" :title="adapter.path">{{ adapter.path }}</span>
            </div>
          </div>
          <div class="lora-controls">
            <label class="lora-switch" :class="{ on: adapter.scale > 0 }">
              <input type="checkbox" :checked="adapter.scale > 0" @change="(e: Event) => handleToggle(adapter, (e.target as HTMLInputElement).checked)" />
              <span class="lora-switch-track"></span>
            </label>
            <div class="lora-scale" v-if="adapter.scale > 0">
              <span class="scale-label">Scale</span>
              <n-input-number
                :value="adapter.scale"
                :min="0.01"
                :max="2"
                :step="0.1"
                size="small"
                style="width: 96px;"
                @update:value="(val: number | null) => handleScaleChange(adapter.id, val)"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- 加载中 -->
      <div v-if="loading && adapters.length === 0" class="lora-loading">
        <svg class="spin-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
        </svg>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { NInputNumber, useMessage } from 'naive-ui'
import { wails, type LoraAdapter } from '../services/wails'
import { showError } from '../utils/showError'

const props = defineProps<{
  loraPaths: string
}>()

const emit = defineEmits<{
  (e: 'update:loraPaths', value: string): void
}>()

const message = useMessage()
const adapters = ref<LoraAdapter[]>([])
const loading = ref(false)
const errorMsg = ref('')
const pathChanged = ref(false)

// 将逗号分隔的字符串解析为路径列表
const loraPathList = ref<string[]>([])

function parsePaths(paths: string): string[] {
  if (!paths) return []
  return paths.split(',').map(p => p.trim()).filter(p => p !== '')
}

function syncPathsToString() {
  const val = loraPathList.value.filter(p => p.trim() !== '').join(',')
  emit('update:loraPaths', val)
  pathChanged.value = val !== props.loraPaths
}

function removePath(index: number) {
  loraPathList.value.splice(index, 1)
  syncPathsToString()
  // 路径确实变化了才标记
  if (pathChanged.value) {
    // 触发父组件保存
    pathChanged.value = true
  }
}

async function handleAddPath() {
  try {
    const filePath = await wails.selectLoraFile()
    // 用户取消选择（返回空字符串），不响应
    if (!filePath) return
    // 检查是否已存在相同路径，避免重复添加
    if (loraPathList.value.includes(filePath)) {
      message.warning('该适配器已在列表中')
      return
    }
    loraPathList.value.push(filePath)
    syncPathsToString()
  } catch (err) {
    showError(message, '选择文件失败', err)
  }
}

async function handleReplacePath(index: number) {
  try {
    const filePath = await wails.selectLoraFile()
    // 用户取消选择，不响应
    if (!filePath) return
    // 路径未变化，不响应
    if (filePath === loraPathList.value[index]) return
    // 检查是否与其他项重复
    if (loraPathList.value.some((p, i) => i !== index && p === filePath)) {
      message.warning('该适配器已在列表中')
      return
    }
    loraPathList.value[index] = filePath
    syncPathsToString()
  } catch (err) {
    showError(message, '选择文件失败', err)
  }
}

// 监听 props 变化
watch(() => props.loraPaths, (newVal) => {
  const newList = parsePaths(newVal)
  const currentClean = loraPathList.value.filter(p => p.trim() !== '')
  if (JSON.stringify(newList) !== JSON.stringify(currentClean)) {
    loraPathList.value = newList.length > 0 ? [...newList] : []
  }
  pathChanged.value = false
}, { immediate: true })

// 加载 LoRA 适配器列表
async function loadAdapters() {
  // 未配置 LoRA 路径时，llama-server 不会加载任何适配器，
  // 调用 /lora-adapters 端点会返回 400 错误（model name is missing）
  if (parsePaths(props.loraPaths).length === 0) {
    adapters.value = []
    errorMsg.value = ''
    return
  }
  loading.value = true
  errorMsg.value = ''
  try {
    adapters.value = await wails.getLoraAdapters()
  } catch (err) {
    errorMsg.value = `加载失败：${err instanceof Error ? err.message : String(err)}`
  } finally {
    loading.value = false
  }
}

// 切换适配器启用/禁用
async function handleToggle(adapter: LoraAdapter, enabled: boolean) {
  const newScale = enabled ? 1.0 : 0
  const oldScale = adapter.scale
  // 无变化不响应
  if ((oldScale > 0) === enabled) return
  adapter.scale = newScale

  try {
    await wails.setLoraAdapters(adapters.value)
    message.success(enabled ? `已启用适配器 #${adapter.id}` : `已禁用适配器 #${adapter.id}`)
  } catch (err) {
    adapter.scale = oldScale
    showError(message, '操作失败', err)
  }
}

// Scale 值修改后自动保存
async function handleScaleChange(id: number, val: number | null) {
  const target = adapters.value.find(a => a.id === id)
  if (!target) return
  const newScale = val === null ? 0 : val
  // 无变化不响应
  if (newScale === target.scale) return
  const oldScale = target.scale
  target.scale = newScale

  try {
    await wails.setLoraAdapters(adapters.value)
    message.success(`Scale 已更新为 ${newScale}`)
  } catch (err) {
    target.scale = oldScale
    showError(message, '保存失败', err)
  }
}

onMounted(() => {
  loadAdapters()
})
</script>

<style scoped>
.lora-manager {
  width: 100%;
}

.lora-section {
  width: 100%;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

/* ===== 路径列表 ===== */

.lora-path-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.lora-path-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md, 12px);
  background: var(--bg-secondary);
  transition: border-color 0.2s, background 0.2s;
}

.lora-path-item:hover {
  border-color: var(--accent-primary);
  background: var(--bg-tertiary, var(--bg-primary));
}

.lora-path-text {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  cursor: pointer;
  user-select: none;
}

.path-value {
  font-size: 12px;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.lora-path-item:hover .path-value {
  color: var(--text-primary);
}

.lora-path-remove {
  flex-shrink: 0;
}

.lora-path-remove:hover {
  color: var(--accent-danger);
}

/* ===== 添加按钮 ===== */

.lora-add-btn {
  width: 100%;
  margin-top: 8px;
}

/* ===== 提示信息 ===== */

.lora-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  padding: 8px 12px;
  border-radius: var(--border-radius-sm, 8px);
  font-size: 12px;
  line-height: 1.4;
}

.lora-hint.warning {
  background: rgba(255, 195, 0, 0.08);
  border: 1px solid rgba(255, 195, 0, 0.2);
  color: var(--accent-warning);
}

.lora-hint.error {
  background: rgba(250, 81, 81, 0.08);
  border: 1px solid rgba(250, 81, 81, 0.2);
  color: var(--accent-danger);
}

.lora-hint-enter-active,
.lora-hint-leave-active {
  transition: opacity 0.25s, transform 0.25s;
}

.lora-hint-enter-from,
.lora-hint-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

/* ===== 刷新按钮 ===== */

.lora-refresh-btn.spinning :deep(svg) {
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* ===== 空状态 ===== */

.lora-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 20px 0;
  color: var(--text-muted);
  font-size: 13px;
}

.lora-empty-hint {
  font-size: 11px;
  color: var(--text-tertiary, var(--text-muted));
}

/* ===== 加载中 ===== */

.lora-loading {
  display: flex;
  justify-content: center;
  padding: 20px 0;
  color: var(--text-muted);
}

.spin-icon {
  animation: spin 0.8s linear infinite;
}

/* ===== 适配器列表 ===== */

.lora-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.lora-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md, 12px);
  background: var(--bg-secondary);
  transition: border-color 0.2s, background 0.2s;
}

.lora-item:hover {
  background: var(--bg-tertiary, var(--bg-primary));
}

.lora-item.active {
  border-color: rgba(7, 193, 96, 0.3);
  background: rgba(7, 193, 96, 0.03);
}

.lora-info {
  flex: 1;
  min-width: 0;
}

.lora-id-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.lora-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 20px;
  padding: 0 6px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
}

.lora-badge.on {
  background: rgba(7, 193, 96, 0.12);
  color: var(--accent-primary);
}

.lora-badge.off {
  background: var(--bg-hover);
  color: var(--text-muted);
}

.lora-path {
  font-size: 12px;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

/* ===== 控制区 ===== */

.lora-controls {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.lora-scale {
  display: flex;
  align-items: center;
  gap: 6px;
}

.scale-label {
  font-size: 11px;
  color: var(--text-muted);
}

/* ===== 自定义开关 ===== */

.lora-switch {
  position: relative;
  display: inline-block;
  width: 34px;
  height: 20px;
  cursor: pointer;
}

.lora-switch input {
  opacity: 0;
  width: 0;
  height: 0;
  position: absolute;
}

.lora-switch-track {
  position: absolute;
  inset: 0;
  border-radius: 10px;
  background: var(--bg-hover);
  border: 1px solid var(--border-color);
  transition: all 0.2s;
}

.lora-switch-track::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--text-muted);
  transition: all 0.2s;
}

.lora-switch input:checked + .lora-switch-track {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

.lora-switch input:checked + .lora-switch-track::after {
  transform: translateX(14px);
  background: #fff;
}

.lora-switch.on .lora-switch-track {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

.lora-switch.on .lora-switch-track::after {
  transform: translateX(14px);
  background: #fff;
}
</style>
