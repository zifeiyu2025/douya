import { createDiscreteApi } from 'naive-ui'

// 单例：整个应用共享同一个 discrete API 实例（包含 message 和 dialog）
// 避免多处 createDiscreteApi 创建独立 Vue 应用实例导致主题不一致和内存浪费
const { message, dialog } = createDiscreteApi(['message', 'dialog'])

export { message as discreteMessage, dialog as discreteDialog }
