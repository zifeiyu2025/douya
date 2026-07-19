// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// ggufKV 表示一个 GGUF 键值对
type ggufKV struct {
	key       string
	valueType uint32
	value     any
}

// ggufBuilder 帮助构建测试用的 GGUF 文件
// 生活类比：就像一个填表机，我们往里面塞键值对，它最后输出一个二进制文件
type ggufBuilder struct {
	version uint32
	kvs     []ggufKV
}

// newGGUFBuilder 创建一个新的 GGUF 构建器，默认版本为 3
func newGGUFBuilder() *ggufBuilder {
	return &ggufBuilder{version: 3}
}

// addString 添加一个字符串类型的键值对
func (b *ggufBuilder) addString(key, value string) *ggufBuilder {
	b.kvs = append(b.kvs, ggufKV{key: key, valueType: ggufTypeSTRING, value: value})
	return b
}

// addUint32 添加一个 uint32 类型的键值对
func (b *ggufBuilder) addUint32(key string, value uint32) *ggufBuilder {
	b.kvs = append(b.kvs, ggufKV{key: key, valueType: ggufTypeUINT32, value: value})
	return b
}

// addFloat32 添加一个 float32 类型的键值对
func (b *ggufBuilder) addFloat32(key string, value float32) *ggufBuilder {
	b.kvs = append(b.kvs, ggufKV{key: key, valueType: ggufTypeFLOAT32, value: value})
	return b
}

// writeTo 将构建好的 GGUF 内容写入 io.Writer
func (b *ggufBuilder) writeTo(w io.Writer) error {
	// 写入 magic number
	if err := binary.Write(w, binary.LittleEndian, uint32(ggufMagic)); err != nil {
		return err
	}
	// 写入版本号
	if err := binary.Write(w, binary.LittleEndian, b.version); err != nil {
		return err
	}
	// 写入 tensor 数量（测试中不使用 tensor，设为 0）
	if err := binary.Write(w, binary.LittleEndian, uint64(0)); err != nil {
		return err
	}
	// 写入 KV 数量
	if err := binary.Write(w, binary.LittleEndian, uint64(len(b.kvs))); err != nil {
		return err
	}
	// 依次写入每个 KV
	for _, kv := range b.kvs {
		// 写入 key（先长度后内容）
		keyBytes := []byte(kv.key)
		if err := binary.Write(w, binary.LittleEndian, uint64(len(keyBytes))); err != nil {
			return err
		}
		if _, err := w.Write(keyBytes); err != nil {
			return err
		}
		// 写入 value 类型
		if err := binary.Write(w, binary.LittleEndian, kv.valueType); err != nil {
			return err
		}
		// 根据 value 类型写入具体值
		if err := writeGGUFValue(w, kv); err != nil {
			return err
		}
	}
	return nil
}

// writeGGUFValue 根据类型写入 GGUF 值
func writeGGUFValue(w io.Writer, kv ggufKV) error {
	switch kv.valueType {
	case ggufTypeSTRING:
		s := kv.value.(string)
		sBytes := []byte(s)
		if err := binary.Write(w, binary.LittleEndian, uint64(len(sBytes))); err != nil {
			return err
		}
		if _, err := w.Write(sBytes); err != nil {
			return err
		}
	case ggufTypeUINT32:
		if err := binary.Write(w, binary.LittleEndian, kv.value.(uint32)); err != nil {
			return err
		}
	case ggufTypeINT32:
		if err := binary.Write(w, binary.LittleEndian, kv.value.(int32)); err != nil {
			return err
		}
	case ggufTypeUINT64:
		if err := binary.Write(w, binary.LittleEndian, kv.value.(uint64)); err != nil {
			return err
		}
	case ggufTypeFLOAT32:
		if err := binary.Write(w, binary.LittleEndian, kv.value.(float32)); err != nil {
			return err
		}
	case ggufTypeUINT8:
		if err := binary.Write(w, binary.LittleEndian, kv.value.(uint8)); err != nil {
			return err
		}
	}
	return nil
}

