import { afterEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App as VueApp } from 'vue'
import ModelDetailCard from '../components/models/ModelDetailCard.vue'
import type { ModelOption } from '../services/wails'

// 纯展示组件：props 进、DOM 出，无 store / wails 依赖，直接真实挂载

function makeModel(overrides: Partial<ModelOption> = {}): ModelOption {
  return {
    name: 'qwen3-8b-instruct-q4_k_m.gguf',
    model_path: 'D:\\models\\qwen3-8b-instruct-q4_k_m.gguf',
    file_name: 'qwen3-8b-instruct-q4_k_m.gguf',
    is_default: false,
    is_loaded: false,
    mmproj_vision: false,
    mmproj_audio: false,
    mmproj_video: false,
    status: '',
    ...overrides
  }
}

async function flushRendering() {
  await Promise.resolve()
  await nextTick()
}

describe('ModelDetailCard 渲染', () => {
  let app: VueApp<Element> | null = null
  let host: HTMLDivElement

  function mountCard(model: ModelOption | null) {
    host = document.createElement('div')
    document.body.appendChild(host)
    const Host = defineComponent({
      name: 'DetailCardHost',
      setup: () => () => h(ModelDetailCard, { model })
    })
    app = createApp(Host)
    app.mount(host)
  }

  afterEach(() => {
    if (app) app.unmount()
    app = null
    host?.remove()
  })

  it('model 为 null 时整卡不渲染', async () => {
    mountCard(null)
    await flushRendering()
    expect(document.querySelector('.model-detail-card')).toBeNull()
  })

  it('加载中的多模态模型：徽章齐全 + 图片理解能力 + 大小格式化 + loading 态', async () => {
    mountCard(
      makeModel({
        size_label: '8B',
        quant_type: 'Q4_K - Medium',
        is_default: true,
        mmproj_vision: true,
        file_size_bytes: 5 * 1024 * 1024 * 1024,
        status: 'loading'
      })
    )
    await flushRendering()

    expect(document.querySelector('.card-title')?.textContent).toBe('qwen3-8b-instruct-q4_k_m.gguf')
    const badges = Array.from(document.querySelectorAll('.badge')).map(b => b.textContent?.trim())
    expect(badges).toEqual(['8B', 'Q4_K - Medium', '默认'])
    // 具备视觉能力时展示对应条目，不出现“纯文本”兜底
    expect(document.querySelector('.capability-row')?.textContent).toContain('图片理解')
    expect(document.querySelector('.capability-row')?.textContent).not.toContain('纯文本模型')
    // 5 GiB → formatFileSize 输出 "5.0 GB"
    const rows = Array.from(document.querySelectorAll('.detail-row'))
    expect(rows.find(r => r.textContent?.includes('文件大小'))?.textContent).toContain('5.0 GB')
    // status=loading 优先于 is_loaded 判定
    expect(rows.find(r => r.textContent?.includes('加载状态'))?.textContent).toContain('加载中')
    expect(document.querySelector('.status-dot.loading')).not.toBeNull()
  })

  it('休眠的纯文本模型：能力兜底文案 + 未知大小 + 休眠态', async () => {
    mountCard(makeModel({ status: 'sleeping', is_loaded: false }))
    await flushRendering()

    // 无任何 mmproj 能力且无徽章数据时的兜底分支
    expect(document.querySelector('.capability-row')?.textContent).toContain('纯文本模型')
    expect(document.querySelector('.badge-row')).toBeNull()
    // file_size_bytes 缺省显示“未知”
    const rows = Array.from(document.querySelectorAll('.detail-row'))
    expect(rows.find(r => r.textContent?.includes('文件大小'))?.textContent).toContain('未知')
    // sleeping 既非 loading 也未加载 → “休眠中” + idle 圆点
    expect(rows.find(r => r.textContent?.includes('加载状态'))?.textContent).toContain('休眠中')
    expect(document.querySelector('.status-dot.idle')).not.toBeNull()
  })
})
