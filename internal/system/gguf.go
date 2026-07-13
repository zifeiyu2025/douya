// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

const ggufMagic = 0x46554747

// GGUF 元数据缓存，避免同一模型文件重复解析
var ggufCache sync.Map // key: resolved path (string), value: *cacheEntry

type cacheEntry struct {
	meta *GGUFMetadata
	err  error
}

// ParseGGUFMetadataCached 返回缓存的 GGUF 元数据，若未缓存则解析并存储
func ParseGGUFMetadataCached(path string) (*GGUFMetadata, error) {
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}
	if v, ok := ggufCache.Load(path); ok {
		e := v.(*cacheEntry)
		return e.meta, e.err
	}
	meta, err := ParseGGUFMetadata(path)
	ggufCache.Store(path, &cacheEntry{meta: meta, err: err})
	return meta, err
}

// InvalidateGGUFCache 清除所有 GGUF 元数据缓存（模型重载时调用）
func InvalidateGGUFCache() {
	ggufCache.Range(func(k, v any) bool {
		ggufCache.Delete(k)
		return true
	})
}

const (
	ggufTypeUINT8   uint32 = 0
	ggufTypeINT8    uint32 = 1
	ggufTypeUINT16  uint32 = 2
	ggufTypeINT16   uint32 = 3
	ggufTypeUINT32  uint32 = 4
	ggufTypeINT32   uint32 = 5
	ggufTypeFLOAT32 uint32 = 6
	ggufTypeBOOL    uint32 = 7
	ggufTypeSTRING  uint32 = 8
	ggufTypeARRAY   uint32 = 9
	ggufTypeUINT64  uint32 = 10
	ggufTypeINT64   uint32 = 11
	ggufTypeFLOAT64 uint32 = 12
)

type GGUFMetadata struct {
	Architecture        string
	BlockCount          int
	EmbeddingLength     int
	ContextLength       int
	FileSize            int64
	ExpertCount         int
	ExpertUsed          int
	HasMTP              bool
	HasReasoning        bool
	SupportsEagle3      bool // 模型是否支持 Eagle3 推测解码（如 Qwen3.5/3.6）
	SizeLabel           string
	NParams             int64
	ChatTemplate        string
	ChatTemplateToolUse string
	KVHeadCount         int
	HeadDimKV           int
	FileType            string // 从 general.file_type 枚举值映射的量化类型名（如 "Q4_K - Medium"）
}