// buildTempGGUFFile 构建一个临时 GGUF 文件并返回其路径
// 调用者负责在测试结束后删除该文件
func buildTempGGUFFile(t *testing.T, b *ggufBuilder) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.gguf")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer f.Close()
	if err := b.writeTo(f); err != nil {
		t.Fatalf("写入 GGUF 内容失败: %v", err)
	}
	return f.Name()
}

// buildTempGGUFFileWithSuffix 构建一个带有指定后缀名的临时 GGUF 文件
// 用于测试文件名推断逻辑（如 HasReasoning 的文件名兜底）
func buildTempGGUFFileWithSuffix(t *testing.T, b *ggufBuilder, suffix string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, suffix)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer f.Close()
	if err := b.writeTo(f); err != nil {
		t.Fatalf("写入 GGUF 内容失败: %v", err)
	}
	return path
}

// TestParseGGUFMetadata_EmptyPath 测试空路径的错误处理
func TestParseGGUFMetadata_EmptyPath(t *testing.T) {
	_, err := ParseGGUFMetadata("")
	if err == nil {
		t.Error("空路径应返回错误，但返回了 nil")
	}
}

// TestParseGGUFMetadata_NonExistentFile 测试不存在的文件
func TestParseGGUFMetadata_NonExistentFile(t *testing.T) {
	_, err := ParseGGUFMetadata("non-existent-file.gguf")
	if err == nil {
		t.Error("不存在的文件应返回错误，但返回了 nil")
	}
}

// TestParseGGUFMetadata_InvalidMagic 测试无效的 magic number
func TestParseGGUFMetadata_InvalidMagic(t *testing.T) {
	// 创建一个内容不是 GGUF 格式的临时文件
	f, err := os.CreateTemp("", "bad-magic-*.gguf")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	// 写入错误的 magic number
	if err := binary.Write(f, binary.LittleEndian, uint32(0x12345678)); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	_, err = ParseGGUFMetadata(f.Name())
	if err == nil {
		t.Error("无效的 magic number 应返回错误，但返回了 nil")
	}
}

// TestParseGGUFMetadata_ValidFile 测试有效的 GGUF 文件解析
func TestParseGGUFMetadata_ValidFile(t *testing.T) {
	b := newGGUFBuilder().
		addString("general.architecture", "qwen3").
		addUint32("qwen3.block_count", 28).
		addUint32("qwen3.embedding_length", 1024).
		addUint32("qwen3.context_length", 4096).
		addString("general.size_label", "1.7B")
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	meta, err := ParseGGUFMetadata(path)
	if err != nil {
		t.Fatalf("解析有效 GGUF 文件失败: %v", err)
	}
	if meta.Architecture != "qwen3" {
		t.Errorf("Architecture 期望 qwen3，实际 %s", meta.Architecture)
	}
	if meta.BlockCount != 28 {
		t.Errorf("BlockCount 期望 28，实际 %d", meta.BlockCount)
	}
	if meta.EmbeddingLength != 1024 {
		t.Errorf("EmbeddingLength 期望 1024，实际 %d", meta.EmbeddingLength)
	}
	if meta.ContextLength != 4096 {
		t.Errorf("ContextLength 期望 4096，实际 %d", meta.ContextLength)
	}
	if meta.SizeLabel != "1.7B" {
		t.Errorf("SizeLabel 期望 1.7B，实际 %s", meta.SizeLabel)
	}
}

