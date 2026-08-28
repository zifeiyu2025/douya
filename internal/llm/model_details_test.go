// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// ---- 最小 GGUF 二进制编码器（仅覆盖本测试需要的类型）----
// 与 internal/system 的 GGUF 解析格式对齐：magic + version + tensor_count + kv_count + kvs。

const testGGUFMagic uint32 = 0x46554747 // "GGUF"

type testKV struct {
	key       string
	valueType uint32
	value     any
}

func writeTestGGUF(t *testing.T, dir, name string, kvs []testKV) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建临时 GGUF 失败: %v", err)
	}
	defer f.Close()

	w := binary.LittleEndian
	_ = binary.Write(f, w, testGGUFMagic)
	_ = binary.Write(f, w, uint32(3))        // version
	_ = binary.Write(f, w, uint64(0))        // tensor count
	_ = binary.Write(f, w, uint64(len(kvs))) // kv count
	for _, kv := range kvs {
		keyBytes := []byte(kv.key)
		_ = binary.Write(f, w, uint64(len(keyBytes)))
		_, _ = f.Write(keyBytes)
		_ = binary.Write(f, w, kv.valueType)
		switch kv.valueType {
		case 8: // STRING
			s := []byte(kv.value.(string))
			_ = binary.Write(f, w, uint64(len(s)))
			_, _ = f.Write(s)
		case 4: // UINT32
			_ = binary.Write(f, w, kv.value.(uint32))
		case 10: // UINT64
			_ = binary.Write(f, w, kv.value.(uint64))
		}
	}
	return path
}

func buildTestModelKVs() []testKV {
	return []testKV{
		{key: "general.architecture", valueType: 8, value: "llama"},
		{key: "general.size_label", valueType: 8, value: "4B"},
		{key: "general.parameter_count", valueType: 10, value: uint64(4000000000)},
		{key: "general.file_type", valueType: 4, value: uint32(15)}, // Q4_K - Medium
		{key: "llama.context_length", valueType: 4, value: uint32(131072)},
		{key: "llama.block_count", valueType: 4, value: uint32(32)},
	}
}

// TestBuildModelDetails_FromGGUF 验证从真实 GGUF 二进制解析出全部详情字段。
func TestBuildModelDetails_FromGGUF(t *testing.T) {
	path := writeTestGGUF(t, t.TempDir(), "test-model-Q4_K_M.gguf", buildTestModelKVs())

	details, err := BuildModelDetails(path, true, false)
	if err != nil {
		t.Fatalf("BuildModelDetails 返回错误: %v", err)
	}

	if details.SizeLabel != "4B" {
		t.Errorf("SizeLabel = %q, 期望 \"4B\"", details.SizeLabel)
	}
	if details.NParams != 4000000000 {
		t.Errorf("NParams = %d, 期望 4000000000", details.NParams)
	}
	if details.QuantType != "Q4_K - Medium" {
		t.Errorf("QuantType = %q, 期望 \"Q4_K - Medium\"", details.QuantType)
	}
	if details.ContextLength != 131072 {
		t.Errorf("ContextLength = %d, 期望 131072", details.ContextLength)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 临时文件失败: %v", err)
	}
	if details.FileSizeBytes != fi.Size() {
		t.Errorf("FileSizeBytes = %d, 期望与文件实际大小一致 (%d)", details.FileSizeBytes, fi.Size())
	}
	if !details.HasVision || details.HasAudio {
		t.Errorf("多模态能力应保留传入值: HasVision=%v HasAudio=%v, 期望 true/false", details.HasVision, details.HasAudio)
	}
}

// TestBuildModelDetails_FileMissing 验证 GGUF 缺失时错误上抛且多模态能力保留。
func TestBuildModelDetails_FileMissing(t *testing.T) {
	details, err := BuildModelDetails(filepath.Join(t.TempDir(), "no-such.gguf"), false, true)
	if err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
	if !details.HasAudio || details.HasVision {
		t.Errorf("错误场景下多模态能力仍应保留传入值: HasVision=%v HasAudio=%v, 期望 false/true", details.HasVision, details.HasAudio)
	}
}