func ParseGGUFMetadata(path string) (*GGUFMetadata, error) {
	kvMap, err := ParseGGUFKV(path)
	if err != nil {
		return nil, err
	}

	meta := &GGUFMetadata{}
	if v, ok := kvMap["general.architecture"].(string); ok {
		meta.Architecture = v
	}

	if v, ok := kvMap["general.size_label"].(string); ok {
		meta.SizeLabel = v
	}
	if n, ok := toInt64(kvMap["general.parameter_count"]); ok {
		meta.NParams = n
	}

	if n, ok := toInt(kvMap["general.file_type"]); ok {
		meta.FileType = fileTypeName(int(n))
	}

	if v, ok := kvMap["tokenizer.chat_template"].(string); ok {
		meta.ChatTemplate = v
	}
	if v, ok := kvMap["tokenizer.chat_template_tool_use"].(string); ok {
		meta.ChatTemplateToolUse = v
	}

	if meta.Architecture != "" {
		prefix := meta.Architecture + "."
		for k, v := range kvMap {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			suffix := k[len(prefix):]
			switch suffix {
			case "block_count":
				if n, ok := toInt(v); ok {
					meta.BlockCount = n
				}
			case "embedding_length":
				if n, ok := toInt(v); ok {
					meta.EmbeddingLength = n
				}
			case "context_length":
				if n, ok := toInt(v); ok {
					meta.ContextLength = n
				}
			case "expert_count":
				if n, ok := toInt(v); ok {
					meta.ExpertCount = n
				}
			case "expert_used_per_token":
				if n, ok := toInt(v); ok {
					meta.ExpertUsed = n
				}
			case "head_count_kv":
				if n, ok := toInt(v); ok {
					meta.KVHeadCount = n
				}
			case "key_length":
				if n, ok := toInt(v); ok {
					meta.HeadDimKV = n
				}
			case "mtp_count":
				if n, ok := toInt(v); ok && n > 0 {
					meta.HasMTP = true
				}
			case "n_mtp":
				if n, ok := toInt(v); ok && n > 0 {
					meta.HasMTP = true
				}
			case "nextn_predict_layers":
				if n, ok := toInt(v); ok && n > 0 {
					meta.HasMTP = true
				}
			}
		}
	}

	if fi, err := os.Stat(path); err == nil {
		meta.FileSize = fi.Size()
	}

	// 优先使用 GGUF 元数据中的 architecture 字段推断思考能力
	if !meta.HasReasoning && meta.Architecture != "" {
		lowerArch := strings.ToLower(meta.Architecture)
		// Template 模式：通过 chat template 的 enable_thinking 控制
		archTemplateKeywords := []string{"gemma2", "gemma4", "qwen3", "llama4", "phi4", "qwen3moe", "qwen3next", "qwen3vl", "qwen3vlmoe", "gemma3n", "mistral3", "mistral4", "granite_speech", "glm4", "chatglm4", "glm4moe", "cohere2moe", "tiny-aya", "qwen35", "qwen35moe", "qwen36", "ernie4-5", "ernie4-5-moe", "minimax-m2", "minicpm5", "smollm3", "hunyuan-moe", "hunyuan-dense", "step35", "kimi-linear", "arcee", "dots1", "dream", "smallthinker"}
		// 安全实践：使用精确匹配避免未来架构误匹配（见安全审查 #29）
		// archTemplateKeywords 中的关键词均为完整架构名（如 "qwen3"、"qwen3moe" 是不同架构），
		// 无需前缀/子串匹配
		if slices.Contains(archTemplateKeywords, lowerArch) {
			meta.HasReasoning = true
		}
		// Reasoning 模式：通过 reasoning 参数控制
		if !meta.HasReasoning {
			archReasoningKeywords := []string{"deepseek3", "deepseek-v3", "deepseek2", "deepseek32", "deepseek4", "deepseek-v4"}
			if matchAnyKeyword(lowerArch, archReasoningKeywords) {
				meta.HasReasoning = true
			}
		}
	}

	// 兜底：通过文件名推断模型是否支持思考能力
	if !meta.HasReasoning {
		lowerName := strings.ToLower(path)
		reasoningKeywords := []string{
			"qwen3", "qwen3.5", "qwen3.6", "qwen35", "qwen36", "gemma-4", "gemma4", "gemma-3", "gemma3", "gemma-2", "llama-4", "llama4",
			"mistral-small-3", "mistral-small3", "mistral-4", "mistral4",
			"deepseek-r1", "deepseek-v2", "deepseek-v3", "deepseek-v4", "deepseek-r", "deepseek-3.2", "deepseek32",
			"phi-4-reasoning", "phi4-reasoning",
			"glm4", "chatglm4", "glm-4-moe", "glm4moe", "cohere2moe", "tiny-aya",
			"ernie-4.5", "ernie4.5", "minimax-m2", "minimaxm2",
			"minicpm5", "minicpm-5",
			"smollm3", "smol-lm3", "hunyuan-moe", "hunyuan-dense", "step3.5", "step3.7", "kimi-linear", "arcee", "dots1", "dream", "smallthinker",
		}
		if matchAnyKeyword(lowerName, reasoningKeywords) {
			meta.HasReasoning = true
		}
	}

	// 兜底：通过文件名推断模型是否支持 MTP（Step 系列支持 flash mtp3）
	if !meta.HasMTP {
		lowerName := strings.ToLower(path)
		mtpKeywords := []string{"step3.5", "step3.7"}
		if matchAnyKeyword(lowerName, mtpKeywords) {
			meta.HasMTP = true
		}
	}

	log.Debug().Str("architecture", meta.Architecture).Bool("has_mtp", meta.HasMTP).Msg("[gguf] MTP detection result")

	// 架构排除：某些模型虽然 GGUF 元数据中有 nextn_predict_layers，
	// 但其 MTP 实现与 llama-server 的 --spec-type draft-mtp 不兼容
	if meta.HasMTP && meta.Architecture != "" {
		lowerArch := strings.ToLower(meta.Architecture)
		// gemma4 的 MTP 层格式与 llama-server 的 spec-decoding 不兼容
		// 强制关闭 HasMTP，改走 ngram-mod 分支
		mtpExcludeKeywords := []string{"gemma4", "gemma-4", "gemma_4"}
		if matchAnyKeyword(lowerArch, mtpExcludeKeywords) {
			log.Warn().
				Str("architecture", meta.Architecture).
				Msg("[gguf] MTP detected but architecture incompatible with llama-server spec-decoding, disabling HasMTP")
			meta.HasMTP = false
		}
	}

	// Eagle3 推测解码支持检测（llama.cpp 最新更新：Eagle3 支持 Qwen3.5/3.6）
	// Eagle3 需要独立的 draft 模型，用户需配置 spec_draft_model 才能启用
	// 这里仅标记模型支持 Eagle3，实际启用在 smartparams.go 中根据用户配置决定
	if meta.Architecture != "" {
		lowerArch := strings.ToLower(meta.Architecture)
		eagle3ArchKeywords := []string{"qwen3.5", "qwen3.6", "qwen35", "qwen36"}
		if matchAnyKeyword(lowerArch, eagle3ArchKeywords) {
			meta.SupportsEagle3 = true
		}
	}
	// 兜底：通过文件名检测 Qwen3.5/3.6
	if !meta.SupportsEagle3 {
		lowerName := strings.ToLower(path)
		eagle3NameKeywords := []string{"qwen3.5", "qwen3.6", "qwen35", "qwen36"}
		if matchAnyKeyword(lowerName, eagle3NameKeywords) {
			meta.SupportsEagle3 = true
		}
	}
	if meta.SupportsEagle3 {
		log.Info().Str("architecture", meta.Architecture).Msg("[gguf] Eagle3 speculative decoding supported (Qwen3.5/3.6 detected)")
	}

	return meta, nil
}