// TestParseGGUFMetadata_Architectures 测试不同架构的识别
// 通过 table-driven 方式测试多种架构的 KV 解析
func TestParseGGUFMetadata_Architectures(t *testing.T) {
	tests := []struct {
		name          string
		architecture  string
		blockCount    uint32
		embedLength   uint32
		contextLength uint32
	}{
		{"qwen3", "qwen3", 28, 1024, 4096},
		{"gemma4", "gemma4", 42, 2048, 8192},
		{"llama4", "llama4", 32, 1536, 4096},
		{"deepseek3", "deepseek3", 61, 2048, 8192},
		{"gemma2", "gemma2", 26, 1024, 4096},
		{"phi4", "phi4", 40, 2048, 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGGUFBuilder().
				addString("general.architecture", tt.architecture).
				addUint32(tt.architecture+".block_count", tt.blockCount).
				addUint32(tt.architecture+".embedding_length", tt.embedLength).
				addUint32(tt.architecture+".context_length", tt.contextLength)
			path := buildTempGGUFFile(t, b)
			defer os.Remove(path)

			meta, err := ParseGGUFMetadata(path)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if meta.Architecture != tt.architecture {
				t.Errorf("Architecture 期望 %s，实际 %s", tt.architecture, meta.Architecture)
			}
			if meta.BlockCount != int(tt.blockCount) {
				t.Errorf("BlockCount 期望 %d，实际 %d", tt.blockCount, meta.BlockCount)
			}
			if meta.EmbeddingLength != int(tt.embedLength) {
				t.Errorf("EmbeddingLength 期望 %d，实际 %d", tt.embedLength, meta.EmbeddingLength)
			}
			if meta.ContextLength != int(tt.contextLength) {
				t.Errorf("ContextLength 期望 %d，实际 %d", tt.contextLength, meta.ContextLength)
			}
		})
	}
}

// TestParseGGUFMetadataCached_EmptyPath 测试缓存的空路径处理
func TestParseGGUFMetadataCached_EmptyPath(t *testing.T) {
	_, err := ParseGGUFMetadataCached("")
	if err == nil {
		t.Error("空路径应返回错误，但返回了 nil")
	}
}

// TestParseGGUFMetadataCached_CacheHit 测试缓存命中机制
// 同一路径不应重复解析：第一次解析后删除文件，第二次仍应返回缓存结果
func TestParseGGUFMetadataCached_CacheHit(t *testing.T) {
	InvalidateGGUFCache()
	defer InvalidateGGUFCache()

	b := newGGUFBuilder().
		addString("general.architecture", "qwen3").
		addUint32("qwen3.block_count", 28)
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	// 第一次调用：解析并缓存
	meta1, err := ParseGGUFMetadataCached(path)
	if err != nil {
		t.Fatalf("第一次调用失败: %v", err)
	}
	if meta1.Architecture != "qwen3" {
		t.Fatalf("第一次调用 Architecture 期望 qwen3，实际 %s", meta1.Architecture)
	}

	// 删除文件，如果再次解析会失败
	if err := os.Remove(path); err != nil {
		t.Fatalf("删除文件失败: %v", err)
	}

	// 第二次调用：应从缓存返回，不会因为文件不存在而失败
	meta2, err := ParseGGUFMetadataCached(path)
	if err != nil {
		t.Fatalf("第二次调用应从缓存返回，但返回错误: %v", err)
	}
	if meta2.Architecture != "qwen3" {
		t.Errorf("第二次调用 Architecture 期望 qwen3，实际 %s", meta2.Architecture)
	}
	if meta2.BlockCount != 28 {
		t.Errorf("第二次调用 BlockCount 期望 28，实际 %d", meta2.BlockCount)
	}
}

// TestParseGGUFMetadataCached_CacheStoresError 测试缓存存储错误
// 第一次调用失败时，错误也应被缓存
func TestParseGGUFMetadataCached_CacheStoresError(t *testing.T) {
	InvalidateGGUFCache()
	defer InvalidateGGUFCache()

	nonExistentPath := "non-existent-cache-test.gguf"

	// 第一次调用：文件不存在，返回错误并缓存
	_, err1 := ParseGGUFMetadataCached(nonExistentPath)
	if err1 == nil {
		t.Fatal("第一次调用不存在的文件应返回错误")
	}

	// 创建该文件
	b := newGGUFBuilder().addString("general.architecture", "qwen3")
	f, err := os.Create(nonExistentPath)
	if err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	defer os.Remove(nonExistentPath)
	if err := b.writeTo(f); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	f.Close()

	// 第二次调用：应从缓存返回错误，而不是重新解析成功
	_, err2 := ParseGGUFMetadataCached(nonExistentPath)
	if err2 == nil {
		t.Error("第二次调用应返回缓存的错误，但返回了 nil")
	}
}

