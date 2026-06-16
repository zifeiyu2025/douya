/**
 * 集中类型定义：搜索/知识库相关
 */

export interface SearchResultItem {
    title: string
    url: string
    snippet: string
}

export interface CollectionInfo {
    name: string
    dim: number
    vector_count: number
}

export interface DocumentMeta {
    id: string
    collection: string
    file_name: string
    file_size: number
    mime_type: string
    chunk_count: number
    ingested_at: string
    tags?: Record<string, string>
}
