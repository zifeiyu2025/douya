/*
 * knowledgeContext: 知识库域上下文类型与注入令牌
 * 类型直接从 useKnowledge 返回值推导，新增成员自动同步。
 */
import type { InjectionKey } from 'vue'
import type { KnowledgeContext } from './useKnowledge'

// re-export：子组件统一从本文件取上下文类型
export type { KnowledgeContext }

export const KNOWLEDGE_CONTEXT_KEY: InjectionKey<KnowledgeContext> = Symbol('knowledgeContext')