// TestInvalidateGGUFCache 测试清除缓存
func TestInvalidateGGUFCache(t *testing.T) {
	InvalidateGGUFCache()
	defer InvalidateGGUFCache()

	b := newGGUFBuilder().addString("general.architecture", "qwen3")
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	// 第一次调用：缓存结果
	_, err := ParseGGUFMetadataCached(path)
	if err != nil {
		t.Fatalf("第一次调用失败: %v", err)
	}

	// 删除文件
	if err := os.Remove(path); err != nil {
		t.Fatalf("删除文件失败: %v", err)
	}

	// 清除缓存
	InvalidateGGUFCache()

	// 第二次调用：缓存已被清除，应重新解析（文件已删除，应返回错误）
	_, err = ParseGGUFMetadataCached(path)
	if err == nil {
		t.Error("清除缓存后，文件已删除，应返回错误，但返回了 nil")
	}
}

// TestParseGGUFMetadata_HasReasoning 测试 HasReasoning 检测逻辑
// 通过架构关键词和文件名兜底两种方式检测推理能力
func TestParseGGUFMetadata_HasReasoning(t *testing.T) {
	tests := []struct {
		name          string
		architecture  string
		fileSuffix    string // 文件名后缀，用于测试文件名兜底逻辑
		wantReasoning bool
	}{
		// 通过架构关键词检测（Template 模式）
		{"qwen3 架构", "qwen3", "model.gguf", true},
		{"gemma4 架构", "gemma4", "model.gguf", true},
		{"llama4 架构", "llama4", "model.gguf", true},
		{"gemma2 架构", "gemma2", "model.gguf", true},
		{"phi4 架构", "phi4", "model.gguf", true},
		{"qwen3moe 架构", "qwen3moe", "model.gguf", true},
		{"qwen3vl 架构", "qwen3vl", "model.gguf", true},
		// 通过架构关键词检测（Reasoning 模式）
		{"deepseek3 架构", "deepseek3", "model.gguf", true},
		{"deepseek2 架构", "deepseek2", "model.gguf", true},
		// 无推理能力的架构
		{"未知架构", "unknown", "model.gguf", false},
		// 文件名兜底：架构未知但文件名包含关键词
		{"文件名含 qwen3", "unknown", "qwen3-model.gguf", true},
		{"文件名含 gemma4", "unknown", "gemma4-model.gguf", true},
		{"文件名含 llama4", "unknown", "llama4-model.gguf", true},
		{"文件名含 deepseek-r1", "unknown", "deepseek-r1.gguf", true},
		{"文件名含 deepseek-v3", "unknown", "deepseek-v3.gguf", true},
		{"文件名含 phi4-reasoning", "unknown", "phi4-reasoning.gguf", true},
		// 架构和文件名都不包含关键词
		{"无任何关键词", "unknown", "plain.gguf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGGUFBuilder()
			if tt.architecture != "" {
				b.addString("general.architecture", tt.architecture)
			}
			path := buildTempGGUFFileWithSuffix(t, b, tt.fileSuffix)
			// 注意：buildTempGGUFFileWithSuffix 已使用 t.TempDir()，无需手动删除

			meta, err := ParseGGUFMetadata(path)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if meta.HasReasoning != tt.wantReasoning {
				t.Errorf("HasReasoning 期望 %v，实际 %v（架构: %s，路径: %s）",
					tt.wantReasoning, meta.HasReasoning, tt.architecture, path)
			}
		})
	}
}

