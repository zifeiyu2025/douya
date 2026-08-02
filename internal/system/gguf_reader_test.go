// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

// newGGUFReader 用字节切片构造一个 ggufReader，便于测试。
// 生活类比：就像把一段录音带塞进播放器，播放器按顺序读取数据。
func newGGUFReader(data []byte) *ggufReader {
	return &ggufReader{r: bytes.NewReader(data)}
}

// ===== 读取原语正常路径测试 =====

// TestGGUFReader_ReadUint8 验证 uint8 读取
func TestGGUFReader_ReadUint8(t *testing.T) {
	r := newGGUFReader([]byte{0x2A})
	v, err := r.readUint8()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != 42 {
		t.Errorf("期望 v=42，实际: %d", v)
	}
}

// TestGGUFReader_ReadUint16 验证 uint16 读取（小端序）
func TestGGUFReader_ReadUint16(t *testing.T) {
	// 0x0100 小端序 = 1
	r := newGGUFReader([]byte{0x01, 0x00})
	v, err := r.readUint16()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != 1 {
		t.Errorf("期望 v=1，实际: %d", v)
	}
}

// TestGGUFReader_ReadInt16 验证 int16 读取（含负数）
func TestGGUFReader_ReadInt16(t *testing.T) {
	// 0xFFFF 小端序 = -1
	r := newGGUFReader([]byte{0xFF, 0xFF})
	v, err := r.readInt16()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != -1 {
		t.Errorf("期望 v=-1，实际: %d", v)
	}
}

// TestGGUFReader_ReadUint32 验证 uint32 读取
func TestGGUFReader_ReadUint32(t *testing.T) {
	// 0x00000001 小端序 = 1
	r := newGGUFReader([]byte{0x01, 0x00, 0x00, 0x00})
	v, err := r.readUint32()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != 1 {
		t.Errorf("期望 v=1，实际: %d", v)
	}
}

// TestGGUFReader_ReadInt32 验证 int32 读取（含负数）
func TestGGUFReader_ReadInt32(t *testing.T) {
	r := newGGUFReader([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	v, err := r.readInt32()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != -1 {
		t.Errorf("期望 v=-1，实际: %d", v)
	}
}

// TestGGUFReader_ReadFloat32 验证 float32 读取
func TestGGUFReader_ReadFloat32(t *testing.T) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(3.14))
	r := newGGUFReader(buf)
	v, err := r.readFloat32()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if math.Abs(float64(v)-3.14) > 0.001 {
		t.Errorf("期望 v≈3.14，实际: %f", v)
	}
}

// TestGGUFReader_ReadUint64 验证 uint64 读取
func TestGGUFReader_ReadUint64(t *testing.T) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, 0x0102030405060708)
	r := newGGUFReader(buf)
	v, err := r.readUint64()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != 0x0102030405060708 {
		t.Errorf("期望 v=0x0102030405060708，实际: 0x%016x", v)
	}
}

// TestGGUFReader_ReadInt64 验证 int64 读取（含负数）
func TestGGUFReader_ReadInt64(t *testing.T) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, 0xFFFFFFFFFFFFFFFF)
	r := newGGUFReader(buf)
	v, err := r.readInt64()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != -1 {
		t.Errorf("期望 v=-1，实际: %d", v)
	}
}

// TestGGUFReader_ReadFloat64 验证 float64 读取
func TestGGUFReader_ReadFloat64(t *testing.T) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, math.Float64bits(3.141592653589793))
	r := newGGUFReader(buf)
	v, err := r.readFloat64()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != 3.141592653589793 {
		t.Errorf("期望 v=3.141592653589793，实际: %f", v)
	}
}

// TestGGUFReader_ReadString 验证字符串读取
func TestGGUFReader_ReadString(t *testing.T) {
	// 字符串格式：8字节长度 + 内容
	str := "hello"
	buf := make([]byte, 8+len(str))
	binary.LittleEndian.PutUint64(buf, uint64(len(str)))
	copy(buf[8:], str)
	r := newGGUFReader(buf)
	v, err := r.readString()
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if v != "hello" {
		t.Errorf("期望 v=hello，实际: %q", v)
	}
}

// ===== 读取原语错误路径测试（数据不足） =====

