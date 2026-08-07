package main

import (
	"fmt"
)

// ChunkStatus はチャンクの読み込み結果ステータスを表す enum です
type ChunkStatus int

const (
	ChunkStatusGenerated    ChunkStatus = iota // 正常に生成済み・読み込み成功
	ChunkStatusNotGenerated                    // 未生成チャンク
	ChunkStatusError                           // 読み込み/解析エラー
)

func (s ChunkStatus) String() string {
	switch s {
	case ChunkStatusGenerated:
		return "Generated"
	case ChunkStatusNotGenerated:
		return "NotGenerated"
	case ChunkStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

type ChunkData struct {
	X, Z             int
	CompressionType  byte
	UncompressedData []byte
	IsExternal       bool        // .mcc 外部ファイルから読み込まれたかのフラグ
	Status           ChunkStatus // チャンクの生成/読み込みステータス
}

type Block struct {
	Name       string
	Properties map[string]string
}

// SectionData は1セクション(16x16x16)分のパレットと元データ配列を保持します
type SectionData struct {
	Y            int8
	Palette      []Block
	Data         []int64 // 圧縮ビット配列 raw データ
	BitsPerBlock int     // エントリあたりのビット数
}

// GetIndexAt は指定されたブロックインデックス(0..4095)のパレット番号をオンデマンドで直接計算します
func (s *SectionData) GetIndexAt(i int) uint32 {
	if s.BitsPerBlock <= 0 || len(s.Data) == 0 {
		return 0
	}
	entriesPerLong := 64 / s.BitsPerBlock
	longIndex := i / entriesPerLong
	if longIndex >= len(s.Data) {
		return 0
	}
	subIndex := i % entriesPerLong
	shift := subIndex * s.BitsPerBlock
	mask := uint64((1 << s.BitsPerBlock) - 1)
	return uint32((uint64(s.Data[longIndex]) >> shift) & mask)
}

// ChunkBlocksDynamic は任意の高さ・範囲に対応できるチャンク構造
type ChunkBlocksDynamic struct {
	MinY     int                   // 実際の最小ブロックY座標 (例: -64, 0 など)
	MaxY     int                   // 実際の最大ブロックY座標 (例: 319, 255 など)
	Sections map[int8]*SectionData // SectionY -> セクションメタデータ
	Biomes   map[int8][4][4][4]string
}

// GetBlock は呼び出された時に必要箇所のみ遅延評価・動的計算でブロックを取得します
func (c *ChunkBlocksDynamic) GetBlock(x, y, z int) Block {
	if x < 0 || x >= 16 || z < 0 || z >= 16 || y < c.MinY || y > c.MaxY {
		return Block{Name: "minecraft:void"}
	}

	sectionY := int8(y >> 4)
	sec, ok := c.Sections[sectionY]
	if !ok || len(sec.Palette) == 0 {
		return Block{Name: "minecraft:air"}
	}

	// 1. 単一パレットの場合は計算なしで即座に返す
	if len(sec.Palette) == 1 {
		return sec.Palette[0]
	}

	// 2. 複数パレットの場合、 GetIndexAt で必要な箇所のみオンデマンド解凍
	localY := y & 15
	localX := x & 15
	localZ := z & 15
	blockIdx := localY*256 + localZ*16 + localX

	paletteIdx := sec.GetIndexAt(blockIdx)
	if int(paletteIdx) < len(sec.Palette) {
		return sec.Palette[paletteIdx]
	}

	return Block{Name: "minecraft:air"}
}

func (c *ChunkBlocksDynamic) GetBiome(x, y, z int) string {
	sectionY := int8(y >> 4)
	bx := (x & 15) >> 2
	by := ((y & 15) + 16) % 16 >> 2
	bz := (z & 15) >> 2

	if sec, ok := c.Biomes[sectionY]; ok {
		return sec[bx][by][bz]
	}
	return "minecraft:plains"
}

func ParseChunkBlocksDynamic(uncompressedNBT []byte) (*ChunkBlocksDynamic, error) {
	rawData, err := parseNBT(uncompressedNBT)
	if err != nil {
		return nil, err
	}

	sections, ok := rawData["sections"].([]interface{})
	if !ok || len(sections) == 0 {
		return nil, fmt.Errorf("sectionsタグが見つかりません")
	}

	// 1. 存在するセクションの最小Yと最大Yを特定する
	minSectionY := 127
	maxSectionY := -128

	parsedSections := make(map[int8]*SectionData)
	biomesMapResult := make(map[int8][4][4][4]string)

	for _, s := range sections {
		section, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		var sectionY int8
		switch yVal := section["Y"].(type) {
		case int8:
			sectionY = yVal
		case int32:
			sectionY = int8(yVal)
		case int64:
			sectionY = int8(yVal)
		default:
			continue
		}

		if int(sectionY) < minSectionY {
			minSectionY = int(sectionY)
		}
		if int(sectionY) > maxSectionY {
			maxSectionY = int(sectionY)
		}

		// --- バイオーム情報の解析 ---
		if biomesMap, ok := section["biomes"].(map[string]interface{}); ok {
			if paletteRaw, ok := biomesMap["palette"].([]interface{}); ok && len(paletteRaw) > 0 {
				defaultBiome := "minecraft:plains"
				if bName, ok := paletteRaw[0].(string); ok {
					defaultBiome = bName
				}

				var cell [4][4][4]string
				for bx := 0; bx < 4; bx++ {
					for by := 0; by < 4; by++ {
						for bz := 0; bz < 4; bz++ {
							cell[bx][by][bz] = defaultBiome
						}
					}
				}

				if dataRaw, ok := biomesMap["data"].([]int64); ok && len(paletteRaw) > 1 {
					bits := max(1, bitLen(uint(len(paletteRaw)-1)))
					indices := unpackBitArray(dataRaw, bits, 64)
					for i, idx := range indices {
						if int(idx) < len(paletteRaw) {
							bx := i % 4
							bz := (i / 4) % 4
							by := i / 16
							if str, ok := paletteRaw[idx].(string); ok {
								cell[bx][by][bz] = str
							}
						}
					}
				}
				biomesMapResult[sectionY] = cell
			}
		}

		// --- ブロックパレットの解析 ---
		blockStates, ok := section["block_states"].(map[string]interface{})
		if !ok {
			continue
		}

		paletteRaw, ok := blockStates["palette"].([]interface{})
		if !ok || len(paletteRaw) == 0 {
			continue
		}

		// (パレットの復元処理...)
		palette := make([]Block, len(paletteRaw))
		for i, p := range paletteRaw {
			pMap, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			rawName, _ := pMap["Name"].(string)
			// "namespace:blockID" 形式に統一（namespace 省略時は minecraft: を付与）
			cleanName := normalizeBlockID(rawName)

			props := make(map[string]string)
			if pProps, ok := pMap["Properties"].(map[string]interface{}); ok {
				for k, v := range pProps {
					if strV, ok := v.(string); ok {
						props[k] = strV
					}
				}
			}
			palette[i] = Block{Name: cleanName, Properties: props}
		}

		secData := &SectionData{
			Y:       sectionY,
			Palette: palette,
		}

		// 単一パレットの場合はデータ参照を持たせない
		if len(palette) > 1 {
			if dataRaw, ok := blockStates["data"].([]int64); ok {
				secData.Data = dataRaw
				secData.BitsPerBlock = max(4, bitLen(uint(len(palette)-1)))
			}
		}

		parsedSections[sectionY] = secData
	}

	if len(parsedSections) == 0 {
		return nil, fmt.Errorf("有効なセクションが存在しません")
	}

	minY := minSectionY * 16
	maxY := (maxSectionY+1)*16 - 1

	return &ChunkBlocksDynamic{
		MinY:     minY,
		MaxY:     maxY,
		Sections: parsedSections,
		Biomes:   biomesMapResult,
	}, nil
}
