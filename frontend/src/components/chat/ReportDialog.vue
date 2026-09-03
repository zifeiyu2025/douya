<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    :mask-closable="false"
    :style="{ width: '560px' }"
    title="报告问题"
    class="report-dialog"
  >
    <div class="report-body">
      <p class="report-tip">
        如果你认为这条 AI
        生成的内容不当，请选择原因并提交。报告将通过你本机的邮件客户端发送给开发者（豆芽是本地应用，不会上传你的对话）。
      </p>

      <div class="report-field">
        <label class="report-label">举报原因</label>
        <n-select v-model:value="reason" :options="reasonOptions" placeholder="请选择举报原因" />
      </div>

      <div class="report-field">
        <label class="report-label">补充说明（可选）</label>
        <n-input
          v-model:value="remark"
          type="textarea"
          :rows="3"
          placeholder="可填写更多细节，帮助我们改进"
        />
      </div>

      <div class="report-field">
        <label class="report-label">被举报的内容</label>
        <pre class="report-content">{{ previewContent }}</pre>
      </div>

      <div class="report-footer">
        <n-button size="small" @click="copyReport">复制内容</n-button>
        <div class="report-footer-right">
          <n-button size="small" @click="closeReport">取消</n-button>
          <n-button
            size="small"
            type="primary"
            :loading="submitting"
            :disabled="!reason"
            @click="submit"
          >
            提交报告
          </n-button>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { useReportProblem } from '../../composables/useReportProblem'
import { wails } from '../../services/wails'

const { visible, content, closeReport } = useReportProblem()
const message = useMessage()

const reasonOptions = [
  { label: '色情或不当内容', value: '色情或不当内容' },
  { label: '暴力或仇恨言论', value: '暴力或仇恨言论' },
  { label: '骚扰或威胁', value: '骚扰或威胁' },
  { label: '违法或危险信息', value: '违法或危险信息' },
  { label: '泄露隐私信息', value: '泄露隐私信息' },
  { label: '广告或垃圾信息', value: '广告或垃圾信息' },
  { label: '其他', value: '其他' }
]

const reason = ref<string | null>(null)
const remark = ref('')
const submitting = ref(false)

// 预览截断到 800 字，避免超长回复把弹窗撑爆
const previewContent = computed(() => {
  const c = content.value
  if (!c) return '（空）'
  return c.length > 800 ? c.slice(0, 800) + '…' : c
})

/** 组装举报正文（与弹窗预览一致），供复制兜底使用 */
function buildReportText(): string {
  return [
    `举报原因：${reason.value ?? '未选择'}`,
    remark.value.trim() ? `补充说明：${remark.value.trim()}` : '',
    '被举报的 AI 内容：',
    content.value || '（空）'
  ]
    .filter(Boolean)
    .join('\n\n')
}

/** 兜底：系统无邮件客户端或用户想手动发送时，一键复制举报内容 */
async function copyReport() {
  try {
    await navigator.clipboard.writeText(buildReportText())
    message.success('举报内容已复制到剪贴板，请粘贴到邮件中发送给开发者')
  } catch {
    message.error('复制失败，请手动选择复制')
  }
}

async function submit() {
  if (!reason.value) {
    message.warning('请选择举报原因')
    return
  }
  submitting.value = true
  try {
    await wails.reportProblem(content.value, reason.value, remark.value.trim())
    message.success('已打开邮件客户端，请点击发送以完成举报；若未弹出，可点「复制内容」手动发送')
    closeReport()
  } catch {
    // 打开邮件失败时自动复制，保证举报内容不丢失
    try {
      await navigator.clipboard.writeText(buildReportText())
      message.warning('未能打开邮件客户端，举报内容已复制到剪贴板，请粘贴发送给开发者')
    } catch {
      message.error('举报提交失败，请手动复制内容')
    }
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.report-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.report-tip {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.7;
  color: var(--text-secondary);
}

.report-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.report-label {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
}

.report-content {
  margin: 0;
  max-height: 180px;
  overflow: auto;
  padding: 10px 12px;
  border-radius: var(--border-radius-sm);
  background: color-mix(in srgb, var(--bg-primary) 60%, transparent);
  border: 1px solid var(--border-color);
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
}

.report-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.report-footer-right {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