// TestParseGGUFMetadata_HasMTP 测试 HasMTP 检测逻辑
// 通过不同的 KV 键检测 MTP（Multi-Token Prediction）
func TestParseGGUFMetadata_HasMTP(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		mtpKey   string // MTP 相关的 KV 键名
		mtpValue uint32 // MTP 相关的 KV 值
		wantMTP  bool
	}{
		// mtp_count > 0 应检测到 MTP
		{"mtp_count=1", "qwen3", "qwen3.mtp_count", 1, true},
		{"mtp_count=0", "qwen3", "qwen3.mtp_count", 0, false},
		// n_mtp > 0 应检测到 MTP
		{"n_mtp=1", "qwen3", "qwen3.n_mtp", 1, true},
		// nextn_predict_layers > 0 应检测到 MTP
		{"nextn_predict_layers=1", "qwen3", "qwen3.nextn_predict_layers", 1, true},
		// 无 MTP 键
		{"无 MTP 键", "qwen3", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGGUFBuilder().addString("general.architecture", tt.arch)
			if tt.mtpKey != "" {
				b.addUint32(tt.mtpKey, tt.mtpValue)
			}
			path := buildTempGGUFFileWithSuffix(t, b, "model.gguf")

			meta, err := ParseGGUFMetadata(path)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if meta.HasMTP != tt.wantMTP {
				t.Errorf("HasMTP 期望 %v，实际 %v", tt.wantMTP, meta.HasMTP)
			}
		})
	}
}

// TestParseGGUFMetadata_HasMTP_Gemma4Exclusion 测试 gemma4 架构的 MTP 排除逻辑
// gemma4 虽然检测到 MTP，但因与 llama-server 不兼容而被强制关闭
func TestParseGGUFMetadata_HasMTP_Gemma4Exclusion(t *testing.T) {
	tests := []struct {
		name    string
		arch    string
		wantMTP bool
	}{
		// gemma4 虽有 nextn_predict_layers，但应被排除
		{"gemma4 应排除 MTP", "gemma4", false},
		{"gemma-4 应排除 MTP", "gemma-4", false},
		// qwen3 不在排除列表，应保留 MTP
		{"qwen3 应保留 MTP", "qwen3", true},
		// deepseek3 不在排除列表，应保留 MTP
		{"deepseek3 应保留 MTP", "deepseek3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGGUFBuilder().
				addString("general.architecture", tt.arch).
				addUint32(tt.arch+".nextn_predict_layers", 1)
			path := buildTempGGUFFileWithSuffix(t, b, "model.gguf")

			meta, err := ParseGGUFMetadata(path)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if meta.HasMTP != tt.wantMTP {
				t.Errorf("HasMTP 期望 %v，实际 %v（架构: %s）", tt.wantMTP, meta.HasMTP, tt.arch)
			}
		})
	}
}