// matchAnyKeyword 检查 target 是否包含 keywords 中的任意关键词（子串匹配）。
//
// 抽取原因（基于 B-1.15+B-3.8）：ParseGGUFMetadata 中 6 处重复的
// `for _, kw := range keywords { if strings.Contains(...) { ...; break } }` 模式，
// 提取为公共函数消除重复，便于后续新增关键词集合时无需复制 for 循环模板。
//
// 注意：archTemplateKeywords 使用 slices.Contains（精确匹配），与本函数语义不同，
// 不可混用——精确匹配是为了避免未来新架构误命中旧关键词（见安全审查 #29）。
//
// 生活类比：像安检员拿着一份"重点物品清单"，逐项核对旅客行李里是否包含清单上的物品，
// 命中任意一项就立即放行（返回 true），全部不命中才返回 false。
func matchAnyKeyword(target string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(target, kw) {
			return true
		}
	}
	return false
}

func ParseGGUFKV(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open gguf: %w", err)
	}
	defer f.Close()

	r := &ggufReader{r: f}

	magic, err := r.readUint32()
	if err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if magic != ggufMagic {
		return nil, fmt.Errorf("not a GGUF file (magic: 0x%08x)", magic)
	}

	version, err := r.readUint32()
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}
	if version < 2 || version > 3 {
		return nil, fmt.Errorf("unsupported GGUF version: %d", version)
	}

	if _, err := r.readUint64(); err != nil {
		return nil, fmt.Errorf("read n_tensors: %w", err)
	}

	nKV, err := r.readUint64()
	if err != nil {
		return nil, fmt.Errorf("read n_kv: %w", err)
	}

	kvMap := make(map[string]any, nKV)
	for range nKV {
		key, err := r.readString()
		if err != nil {
			break
		}
		valueType, err := r.readUint32()
		if err != nil {
			break
		}
		value, err := r.readValue(valueType)
		if err != nil {
			break
		}
		kvMap[key] = value
	}

	return kvMap, nil
}

type ggufReader struct {
	r io.Reader
}

