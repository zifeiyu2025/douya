/**
 * Wails 服务门面 - 系统域
 * 配置读写/关闭流程/版本信息/TTS 语音/启动期前端化对话框
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import {
  GetConfig,
  UpdateConfig,
  GetCleanupResult,
  HandleCloseRequest,
  SetCloseAction,
  GracefulExit,
  RestartApp,
  SelectImageFile,
  GetAppVersion,
  ConfirmStartupError,
  GetStartupError,
  SynthesizeSpeech
} from '../../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import { EventWindowCloseRequest, EventStartupError } from '../events'
import type { Config } from '../../types/chat'
import type { CleanupResult, StartupErrorPayload } from './types'
import { adaptConfig, toWailsConfig } from './adapters'

export const systemMethods = {
  getConfig: async (): Promise<Config> => adaptConfig(await GetConfig()),
  getCleanupResult: async (): Promise<CleanupResult[]> => {
    return (await GetCleanupResult()) as CleanupResult[]
  },
  updateConfig: async (cfg: Config): Promise<void> => {
    await UpdateConfig(toWailsConfig(cfg))
  },
  handleCloseRequest: async (): Promise<string> => {
    return await HandleCloseRequest()
  },
  setCloseAction: async (action: string): Promise<void> => {
    await SetCloseAction(action)
  },
  gracefulExit: async (): Promise<void> => {
    await GracefulExit()
  },
  // 重启应用：启动新进程后退出当前进程（用于下载完成后自动重启）
  restartApp: async (): Promise<void> => {
    await RestartApp()
  },
  selectImageFile: async (): Promise<string> => {
    return (await SelectImageFile()) as string
  },
  // 版本信息：更新统一由 Microsoft Store 接管，前端仅展示当前版本号
  getAppVersion: async (): Promise<string> => {
    return await GetAppVersion()
  },
  subscribeCloseRequest: (callback: () => void): (() => void) => {
    EventsOn(EventWindowCloseRequest, callback)
    return () => EventsOff(EventWindowCloseRequest)
  },
  // ============ 启动期前端化对话框（区别于 OS 级弹窗） ============
  // 让"启动期必要的弹窗"都改由前端呈现：后端推事件 → 前端弹界面组件 → 用户作答 → RPC 回传。
  // 启动致命错误：后端无法继续启动时推送，前端展示错误卡
  subscribeStartupError: (callback: (err: StartupErrorPayload) => void): (() => void) => {
    EventsOn(EventStartupError, callback)
    return () => EventsOff(EventStartupError)
  },
  // 兜底查询当前是否有待确认的启动致命错误（避免事件因 WebView 未挂载而错过）
  getStartupError: async (): Promise<StartupErrorPayload | null> => {
    return (await GetStartupError()) as StartupErrorPayload | null
  },
  // 用户在错误卡上点"退出"后调用，通知后端可以退出了
  confirmStartupError: async (): Promise<void> => {
    await ConfirmStartupError()
  },
  // ============ TTS 在线合成（Edge TTS / 微软在线神经语音） ============
  // 有网时优先调用，返回 MP3 的 base64 字符串；无网/失败由前端 useTTS 回退本地 Web Speech API。
  // voice 为设置页选的本地发音人名（如 "Microsoft Xiaoxiao"），后端映射为对应在线 Neural 音色；
  // 不传（空）时后端默认使用微软晓晓。语速/音调/音量沿用用户设置的倍率。
  synthesizeSpeech: async (
    text: string,
    voice: string,
    rate: number,
    pitch: number,
    volume: number
  ): Promise<string> => {
    // Wails 运行时把 Go 的 []byte 以 base64 字符串传给前端，但绑定生成器把 []byte 映射为
    // Array<number>（对 []uint8 的映射缺陷），故先用 unknown 中转再做 string 断言。
    return (await SynthesizeSpeech(text, voice ?? '', rate, pitch, volume)) as unknown as string
  }
} as const
