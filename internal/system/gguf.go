// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

const ggufMagic = 0x46475547

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
	Architecture    string
	BlockCount      int
	EmbeddingLength int
	ContextLength   int
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
			}
		}
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