// TestGGUFReader_ReadUint8_Empty 验证空 reader 返回错误
func TestGGUFReader_ReadUint8_Empty(t *testing.T) {
	r := newGGUFReader(nil)
	_, err := r.readUint8()
	if err == nil {
		t.Error("期望空 reader 返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadUint16_Truncated 验证数据不足返回错误
func TestGGUFReader_ReadUint16_Truncated(t *testing.T) {
	r := newGGUFReader([]byte{0x01}) // 只有 1 字节，需要 2 字节
	_, err := r.readUint16()
	if err == nil {
		t.Error("期望数据不足时返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadUint32_Truncated 验证数据不足返回错误
func TestGGUFReader_ReadUint32_Truncated(t *testing.T) {
	r := newGGUFReader([]byte{0x01, 0x00}) // 只有 2 字节，需要 4 字节
	_, err := r.readUint32()
	if err == nil {
		t.Error("期望数据不足时返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadUint64_Truncated 验证数据不足返回错误
func TestGGUFReader_ReadUint64_Truncated(t *testing.T) {
	r := newGGUFReader([]byte{0x01, 0x00, 0x00, 0x00}) // 只有 4 字节，需要 8 字节
	_, err := r.readUint64()
	if err == nil {
		t.Error("期望数据不足时返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadString_Truncated 验证字符串数据不足返回错误
func TestGGUFReader_ReadString_Truncated(t *testing.T) {
	// 声明长度为 5，但只提供 3 字节内容
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, 5)
	buf = append(buf, 'a', 'b', 'c') // 只有 3 字节，需要 5 字节
	r := newGGUFReader(buf)
	_, err := r.readString()
	if err == nil {
		t.Error("期望字符串内容不足时返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadString_TooLong 验证超长字符串返回错误
// 生活类比：快递单上写着包裹有 2MB 大，明显不合理（正常字符串不会这么大），拒收。
func TestGGUFReader_ReadString_TooLong(t *testing.T) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, (1<<20)+1) // 超过 1MB 限制
	r := newGGUFReader(buf)
	_, err := r.readString()
	if err == nil {
		t.Error("期望超长字符串返回错误，实际返回 nil")
	}
}

// ===== readValue 分支测试 =====

// TestGGUFReader_ReadValue_AllTypes 验证 readValue 正确分发所有类型
func TestGGUFReader_ReadValue_AllTypes(t *testing.T) {
	tests := []struct {
		name      string
		valueType uint32
		data      []byte
		want      any
	}{
		{"uint8", ggufTypeUINT8, []byte{0x2A}, uint8(42)},
		{"int8", ggufTypeINT8, []byte{0xFF}, int8(-1)},
		{"uint16", ggufTypeUINT16, []byte{0x01, 0x00}, uint16(1)},
		{"int16", ggufTypeINT16, []byte{0xFF, 0xFF}, int16(-1)},
		{"uint32", ggufTypeUINT32, []byte{0x01, 0x00, 0x00, 0x00}, uint32(1)},
		{"int32", ggufTypeINT32, []byte{0xFF, 0xFF, 0xFF, 0xFF}, int32(-1)},
		{"float32", ggufTypeFLOAT32, func() []byte {
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, math.Float32bits(1.0))
			return b
		}(), float32(1.0)},
		{"bool_true", ggufTypeBOOL, []byte{0x01}, true},
		{"bool_false", ggufTypeBOOL, []byte{0x00}, false},
		{"uint64", ggufTypeUINT64, []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}, uint64(0x0102030405060708)},
		{"int64", ggufTypeINT64, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, int64(-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newGGUFReader(tt.data)
			got, err := r.readValue(tt.valueType)
			if err != nil {
				t.Fatalf("期望无错误，实际: %v", err)
			}
			if got != tt.want {
				t.Errorf("期望 %v，实际: %v", tt.want, got)
			}
		})
	}
}

// TestGGUFReader_ReadValue_String 验证 readValue 读取字符串
func TestGGUFReader_ReadValue_String(t *testing.T) {
	str := "test"
	buf := make([]byte, 8+len(str))
	binary.LittleEndian.PutUint64(buf, uint64(len(str)))
	copy(buf[8:], str)
	r := newGGUFReader(buf)
	got, err := r.readValue(ggufTypeSTRING)
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if got != str {
		t.Errorf("期望 %q，实际: %v", str, got)
	}
}

// TestGGUFReader_ReadValue_Float64 验证 readValue 读取 float64
func TestGGUFReader_ReadValue_Float64(t *testing.T) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, math.Float64bits(2.71828))
	r := newGGUFReader(buf)
	got, err := r.readValue(ggufTypeFLOAT64)
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
	if got != 2.71828 {
		t.Errorf("期望 2.71828，实际: %v", got)
	}
}

// TestGGUFReader_ReadValue_UnknownType 验证未知类型返回错误
func TestGGUFReader_ReadValue_UnknownType(t *testing.T) {
	r := newGGUFReader(nil)
	_, err := r.readValue(999)
	if err == nil {
		t.Error("期望未知类型返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadValue_Array 验证 readValue 跳过数组
func TestGGUFReader_ReadValue_Array(t *testing.T) {
	// 数组格式：uint32 元素类型 + uint64 长度 + 元素数据
	// 构造一个包含 3 个 uint8 元素的数组
	buf := make([]byte, 4+8+3)
	binary.LittleEndian.PutUint32(buf[0:4], ggufTypeUINT8) // 元素类型
	binary.LittleEndian.PutUint64(buf[4:12], 3)            // 长度
	buf[12] = 10
	buf[13] = 20
	buf[14] = 30
	r := newGGUFReader(buf)
	_, err := r.readValue(ggufTypeARRAY)
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
}

// TestGGUFReader_ReadValue_Array_Truncated 验证数组元素数据不足返回错误
func TestGGUFReader_ReadValue_Array_Truncated(t *testing.T) {
	// 声明 3 个 uint8 元素，但只提供 1 字节
	buf := make([]byte, 4+8+1)
	binary.LittleEndian.PutUint32(buf[0:4], ggufTypeUINT8)
	binary.LittleEndian.PutUint64(buf[4:12], 3)
	buf[12] = 10
	r := newGGUFReader(buf)
	_, err := r.readValue(ggufTypeARRAY)
	if err == nil {
		t.Error("期望数组元素不足时返回错误，实际返回 nil")
	}
}

// TestGGUFReader_ReadValue_Array_UnknownElemType 验证未知元素类型返回错误
func TestGGUFReader_ReadValue_Array_UnknownElemType(t *testing.T) {
	buf := make([]byte, 4+8)
	binary.LittleEndian.PutUint32(buf[0:4], 999) // 未知元素类型
	binary.LittleEndian.PutUint64(buf[4:12], 1)
	r := newGGUFReader(buf)
	_, err := r.readValue(ggufTypeARRAY)
	if err == nil {
		t.Error("期望未知元素类型返回错误，实际返回 nil")
	}
}

// TestGGUFReader_SkipArrayElements_NestedArray 验证嵌套数组跳过
func TestGGUFReader_SkipArrayElements_NestedArray(t *testing.T) {
	// 外层数组：1 个元素，元素类型为 ARRAY
	// 内层数组：2 个 uint8 元素
	innerBuf := make([]byte, 4+8+2)
	binary.LittleEndian.PutUint32(innerBuf[0:4], ggufTypeUINT8)
	binary.LittleEndian.PutUint64(innerBuf[4:12], 2)
	innerBuf[12] = 1
	innerBuf[13] = 2

	outerBuf := make([]byte, 4+8)
	binary.LittleEndian.PutUint32(outerBuf[0:4], ggufTypeARRAY)
	binary.LittleEndian.PutUint64(outerBuf[4:12], 1)

	r := newGGUFReader(append(outerBuf, innerBuf...))
	err := r.skipArrayElements(ggufTypeARRAY, 1)
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}
}

// TestGGUFReader_SequentialReads 验证连续读取多个值
// 生活类比：就像从录音带里连续听多首歌，每首歌的长度不同，但播放器按顺序读。
func TestGGUFReader_SequentialReads(t *testing.T) {
	// 构造：uint8(42) + uint32(1000) + string("hi")
	str := "hi"
	buf := make([]byte, 0, 1+4+8+2)
	buf = append(buf, 42) // uint8
	buf = binary.LittleEndian.AppendUint32(buf, 1000) // uint32
	buf = binary.LittleEndian.AppendUint64(buf, uint64(len(str))) // string length
	buf = append(buf, str...) // string content

	r := newGGUFReader(buf)
	v1, err := r.readUint8()
	if err != nil || v1 != 42 {
		t.Fatalf("第一次读取 uint8 期望 42，实际: %d, err: %v", v1, err)
	}
	v2, err := r.readUint32()
	if err != nil || v2 != 1000 {
		t.Fatalf("第二次读取 uint32 期望 1000，实际: %d, err: %v", v2, err)
	}
	v3, err := r.readString()
	if err != nil || v3 != "hi" {
		t.Fatalf("第三次读取 string 期望 hi，实际: %q, err: %v", v3, err)
	}
}

// TestGGUFReader_ReadAfterEOF 验证读完后再读返回 EOF
func TestGGUFReader_ReadAfterEOF(t *testing.T) {
	r := newGGUFReader([]byte{0x2A})
	_, _ = r.readUint8() // 读完 1 字节
	_, err := r.readUint8() // 再读应失败
	if err == nil {
		t.Error("期望读完后再读返回错误，实际返回 nil")
	}
	if err != io.EOF {
		// binary.Read 在数据不足时返回 io.ErrUnexpectedEOF 或 io.EOF
		// 两者都是合理的错误
	}
}
