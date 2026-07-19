// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

import { describe, it, expect, vi } from 'vitest'
import { ref } from 'vue'
import { useSwitchStateMachine } from '../stores/settings/switchStateMachine'
import type { ModelSwitchState } from '../types/settings'

/**
 * 模型切换状态机单元测试。
 *
 * 重点验证：修复 "卡在准备切换" bug 的核心——reportProgress 必须真正更新
 * switchState.progressStage，让 UI 能显示 loading/waiting/detecting 等中间阶段。
 */
describe('switchStateMachine', () => {
  function createMachine(initialPhase: ModelSwitchState = { phase: 'idle' }) {
    const switchState = ref<ModelSwitchState>(initialPhase)
    const currentModel = ref('old-model')
    const hasEverBeenReady = ref(false)
    const checkServerStatus = vi.fn().mockResolvedValue(undefined)
    const machine = useSwitchStateMachine({
      switchState,
      currentModel,
      hasEverBeenReady,
      checkServerStatus
    })
    return { switchState, currentModel, machine }
  }

  describe('startSwitch', () => {
    it('进入 switching 阶段时 progressStage 初始化为 preparing', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      expect(switchState.value.phase).toBe('switching')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('preparing')
      }
      expect(switchState.value.targetModel).toBe('new-model')
    })
  })

  describe('reportProgress（核心修复点）', () => {
    it('switching 阶段收到 loading 事件应更新 progressStage', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('loading')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('loading')
      }
    })

    it('switching 阶段收到 waiting 事件应更新 progressStage', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('waiting')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('waiting')
      }
    })

    it('switching 阶段收到 detecting 事件应更新 progressStage', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('detecting')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('detecting')
      }
    })

    it('收到 done 事件应更新 progressStage（终态切换由 finishSuccess 完成）', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('done')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('done')
      }
    })

    it('vram-warning 警告事件不应改变主进度阶段', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('loading')
      machine.reportProgress('vram-warning')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('loading')
      }
    })

    it('spec-warning 警告事件不应改变主进度阶段', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('waiting')
      machine.reportProgress('spec-warning')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('waiting')
      }
    })

    it('idle 阶段收到进度事件应被忽略（不崩溃）', () => {
      const { switchState, machine } = createMachine()
      machine.reportProgress('loading')
      expect(switchState.value.phase).toBe('idle')
    })

    it('ready_after_switch 终态收到进度事件应被忽略', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.finishSuccess('new-model')
      machine.reportProgress('loading')
      expect(switchState.value.phase).toBe('ready_after_switch')
    })

    it('failed 终态收到进度事件应被忽略', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.finishFailure('加载失败', 'old-model', false, false)
      machine.reportProgress('loading')
      expect(switchState.value.phase).toBe('failed')
    })

    it('连续多个进度事件应保留最后一个', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('loading')
      machine.reportProgress('waiting')
      machine.reportProgress('detecting')
      if (switchState.value.phase === 'switching') {
        expect(switchState.value.progressStage).toBe('detecting')
      }
    })
  })

  describe('finishSuccess', () => {
    it('从 switching 进入 ready_after_switch', () => {
      const { switchState, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.reportProgress('loading')
      machine.finishSuccess('new-model')
      expect(switchState.value.phase).toBe('ready_after_switch')
    })
  })

  describe('finishFailure', () => {
    it('从 switching 进入 failed 并回滚 previousModel', () => {
      const { switchState, currentModel, machine } = createMachine()
      machine.startSwitch('new-model')
      machine.finishFailure('加载失败', 'old-model', false, false)
      expect(switchState.value.phase).toBe('failed')
      expect(currentModel.value).toBe('old-model')
    })
  })
})
