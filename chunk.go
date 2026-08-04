package main

import "fmt"

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

// ChunkBlocksDynamic は任意の高さ・範囲に対応できるチャンク構造
type ChunkBlocksDynamic struct {
	MinY   int             // 実際の最小ブロックY座標 (例: -64, 0 など)
	MaxY   int             // 実際の最大ブロックY座標 (例: 319, 255 など)
	Blocks [16][][16]Block // [X][Y][Z] の動的スライス (Yの長さは可変)
	Biomes map[int8][4][4][4]string
}

func (c *ChunkBlocksDynamic) GetBlock(x, y, z int) Block {
	yIndex := y - c.MinY
	if x < 0 || x >= 16 || z < 0 || z >= 16 || yIndex < 0 || yIndex >= len(c.Blocks[0]) {
		return Block{Name: "minecraft:void"}
	}
	return c.Blocks[x][yIndex][z]
}

func (c *ChunkBlocksDynamic) SetBlock(x, y, z int, block Block) {
	yIndex := y - c.MinY
	if x >= 0 && x < 16 && z >= 0 && z < 16 && yIndex >= 0 && yIndex < len(c.Blocks[0]) {
		c.Blocks[x][yIndex][z] = block
	}
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
	validSections := make(map[int8]map[string]interface{})

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

		// 有効なセクション情報を保持
		validSections[sectionY] = section

		if int(sectionY) < minSectionY {
			minSectionY = int(sectionY)
		}
		if int(sectionY) > maxSectionY {
			maxSectionY = int(sectionY)
		}
	}

	if len(validSections) == 0 {
		return nil, fmt.Errorf("有効なセクションが存在しません")
	}

	// 2. 最小・最大のワールドY座標と高さを算出
	minY := minSectionY * 16
	maxY := (maxSectionY+1)*16 - 1
	totalHeight := maxY - minY + 1

	// 3. 動的に 3次元スライス [16][totalHeight][16] を確保
	chunk := &ChunkBlocksDynamic{
		MinY:   minY,
		MaxY:   maxY,
		Biomes: make(map[int8][4][4][4]string),
	}

	for x := 0; x < 16; x++ {
		chunk.Blocks[x] = make([][16]Block, totalHeight)
	}

	// 4. セクションごとにブロックを配置
	for sectionY, section := range validSections {
		// バイオーム情報の復元
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
				chunk.Biomes[sectionY] = cell
			}
		}

		// ブロック情報の復元
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
			name, _ := pMap["Name"].(string)

			props := make(map[string]string)
			if pProps, ok := pMap["Properties"].(map[string]interface{}); ok {
				for k, v := range pProps {
					if strV, ok := v.(string); ok {
						props[k] = strV
					}
				}
			}
			palette[i] = Block{Name: name, Properties: props}
		}

		// セクション内すべてのブロックを展開
		if len(palette) == 1 {
			// 単一ブロックで満たされている場合
			for ly := 0; ly < 16; ly++ {
				y := int(sectionY)*16 + ly
				for lx := 0; lx < 16; lx++ {
					for lz := 0; lz < 16; lz++ {
						// 修正: lz を追加 (lx, y, lz, block)
						chunk.SetBlock(lx, y, lz, palette[0])
					}
				}
			}
			continue
		}

		dataRaw, ok := blockStates["data"].([]int64)
		if !ok {
			continue
		}

		bitsPerBlock := max(4, bitLen(uint(len(palette)-1)))
		indices := unpackBitArray(dataRaw, bitsPerBlock, 4096)

		for i, paletteIndex := range indices {
			if int(paletteIndex) >= len(palette) {
				continue
			}

			localX := i % 16
			localZ := (i / 16) % 16
			localY := i / (16 * 16)

			worldY := int(sectionY)*16 + localY
			chunk.SetBlock(localX, worldY, localZ, palette[paletteIndex])
		}
	}

	return chunk, nil
}
