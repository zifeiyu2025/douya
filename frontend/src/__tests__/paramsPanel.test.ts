import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App as VueApp } from 'vue'

const { messageError } = vi.hoisted(() => ({ messageError: vi.fn() }))

vi.mock('naive-ui', () => ({
  // 抽屉桩直接透传 slot：让内容在 jsdom 下始终可查询
  NDrawer: { name: 'NDrawerStub', template: '<div class="drawer-stub"><slot /></div>' },
  NDrawerContent: {
    name: 'NDrawerContentStub',
    template: '<div class="drawer-content-stub"><slot /></div>'
  },
  NSlider: { name: 'NSliderStub', template: '<input class="slider-stub" type="range" />' },
  NButton: { name: 'NButtonStub', template: '<button><slot /></button>' },
  useMessage: () => ({ error: messageError, success: vi.fn() })
}))

vi.mock('../components/ui/HelpTip.vue', () => ({
  default: { name: 'HelpTipStub', template: '<span class="helptip-stub" />' }
}))

// composable 是模块级单例（isOpen/draft 跨组件共享），mock factory 内建真 ref
// 并经 __bindings 暴露给测试体操控；单例特性保证所有用例共享同一份状态
vi.mock('../composables/useSamplingSettings', async () => {
  const { ref } = await import('vue')
  const isOpen = ref(true)
  const draft = ref<Record<string, number>>({
    temperature: 0.7,
    top_p: 0.95,
    top_k: 40,
    min_p: 0.05,
    repeat_penalty: 1.1,
    dry_multiplier: 0
  })
  const lastError = ref('')
  const matchedModelRef = ref<Record<string, unknown> | null>(null)
  const recommendedRaw = ref<Record<string, number> | null>(null)
  const scheduleFlush = vi.fn()
  const closeParamsPanel = vi.fn()
  const setToRecommended = vi.fn()
  const applyAllRecommended = vi.fn()
  return {
    // 精简两条覆盖 recommendable 两态：temperature 有角标 / min_p 无角标
    SAMPLER_SLIDERS: [
      {
        key: 'temperature',
        label: 'Temperature',
        tip: '随机性',
        min: 0,
        max: 2,
        step: 0.01,
        recommendable: true
      },
      {
        key: 'min_p',
        label: 'Min-P',
        tip: '动态过滤',
        min: 0,
        max: 1,
        step: 0.01,
        recommendable: false
      }
    ],
    __bindings: {
      isOpen,
      draft,
      lastError,
      matchedModelRef,
      recommendedRaw,
      scheduleFlush,
      closeParamsPanel,
      setToRecommended,
      applyAllRecommended
    },
    useSamplingSettings: () => ({
      isOpen,
      draft,
      lastError,
      matchedModelRef,
      recommendedRaw,
      scheduleFlush,
      closeParamsPanel,
      setToRecommended,
      applyAllRecommended
    })
  }
})

import ParamsPanel from '../components/chat/ParamsPanel.vue'
import * as samplingModule from '../composables/useSamplingSettings'

const bindings = (samplingModule as unknown as { __bindings: Record<string, any> }).__bindings

async function flushRendering() {
  await Promise.resolve()
  await nextTick()
}

describe('ParamsPanel 渲染与交互', () => {
  let app: VueApp<Element> | null = null
  let host: HTMLDivElement

  beforeEach(() => {
    messageError.mockReset()
    bindings.isOpen.value = true
    bindings.draft.value = {
      temperature: 0.7,
      top_p: 0.95,
      top_k: 40,
      min_p: 0.05,
      repeat_penalty: 1.1,
      dry_multiplier: 0
    }
    bindings.lastError.value = ''
    bindings.matchedModelRef.value = null
    bindings.recommendedRaw.value = null
    bindings.setToRecommended.mockClear()
    bindings.applyAllRecommended.mockClear()
    bindings.scheduleFlush.mockClear()

    host = document.createElement('div')
    document.body.appendChild(host)
    const Host = defineComponent({
      name: 'ParamsPanelHost',
      setup: () => () => h(ParamsPanel)
    })
    app = createApp(Host)
    app.mount(host)
  })

  afterEach(() => {
    if (app) app.unmount()
    app = null
    host.remove()
  })

  it('匹配到官方推荐模型：来源文案 + 仅 recommendable 行有角标 + 单项/一键回推荐', async () => {
    bindings.matchedModelRef.value = { name: 'Qwen3-8B-TEST' }
    bindings.recommendedRaw.value = {
      temperature: 0.6,
      top_p: 0.95,
      top_k: 40,
      repeat_penalty: 1.05
    }
    await flushRendering()

    const sourceLine = document.querySelector('.params-source-line')?.textContent ?? ''
    expect(sourceLine).toContain('Qwen3-8B-TEST 官方推荐值')

    // 两个滑块行都渲染，但只有 temperature（recommendable）带推荐角标，值为 raw.temperature
    expect(document.querySelectorAll('.param-row').length).toBe(2)
    const chips = document.querySelectorAll('.ref-chip')
    expect(chips.length).toBe(1)
    expect(chips[0].textContent?.trim()).toBe('0.6')

    chips[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushRendering()
    expect(bindings.setToRecommended).toHaveBeenCalledWith('temperature')

    const applyAll = document.querySelector<HTMLButtonElement>('.apply-all-btn')
    expect(applyAll).not.toBeNull()
    applyAll!.click()
    await flushRendering()
    expect(bindings.applyAllRecommended).toHaveBeenCalledTimes(1)
  })

  it('无官方推荐的模型：兜底文案 + 无角标 + 无一键回推荐按钮', async () => {
    await flushRendering()

    expect(document.querySelector('.params-source-line')?.textContent).toContain(
      '当前模型无官方推荐参数'
    )
    expect(document.querySelectorAll('.ref-chip').length).toBe(0)
    expect(document.querySelector('.apply-all-btn')).toBeNull()
    // 滑块行仍正常渲染并展示草稿值
    expect(document.querySelectorAll('.param-row').length).toBe(2)
    expect(document.querySelector('.slider-value')?.textContent?.trim()).toBe('0.7')
  })

  it('composable 上报保存失败时经 useMessage 弹 toast 并复位错误', async () => {
    bindings.lastError.value = '参数保存失败，请稍后在设置页重试'
    await flushRendering()

    expect(messageError).toHaveBeenCalledTimes(1)
    expect(messageError).toHaveBeenCalledWith('参数保存失败，请稍后在设置页重试')
    // 弹完即清空，避免抽屉下次打开重复弹旧错误
    expect(bindings.lastError.value).toBe('')
  })
})
