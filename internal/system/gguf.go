// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
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
	ggufCache.Range(func(k, v interface{}) bool {
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
	Architecture         string
	BlockCount           int
	EmbeddingLength      int
	ContextLength        int
	FileSize             int64
	ExpertCount          int
	ExpertUsed           int
	HasMTP               bool
	HasReasoning         bool
	SupportsEagle3       bool // 模型是否支持 Eagle3 推测解码（如 Qwen3.5/3.6）
	SizeLabel            string
	NParams              int64
	ChatTemplate         string
	ChatTemplateToolUse  string
	KVHeadCount          int
	HeadDimKV            int
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
		for _, kw := range archTemplateKeywords {
			if strings.Contains(lowerArch, kw) {
				meta.HasReasoning = true
				break
			}
		}
		// Reasoning 模式：通过 reasoning 参数控制
		if !meta.HasReasoning {
			archReasoningKeywords := []string{"deepseek3", "deepseek-v3", "deepseek2", "deepseek32", "deepseek4", "deepseek-v4"}
			for _, kw := range archReasoningKeywords {
				if strings.Contains(lowerArch, kw) {
					meta.HasReasoning = true
					break
				}
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
		for _, kw := range reasoningKeywords {
			if strings.Contains(lowerName, kw) {
				meta.HasReasoning = true
				break
			}
		}
	}

	// 兜底：通过文件名推断模型是否支持 MTP（Step 系列支持 flash mtp3）
	if !meta.HasMTP {
		lowerName := strings.ToLower(path)
		mtpKeywords := []string{"step3.5", "step3.7"}
		for _, kw := range mtpKeywords {
			if strings.Contains(lowerName, kw) {
				meta.HasMTP = true
				break
			}
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
		for _, kw := range mtpExcludeKeywords {
			if strings.Contains(lowerArch, kw) {
				log.Warn().
					Str("architecture", meta.Architecture).
					Msg("[gguf] MTP detected but architecture incompatible with llama-server spec-decoding, disabling HasMTP")
				meta.HasMTP = false
				break
			}
		}
	}

	// Eagle3 推测解码支持检测（llama.cpp 最新更新：Eagle3 支持 Qwen3.5/3.6）
	// Eagle3 需要独立的 draft 模型，用户需配置 spec_draft_model 才能启用
	// 这里仅标记模型支持 Eagle3，实际启用在 smartparams.go 中根据用户配置决定
	if meta.Architecture != "" {
		lowerArch := strings.ToLower(meta.Architecture)
		eagle3ArchKeywords := []string{"qwen3.5", "qwen3.6", "qwen35", "qwen36"}
		for _, kw := range eagle3ArchKeywords {
			if strings.Contains(lowerArch, kw) {
				meta.SupportsEagle3 = true
				break
			}
		}
	}
	// 兜底：通过文件名检测 Qwen3.5/3.6
	if !meta.SupportsEagle3 {
		lowerName := strings.ToLower(path)
		eagle3NameKeywords := []string{"qwen3.5", "qwen3.6", "qwen35", "qwen36"}
		for _, kw := range eagle3NameKeywords {
			if strings.Contains(lowerName, kw) {
				meta.SupportsEagle3 = true
				break
			}
		}
	}
	if meta.SupportsEagle3 {
		log.Info().Str("architecture", meta.Architecture).Msg("[gguf] Eagle3 speculative decoding supported (Qwen3.5/3.6 detected)")
	}

	return meta, nil
}

func ParseGGUFKV(path string) (map[string]interface{}, error) {
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

	kvMap := make(map[string]interface{}, nKV)
	for i := uint64(0); i < nKV; i++ {
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

func (g *ggufReader) readValue(valueType uint32) (interface{}, error) {
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
	for i := uint64(0); i < count; i++ {
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

func toInt(v interface{}) (int, bool) {
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

func toInt64(v interface{}) (int64, bool) {
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
