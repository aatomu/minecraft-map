// Package nbt は Minecraft の NBT (Named Binary Tag) 形式の汎用パーサです。
// チャンク/リージョン固有の知識は持たず、バイト列を map[string]interface{} へ変換するだけの役割です。
package nbt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type parser struct {
	r *bytes.Reader
}

// Parse は NBT バイナリをルート Compound タグとしてパースします。
// (旧 main パッケージの parseNBT を公開関数化)
func Parse(data []byte) (map[string]interface{}, error) {
	p := &parser{r: bytes.NewReader(data)}
	tagType, err := p.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if tagType != TagCompound {
		return nil, fmt.Errorf("ルートタグがCompoundではありません: %d", tagType)
	}

	// ルートタグの名前を読み飛ばす
	if _, err := p.readString(); err != nil {
		return nil, err
	}

	// ルートCompoundの中身をパース
	val, err := p.readCompound()
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (p *parser) readTagValue(tagType byte) (interface{}, error) {
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

func (p *parser) readString() (string, error) {
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

func (p *parser) readCompound() (map[string]interface{}, error) {
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
