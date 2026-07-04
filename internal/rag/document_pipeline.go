package rag

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Chunk struct {
	Content  string
	Metadata map[string]string
}

type ChunkConfig struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
		Separators:   []string{"\n\n", "\n", "。", ".", "！", "？", "；", ";", " "},
	}
}

// estimateTokens 粗略估算文本的 token 数
// 中文约 1 字 ≈ 1.5 token，英文约 1 词 ≈ 1.3 token
// 简化估算：rune 数 × 0.7 作为 token 近似值
func estimateTokens(text string) int {
	return int(float64(utf8.RuneCountInString(text)) * 0.7)
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

// sanitizeFileName 过滤文件名中的控制字符并限制长度。
// 生活类比：像快递员在包裹上重新写收件人姓名时，跳过那些看不见的特殊符号（如换行、响铃等控制字符），
// 用下划线替换，避免污染后续的存储系统（如 JSON 序列化、数据库索引）。
// fileName 仅用于扩展名校验和 metadata.source 回显，不作为存储路径使用。
const maxFileNameLength = 255

func sanitizeFileName(fileName string) string {
	// 过滤控制字符（ASCII < 0x20 和 0x7f DEL），替换为下划线
	fileName = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, fileName)
	// 限制长度，避免超长文件名撑大元数据存储
	if len(fileName) > maxFileNameLength {
		// 保留扩展名，截断主体部分
		ext := ""
		if dot := strings.LastIndex(fileName, "."); dot >= 0 && dot > len(fileName)-20 {
			ext = fileName[dot:]
		}
		fileName = fileName[:maxFileNameLength-len(ext)] + ext
	}
	return fileName
}

type IngestResult struct {
	CollectionName string
	DocumentID     string
	TotalChunks    int
	StoredChunks   int
}

const embedBatchSize = 32

// maxChunksPerDocument 限制单文档切分后的最大 chunk 数，避免因超大文档
// 一次性产生过多 chunk 导致内存暴涨或写入耗时过长（任务 33）。
const maxChunksPerDocument = 10000

// chunkWriteFailureThreshold 是 chunk 写入失败比例的容忍上限。
// 超过该阈值时 IngestDocumentWithMeta 会回滚已写入的数据并返回错误（任务 5）。
const chunkWriteFailureThreshold = 0.1

// chunkWriteErrorHook 仅供测试使用：若非 nil，则在每个 chunk 写入前调用，
// 返回 error 时该 chunk 视为写入失败。生产环境为 nil，零开销。
var chunkWriteErrorHook func(collection, id string) error

// ChunkDocument 使用递归字符分块策略将文本拆分为块。
// 策略：按分隔符优先级递归切分 —— 先用高级分隔符(段落)切分，
// 超过 chunkSize 的块再用低级分隔符(句子)继续切分，依此类推。
// 每个块保留 chunkOverlap 的重叠以保证上下文连贯。
func ChunkDocument(text string, cfg ChunkConfig) []Chunk {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 512
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 64
	}
	if cfg.ChunkOverlap >= cfg.ChunkSize {
		cfg.ChunkOverlap = cfg.ChunkSize / 4
	}
	if len(cfg.Separators) == 0 {
		cfg.Separators = []string{"\n\n", "\n", "。", ".", "！", "？", "；", ";", " "}
	}

	if text == "" {
		return nil
	}

	text = cleanText(text)
	return recursiveChunk(text, cfg.Separators, cfg.ChunkSize, cfg.ChunkOverlap)
}

// recursiveChunk 递归字符分块核心逻辑
func recursiveChunk(text string, separators []string, chunkSize, chunkOverlap int) []Chunk {
	// 没有更多分隔符可用时，按字符硬切
	if len(separators) == 0 {
		return hardSplit(text, chunkSize, chunkOverlap)
	}

	sep := separators[0]
	remainingSeps := separators[1:]
	parts := strings.Split(text, sep)

	var chunks []Chunk
	var current strings.Builder

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 当前块加入此部分后是否超过大小限制
		combinedLen := estimateTokens(current.String()) + estimateTokens(part)
		if current.Len() > 0 && combinedLen > chunkSize {
			// 当前块已满，先保存
			chunks = append(chunks, Chunk{Content: current.String()})

			// 保留重叠部分
			overlapText := takeOverlap(current.String(), chunkOverlap)
			current.Reset()
			current.WriteString(overlapText)
		}

		// 检查单个部分是否就超过大小限制
		if estimateTokens(part) > chunkSize && len(remainingSeps) > 0 {
			// 先把当前积累的内容保存
			if current.Len() > 0 {
				chunks = append(chunks, Chunk{Content: current.String()})
				current.Reset()
			}
			// 递归用下一级分隔符切分
			subChunks := recursiveChunk(part, remainingSeps, chunkSize, chunkOverlap)
			chunks = append(chunks, subChunks...)
			continue
		}

		if current.Len() > 0 {
			current.WriteString(sep)
		}
		current.WriteString(part)
	}

	if current.Len() > 0 {
		chunks = append(chunks, Chunk{Content: current.String()})
	}

	return chunks
}

// takeOverlap 从文本末尾取约 overlapTokens 个 token 的重叠文本
func takeOverlap(text string, overlapTokens int) string {
	runes := []rune(text)
	// overlapTokens 对应的字符数约 overlapTokens / 0.7
	overlapChars := int(float64(overlapTokens) / 0.7)
	if overlapChars >= len(runes) {
		return text
	}
	start := len(runes) - overlapChars
	return string(runes[start:])
}

// hardSplit 按字符硬切（最后兜底）
func hardSplit(text string, chunkSize, chunkOverlap int) []Chunk {
	runes := []rune(text)
	// chunkSize token 对应的字符数
	charSize := int(float64(chunkSize) / 0.7)
	charOverlap := int(float64(chunkOverlap) / 0.7)
	if charOverlap >= charSize {
		charOverlap = charSize / 4
	}

	var chunks []Chunk
	for i := 0; i < len(runes); i += charSize - charOverlap {
		end := min(i+charSize, len(runes))
		chunks = append(chunks, Chunk{Content: string(runes[i:end])})
		if end >= len(runes) {
			break
		}
	}
	return chunks
}

func IngestDocument(ctx context.Context, vs *VectorStore, embedder Embedder, collectionName string, text string, chunkCfg ChunkConfig) (*IngestResult, error) {
	return IngestDocumentWithMeta(ctx, vs, nil, embedder, collectionName, "", text, "", 0, "", chunkCfg)
}

func IngestFile(ctx context.Context, vs *VectorStore, ds *DocumentStore, embedder Embedder,
	collectionName string, fileName string, fileData []byte, mimeType string, chunkCfg ChunkConfig) (*IngestResult, error) {

	text, err := ParseFileFromBytes(fileData, fileName)
	if err != nil {
		return nil, fmt.Errorf("parse file %q: %w", fileName, err)
	}
	if text == "" {
		return nil, fmt.Errorf("file %q produced no text content", fileName)
	}

	return IngestDocumentWithMeta(ctx, vs, ds, embedder, collectionName, "", text, fileName, int64(len(fileData)), mimeType, chunkCfg)
}

func IngestFileFromBase64(ctx context.Context, vs *VectorStore, ds *DocumentStore, embedder Embedder,
	collectionName string, fileName string, base64Data string, mimeType string, chunkCfg ChunkConfig) (*IngestResult, error) {

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("decode base64 for %q: %w", fileName, err)
	}

	return IngestFile(ctx, vs, ds, embedder, collectionName, fileName, data, mimeType, chunkCfg)
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = multiNewlineRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
