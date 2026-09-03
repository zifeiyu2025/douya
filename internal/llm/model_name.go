// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import "strings"

// embeddingNameMarkers 常见嵌入模型名片段（小写匹配）。
// 与前端 frontend/src/utils/model.ts 的 EMBEDDING_NAME_MARKERS 保持同步，
// 用于兜底识别"只能做向量化/检索、不能聊天"的嵌入模型，
// 即使能力检测（text_generation）暂时拿不到，也能拦住常见误选。
var embeddingNameMarkers = []string{
	"bge",
	"text-embedding",
	"text_embedding",
	"embedding",
	"embeddings",
	"gte-",
	"m3e",
	"-e5-",
	"e5-",
	"jina-embeddings",
	"nomic-embed",
	"mxbai-embed",
	"qwen3-embedding",
	"bce-embedding",
	"acge",
	"stella",
	"sentence-",
	"conan-embed",
}

// IsEmbeddingModelName 判断模型名是否属于嵌入模型（不能聊天，只能做向量化/检索）。
// 使用小写匹配，命中常见嵌入模型名称片段即视为嵌入模型。
func IsEmbeddingModelName(name string) bool {
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	for _, m := range embeddingNameMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
