package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	TagEnd       byte = 0
	TagByte      byte = 1
	TagShort     byte = 2
	TagInt       byte = 3
	TagLong      byte = 4
	TagFloat     byte = 5
	TagDouble    byte = 6
	TagByteArray byte = 7
	TagString    byte = 8
	TagList      byte = 9
	TagCompound  byte = 10
	TagIntArray  byte = 11
	TagLongArray byte = 12
)

type NBTParser struct {
	r *bytes.Reader
}

func parseNBT(data []byte) (map[string]interface{}, error) {
	parser := &NBTParser{r: bytes.NewReader(data)}
	tagType, err := parser.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if tagType != TagCompound {
		return nil, fmt.Errorf("ルートタグがCompoundではありません: %d", tagType)
	}

	// ルートタグの名前を読み飛ばす
	if _, err := parser.readString(); err != nil {
		return nil, err
	}

	// ルートCompoundの中身をパース
	val, err := parser.readCompound()
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (p *NBTParser) readTagValue(tagType byte) (interface{}, error) {
	switch tagType {
	case TagEnd:
		return nil, nil
	case TagByte:
		b, err := p.r.ReadByte()
		return int8(b), err
	case TagShort:
		var v int16
		err := binary.Read(p.r, binary.BigEndian, &v)
		return v, err
	case TagInt:
		var v int32
		err := binary.Read(p.r, binary.BigEndian, &v)
		return v, err
	case TagLong:
		var v int64
		err := binary.Read(p.r, binary.BigEndian, &v)
		return v, err
	case TagFloat:
		var v float32
		err := binary.Read(p.r, binary.BigEndian, &v)
		return v, err
	case TagDouble:
		var v float64
		err := binary.Read(p.r, binary.BigEndian, &v)
		return v, err
	case TagByteArray:
		var length int32
		if err := binary.Read(p.r, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("不正な配列長です(ByteArray): %d", length)
		}
		buf := make([]byte, length)
		_, err := io.ReadFull(p.r, buf)
		return buf, err
	case TagString:
		return p.readString()
	case TagList:
		elemType, err := p.r.ReadByte()
		if err != nil {
			return nil, err
		}
		var length int32
		if err := binary.Read(p.r, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("不正な配列長です(List): %d", length)
		}
		list := make([]interface{}, length)
		for i := 0; i < int(length); i++ {
			elem, err := p.readTagValue(elemType)
			if err != nil {
				return nil, err
			}
			list[i] = elem
		}
		return list, nil
	case TagCompound:
		return p.readCompound()
	case TagIntArray:
		var length int32
		if err := binary.Read(p.r, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("不正な配列長です(IntArray): %d", length)
		}
		arr := make([]int32, length)
		err := binary.Read(p.r, binary.BigEndian, &arr)
		return arr, err
	case TagLongArray:
		var length int32
		if err := binary.Read(p.r, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("不正な配列長です(LongArray): %d", length)
		}
		arr := make([]int64, length)
		err := binary.Read(p.r, binary.BigEndian, &arr)
		return arr, err
	default:
		return nil, fmt.Errorf("未知のタグタイプ: %d", tagType)
	}
}

func (p *NBTParser) readString() (string, error) {
	var length uint16
	if err := binary.Read(p.r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (p *NBTParser) readCompound() (map[string]interface{}, error) {
	res := make(map[string]interface{})
	for {
		tagType, err := p.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if tagType == TagEnd {
			break
		}
		name, err := p.readString()
		if err != nil {
			return nil, err
		}
		val, err := p.readTagValue(tagType)
		if err != nil {
			return nil, err
		}
		res[name] = val
	}
	return res, nil
}

// Minecraft 1.16+ パディング対応のビット配列アンパック関数
func unpackBitArray(data []int64, bitsPerBlock, count int) []uint32 {
	result := make([]uint32, count)
	entriesPerLong := 64 / bitsPerBlock
	mask := uint64((1 << bitsPerBlock) - 1)

	for i := 0; i < count; i++ {
		longIndex := i / entriesPerLong
		subIndex := i % entriesPerLong

		if longIndex >= len(data) {
			break
		}

		shift := subIndex * bitsPerBlock
		value := (uint64(data[longIndex]) >> shift) & mask
		result[i] = uint32(value)
	}
	return result
}

func bitLen(n uint) int {
	count := 0
	for n > 0 {
		n >>= 1
		count++
	}
	return count
}