// TestParseGGUFMetadata_ExpertInfo 测试专家信息解析（MoE 模型）
func TestParseGGUFMetadata_ExpertInfo(t *testing.T) {
	b := newGGUFBuilder().
		addString("general.architecture", "qwen3moe").
		addUint32("qwen3moe.expert_count", 60).
		addUint32("qwen3moe.expert_used_per_token", 4)
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	meta, err := ParseGGUFMetadata(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if meta.ExpertCount != 60 {
		t.Errorf("ExpertCount 期望 60，实际 %d", meta.ExpertCount)
	}
	if meta.ExpertUsed != 4 {
		t.Errorf("ExpertUsed 期望 4，实际 %d", meta.ExpertUsed)
	}
}

// TestParseGGUFMetadata_KVHeadInfo 测试 KV head 信息解析
func TestParseGGUFMetadata_KVHeadInfo(t *testing.T) {
	b := newGGUFBuilder().
		addString("general.architecture", "qwen3").
		addUint32("qwen3.head_count_kv", 8).
		addUint32("qwen3.key_length", 128)
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	meta, err := ParseGGUFMetadata(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if meta.KVHeadCount != 8 {
		t.Errorf("KVHeadCount 期望 8，实际 %d", meta.KVHeadCount)
	}
	if meta.HeadDimKV != 128 {
		t.Errorf("HeadDimKV 期望 128，实际 %d", meta.HeadDimKV)
	}
}

// TestParseGGUFMetadata_NParams 测试参数数量解析
func TestParseGGUFMetadata_NParams(t *testing.T) {
	b := newGGUFBuilder().
		addString("general.architecture", "qwen3").
		addFloat32("general.parameter_count", 1700000000) // 1.7B
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	meta, err := ParseGGUFMetadata(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if meta.NParams != 1700000000 {
		t.Errorf("NParams 期望 1700000000，实际 %d", meta.NParams)
	}
}

// TestParseGGUFMetadata_ChatTemplate 测试 chat template 解析
func TestParseGGUFMetadata_ChatTemplate(t *testing.T) {
	b := newGGUFBuilder().
		addString("general.architecture", "qwen3").
		addString("tokenizer.chat_template", "test-chat-template").
		addString("tokenizer.chat_template_tool_use", "test-tool-template")
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	meta, err := ParseGGUFMetadata(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if meta.ChatTemplate != "test-chat-template" {
		t.Errorf("ChatTemplate 期望 test-chat-template，实际 %s", meta.ChatTemplate)
	}
	if meta.ChatTemplateToolUse != "test-tool-template" {
		t.Errorf("ChatTemplateToolUse 期望 test-tool-template，实际 %s", meta.ChatTemplateToolUse)
	}
}

// TestToInt 测试 toInt 辅助函数对所有数值类型的处理
func TestToInt(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   int
		wantOk bool
	}{
		{"uint8", uint8(42), 42, true},
		{"int8", int8(-42), -42, true},
		{"uint16", uint16(1000), 1000, true},
		{"int16", int16(-1000), -1000, true},
		{"uint32", uint32(100000), 100000, true},
		{"int32", int32(-100000), -100000, true},
		{"uint64", uint64(100000), 100000, true},
		{"int64", int64(-100000), -100000, true},
		// 不支持的类型
		{"float32 不支持", float32(3.14), 0, false},
		{"float64 不支持", float64(3.14), 0, false},
		{"string 不支持", "123", 0, false},
		{"bool 不支持", true, 0, false},
		{"nil 不支持", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toInt(tt.input)
			if ok != tt.wantOk {
				t.Errorf("toInt(%T) ok 期望 %v，实际 %v", tt.input, tt.wantOk, ok)
			}
			if got != tt.want {
				t.Errorf("toInt(%T) 期望 %d，实际 %d", tt.input, tt.want, got)
			}
		})
	}
}

// TestToInt64 测试 toInt64 辅助函数对所有数值类型的处理
func TestToInt64(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   int64
		wantOk bool
	}{
		{"uint8", uint8(42), 42, true},
		{"int8", int8(-42), -42, true},
		{"uint16", uint16(1000), 1000, true},
		{"int16", int16(-1000), -1000, true},
		{"uint32", uint32(100000), 100000, true},
		{"int32", int32(-100000), -100000, true},
		{"uint64", uint64(100000), 100000, true},
		{"int64", int64(-100000), -100000, true},
		// toInt64 额外支持 float32 和 float64
		{"float32", float32(3.14), 3, true},
		{"float64", float64(99.9), 99, true},
		// 不支持的类型
		{"string 不支持", "123", 0, false},
		{"bool 不支持", true, 0, false},
		{"nil 不支持", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toInt64(tt.input)
			if ok != tt.wantOk {
				t.Errorf("toInt64(%T) ok 期望 %v，实际 %v", tt.input, tt.wantOk, ok)
			}
			if got != tt.want {
				t.Errorf("toInt64(%T) 期望 %d，实际 %d", tt.input, tt.want, got)
			}
		})
	}
}

// TestParseGGUFKV_InvalidVersion 测试不支持的 GGUF 版本
func TestParseGGUFKV_InvalidVersion(t *testing.T) {
	// 创建一个 magic 正确但版本不支持的文件
	f, err := os.CreateTemp("", "bad-version-*.gguf")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	// 写入正确的 magic
	if err := binary.Write(f, binary.LittleEndian, uint32(ggufMagic)); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	// 写入不支持的版本号（1）
	if err := binary.Write(f, binary.LittleEndian, uint32(1)); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	_, err = ParseGGUFKV(f.Name())
	if err == nil {
		t.Error("不支持的版本应返回错误，但返回了 nil")
	}
}