func (g *ggufReader) readUint8() (uint8, error) {
	var v uint8
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readUint16() (uint16, error) {
	var v uint16
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readInt16() (int16, error) {
	var v int16
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readUint32() (uint32, error) {
	var v uint32
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readInt32() (int32, error) {
	var v int32
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readFloat32() (float32, error) {
	var v float32
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readUint64() (uint64, error) {
	var v uint64
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readInt64() (int64, error) {
	var v int64
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readFloat64() (float64, error) {
	var v float64
	err := binary.Read(g.r, binary.LittleEndian, &v)
	return v, err
}

func (g *ggufReader) readString() (string, error) {
	length, err := g.readUint64()
	if err != nil {
		return "", err
	}
	if length > 1<<20 {
		return "", fmt.Errorf("string too long: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(g.r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (g *ggufReader) readValue(valueType uint32) (any, error) {
	switch valueType {
	case ggufTypeUINT8:
		return g.readUint8()
	case ggufTypeINT8:
		v, err := g.readUint8()
		return int8(v), err
	case ggufTypeUINT16:
		return g.readUint16()
	case ggufTypeINT16:
		return g.readInt16()
	case ggufTypeUINT32:
		return g.readUint32()
	case ggufTypeINT32:
		return g.readInt32()
	case ggufTypeFLOAT32:
		return g.readFloat32()
	case ggufTypeBOOL:
		v, err := g.readUint8()
		return v != 0, err
	case ggufTypeSTRING:
		return g.readString()
	case ggufTypeARRAY:
		elemType, err := g.readUint32()
		if err != nil {
			return nil, err
		}
		length, err := g.readUint64()
		if err != nil {
			return nil, err
		}
		return nil, g.skipArrayElements(elemType, length)
	case ggufTypeUINT64:
		return g.readUint64()
	case ggufTypeINT64:
		return g.readInt64()
	case ggufTypeFLOAT64:
		return g.readFloat64()
	default:
		return nil, fmt.Errorf("unknown GGUF value type: %d", valueType)
	}
}

func (g *ggufReader) skipArrayElements(elemType uint32, count uint64) error {
	for range count {
		switch elemType {
		case ggufTypeUINT8, ggufTypeINT8, ggufTypeBOOL:
			if _, err := g.readUint8(); err != nil {
				return err
			}
		case ggufTypeUINT16, ggufTypeINT16:
			if _, err := g.readUint16(); err != nil {
				return err
			}
		case ggufTypeUINT32, ggufTypeINT32, ggufTypeFLOAT32:
			if _, err := g.readUint32(); err != nil {
				return err
			}
		case ggufTypeUINT64, ggufTypeINT64, ggufTypeFLOAT64:
			if _, err := g.readUint64(); err != nil {
				return err
			}
		case ggufTypeSTRING:
			if _, err := g.readString(); err != nil {
				return err
			}
		case ggufTypeARRAY:
			innerElemType, err := g.readUint32()
			if err != nil {
				return err
			}
			innerLength, err := g.readUint64()
			if err != nil {
				return err
			}
			if err := g.skipArrayElements(innerElemType, innerLength); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown array element type: %d", elemType)
		}
	}
	return nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case uint8:
		return int(n), true
	case int8:
		return int(n), true
	case uint16:
		return int(n), true
	case int16:
		return int(n), true
	case uint32:
		return int(n), true
	case int32:
		return int(n), true
	case uint64:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case uint8:
		return int64(n), true
	case int8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case int16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case int32:
		return int64(n), true
	case uint64:
		return int64(n), true
	case int64:
		return n, true
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// fileTypeName 将 GGUF general.file_type 枚举值映射为量化类型名
// 参考 llama.cpp 的 llama_ftype 枚举定义
func fileTypeName(ftype int) string {
	switch ftype {
	case 0:
		return "F32"
	case 1:
		return "F16"
	case 2:
		return "Q4_0"
	case 3:
		return "Q4_1"
	case 7:
		return "Q8_0"
	case 8:
		return "Q5_0"
	case 9:
		return "Q5_1"
	case 10:
		return "Q2_K"
	case 11:
		return "Q3_K - Small"
	case 12:
		return "Q3_K - Medium"
	case 13:
		return "Q3_K - Large"
	case 14:
		return "Q4_K - Small"
	case 15:
		return "Q4_K - Medium"
	case 16:
		return "Q5_K - Small"
	case 17:
		return "Q5_K - Medium"
	case 18:
		return "Q6_K"
	case 19:
		return "IQ2_XXS"
	case 20:
		return "IQ2_XS"
	case 21:
		return "Q2_K - Small"
	case 22:
		return "IQ3_XS"
	case 23:
		return "IQ3_XXS"
	case 24:
		return "IQ1_S"
	case 25:
		return "IQ4_NL"
	case 26:
		return "IQ3_S"
	case 27:
		return "IQ3_M"
	case 28:
		return "IQ2_S"
	case 29:
		return "IQ2_M"
	case 30:
		return "IQ4_XS"
	case 31:
		return "IQ1_M"
	default:
		return ""
	}
}
