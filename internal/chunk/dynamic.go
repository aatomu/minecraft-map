package chunk

import (
	"fmt"

	"github.com/aatomu/minecraft-map/internal/block"
	"github.com/aatomu/minecraft-map/internal/nbt"
)

// ChunkBlocksDynamic は任意の高さ・範囲に対応できるチャンク構造
type ChunkBlocksDynamic struct {
	MinY     int                   // 実際の最小ブロックY座標 (例: -64, 0 など)
	MaxY     int                   // 実際の最大ブロックY座標 (例: 319, 255 など)
	Sections map[int8]*SectionData // SectionY -> セクションメタデータ
	Biomes   map[int8][4][4][4]string
}

// GetBlock は呼び出された時に必要箇所のみ遅延評価・動的計算でブロックを取得します
func (c *ChunkBlocksDynamic) GetBlock(x, y, z int) block.Block {
	if x < 0 || x >= 16 || z < 0 || z >= 16 || y < c.MinY || y > c.MaxY {
		return block.Block{Name: "minecraft:void"}
	}

	sectionY := int8(y >> 4)
	sec, ok := c.Sections[sectionY]
	if !ok {
		return block.Block{Name: "minecraft:air"}
	}

	localY := y & 15
	localX := x & 15
	localZ := z & 15
	blockIdx := localY*256 + localZ*16 + localX

	// 1.13未満: 数値ID+メタデータ形式
	if sec.IsNumeric {
		return sec.GetNumericBlock(blockIdx)
	}

	if len(sec.Palette) == 0 {
		return block.Block{Name: "minecraft:air"}
	}

	// 1. 単一パレットの場合は計算なしで即座に返す
	if len(sec.Palette) == 1 {
		return sec.Palette[0]
	}

	// 2. 複数パレットの場合、 GetIndexAt で必要な箇所のみオンデマンド解凍
	paletteIdx := sec.GetIndexAt(blockIdx)
	if int(paletteIdx) < len(sec.Palette) {
		return sec.Palette[paletteIdx]
	}

	return block.Block{Name: "minecraft:air"}
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

// ParseChunkBlocksDynamic はチャンクのNBTを解析します。
// ルート直下に "sections" があれば 1.18+ 形式、"Level" タグでラップされていれば
// 1.13〜1.17 形式(セクションに "Palette" あり)または 1.12.2以下 形式
// (セクションに数値IDの "Blocks"/"Data"/"Add" あり)として自動判定します。
func ParseChunkBlocksDynamic(uncompressedNBT []byte) (*ChunkBlocksDynamic, error) {
	rawData, err := nbt.Parse(uncompressedNBT)
	if err != nil {
		return nil, err
	}

	// 1.18+: ルート直下に sections が存在する
	if sections, ok := rawData["sections"].([]interface{}); ok && len(sections) > 0 {
		return parseChunkSectionsModern(sections)
	}

	// 1.13〜1.17: "Level" タグの中に Sections / Biomes が存在する
	if level, ok := rawData["Level"].(map[string]interface{}); ok {
		dataVersion := int32(0)
		if dv, ok := rawData["DataVersion"].(int32); ok {
			dataVersion = dv
		}
		return parseChunkLevelLegacy(level, dataVersion)
	}

	return nil, fmt.Errorf("sections/Level タグが見つかりません (対応バージョン外の可能性があります)")
}

// parseChunkSectionsModern は 1.18 以降のフラット化された sections 形式を解析します
func parseChunkSectionsModern(sections []interface{}) (*ChunkBlocksDynamic, error) {
	minSectionY := 127
	maxSectionY := -128

	parsedSections := make(map[int8]*SectionData)
	biomesMapResult := make(map[int8][4][4][4]string)

	for _, s := range sections {
		section, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		sectionY, ok := readSectionY(section["Y"])
		if !ok {
			continue
		}

		if int(sectionY) < minSectionY {
			minSectionY = int(sectionY)
		}
		if int(sectionY) > maxSectionY {
			maxSectionY = int(sectionY)
		}

		// --- バイオーム情報の解析 (1.18+: セクション単位の4x4x4パレット) ---
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
					bits := max(1, nbt.BitLen(uint(len(paletteRaw)-1)))
					indices := nbt.UnpackBitArray(dataRaw, bits, 64)
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

		secData, ok := parseBlockStateSection(sectionY, blockStates["palette"], blockStates["data"], false)
		if !ok {
			continue
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

// parseChunkLevelLegacy は 1.13〜1.17 の "Level" タグでラップされた形式を解析します
func parseChunkLevelLegacy(level map[string]interface{}, dataVersion int32) (*ChunkBlocksDynamic, error) {
	sectionsRaw, ok := level["Sections"].([]interface{})
	if !ok || len(sectionsRaw) == 0 {
		return nil, fmt.Errorf("Sectionsタグが見つかりません")
	}

	// 1.16 (20w17a, DataVersion 2529) 以降は long境界をまたがない新形式のビットパック、
	// それ未満(1.13〜1.15)はまたぐことがある旧形式
	legacyPack := dataVersion < dataVersionNoCrossingBoundary

	minSectionY := 127
	maxSectionY := -128

	parsedSections := make(map[int8]*SectionData)

	for _, s := range sectionsRaw {
		section, ok := s.(map[string]interface{})
		if !ok {
			continue
		}

		sectionY, ok := readSectionY(section["Y"])
		if !ok {
			continue
		}

		if int(sectionY) < minSectionY {
			minSectionY = int(sectionY)
		}
		if int(sectionY) > maxSectionY {
			maxSectionY = int(sectionY)
		}

		secData, ok := parseLegacySection(section, sectionY, legacyPack)
		if !ok {
			continue
		}

		parsedSections[sectionY] = secData
	}

	if len(parsedSections) == 0 {
		return nil, fmt.Errorf("有効なセクションが存在しません")
	}

	minY := minSectionY * 16
	maxY := (maxSectionY+1)*16 - 1

	biomesMapResult := parseLegacyBiomes(level["Biomes"], minSectionY, maxSectionY)

	return &ChunkBlocksDynamic{
		MinY:     minY,
		MaxY:     maxY,
		Sections: parsedSections,
		Biomes:   biomesMapResult,
	}, nil
}

// parseLegacySection は "Level.Sections" の1要素を解析します。
// "Palette" タグがあれば 1.13〜1.17 のフラット化済み形式、
// 無く "Blocks" タグがあれば 1.12.2以下 の数値ID+メタデータ形式として扱います。
func parseLegacySection(section map[string]interface{}, sectionY int8, legacyPack bool) (*SectionData, bool) {
	if _, hasPalette := section["Palette"]; hasPalette {
		return parseBlockStateSection(sectionY, section["Palette"], section["BlockStates"], legacyPack)
	}

	blocksRaw, ok := section["Blocks"].([]byte)
	if !ok || len(blocksRaw) == 0 {
		return nil, false
	}

	dataRaw, _ := section["Data"].([]byte)
	addRaw, _ := section["Add"].([]byte)

	return &SectionData{
		Y:             sectionY,
		IsNumeric:     true,
		NumericBlocks: blocksRaw,
		NumericData:   dataRaw,
		NumericAdd:    addRaw,
	}, true
}

// parseBlockStateSection は Palette/BlockStates(またはblock_states.palette/data) 相当のタグから
// SectionData を構築します。共通ロジックを 1.18+ / 1.13〜1.17 の両方から利用します。
func parseBlockStateSection(sectionY int8, paletteRawIface, dataRawIface interface{}, legacyPack bool) (*SectionData, bool) {
	paletteRaw, ok := paletteRawIface.([]interface{})
	if !ok || len(paletteRaw) == 0 {
		return nil, false
	}

	palette := make([]block.Block, len(paletteRaw))
	for i, p := range paletteRaw {
		pMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		rawName, _ := pMap["Name"].(string)
		// "namespace:blockID" 形式に統一（namespace 省略時は minecraft: を付与）
		cleanName := block.Normalize(rawName)

		props := make(map[string]string)
		if pProps, ok := pMap["Properties"].(map[string]interface{}); ok {
			for k, v := range pProps {
				if strV, ok := v.(string); ok {
					props[k] = strV
				}
			}
		}
		palette[i] = block.Block{Name: cleanName, Properties: props}
	}

	secData := &SectionData{
		Y:          sectionY,
		Palette:    palette,
		LegacyPack: legacyPack,
	}

	// 単一パレットの場合はデータ参照を持たせない
	if len(palette) > 1 {
		if dataRaw, ok := dataRawIface.([]int64); ok {
			secData.Data = dataRaw
			secData.BitsPerBlock = max(4, nbt.BitLen(uint(len(palette)-1)))
		}
	}

	return secData, true
}

// readSectionY はセクションの "Y" タグを int8 として読み取ります (型は NBT の Byte/Int/Long のいずれか)
func readSectionY(v interface{}) (int8, bool) {
	switch yVal := v.(type) {
	case int8:
		return yVal, true
	case int32:
		return int8(yVal), true
	case int64:
		return int8(yVal), true
	default:
		return 0, false
	}
}