// TestParseGGUFMetadata_HFRepo 测试 HuggingFace 仓库地址解析。
//
// GGUF 1.x 标准中通过 general.source.huggingface.repository 字段记录源仓库
// （见 llama.cpp src/llama-arch.cpp 中的 LLM_KV_GENERAL_SOURCE_HF_REPO），
// 豆芽用它构造 hf-mirror.com 下载链接，提示用户下载 sidecar 模型。
//
// 生活类比：像商品包装上的「原厂地址」标签——拿到商品（GGUF 文件）后，
// 通过这个标签可以找到原厂（HF 仓库）的其他配件（如 eagle3-/dflash- 草稿模型）。
func TestParseGGUFMetadata_HFRepo(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		hfRepo   string // general.source.huggingface.repository 的值；空串表示不写入该字段
		wantRepo string
	}{
		{
			name:     "有 HF repo 字段",
			arch:     "qwen3",
			hfRepo:   "unsloth/Qwen3.5-7B-Instruct-GGUF",
			wantRepo: "unsloth/Qwen3.5-7B-Instruct-GGUF",
		},
		{
			name:     "无 HF repo 字段（本地量化模型常见情况）",
			arch:     "qwen3",
			hfRepo:   "",
			wantRepo: "",
		},
		{
			name:     "DFlash 模型的 HF repo",
			arch:     "qwen3.6",
			hfRepo:   "Qwen/Qwen3.6-UD-GGUF",
			wantRepo: "Qwen/Qwen3.6-UD-GGUF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newGGUFBuilder().addString("general.architecture", tt.arch)
			if tt.hfRepo != "" {
				b.addString("general.source.huggingface.repository", tt.hfRepo)
			}
			path := buildTempGGUFFileWithSuffix(t, b, "model.gguf")
			defer os.Remove(path)

			meta, err := ParseGGUFMetadata(path)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if meta.HFRepo != tt.wantRepo {
				t.Errorf("HFRepo 期望 %q，实际 %q", tt.wantRepo, meta.HFRepo)
			}
		})
	}
}

// TestParseGGUFMetadata_FileSize 测试文件大小记录
func TestParseGGUFMetadata_FileSize(t *testing.T) {
	b := newGGUFBuilder().addString("general.architecture", "qwen3")
	path := buildTempGGUFFile(t, b)
	defer os.Remove(path)

	// 获取文件实际大小
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("获取文件信息失败: %v", err)
	}
	expectedSize := fi.Size()

	meta, err := ParseGGUFMetadata(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if meta.FileSize != expectedSize {
		t.Errorf("FileSize 期望 %d，实际 %d", expectedSize, meta.FileSize)
	}
}

// TestFileTypeName_KnownTypes 验证已知 file_type 枚举值映射为正确的量化类型名
//
// 生活类比：就像国际电话区号表，+86 是中国、+1 是美国，
// file_type 枚举值到量化类型名也是一张固定的对照表。
func TestFileTypeName_KnownTypes(t *testing.T) {
	cases := []struct {
		ftype int
		want  string
	}{
		{0, "F32"},
		{1, "F16"},
		{2, "Q4_0"},
		{3, "Q4_1"},
		{7, "Q8_0"},
		{8, "Q5_0"},
		{9, "Q5_1"},
		{10, "Q2_K"},
		{14, "Q4_K - Small"},
		{15, "Q4_K - Medium"},
		{18, "Q6_K"},
		{25, "IQ4_NL"},
		{30, "IQ4_XS"},
	}
	for _, c := range cases {
		got := fileTypeName(c.ftype)
		if got != c.want {
			t.Errorf("fileTypeName(%d) = %q, 期望 %q", c.ftype, got, c.want)
		}
	}
}

// TestFileTypeName_UnknownType 验证未知枚举值返回空字符串
func TestFileTypeName_UnknownType(t *testing.T) {
	cases := []int{-1, 32, 50, 100}
	for _, ftype := range cases {
		got := fileTypeName(ftype)
		if got != "" {
			t.Errorf("fileTypeName(%d) 未知类型应返回 ''，实际: %q", ftype, got)
		}
	}
}
