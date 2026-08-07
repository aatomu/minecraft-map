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

// dataVersionNoCrossingBoundary は、ブロック配列のビットパック形式が変更された
// スナップショット 20w17a (1.16) の DataVersion です。
// これ以上であれば「エントリがlong境界をまたがない」新形式、未満であれば
// 「エントリがlong境界をまたぐことがある」旧形式(1.13〜1.15)として扱います。
const dataVersionNoCrossingBoundary = 2529

// SectionData は1セクション(16x16x16)分のパレットと元データ配列を保持します
type SectionData struct {
	Y            int8
	Palette      []Block
	Data         []int64 // 圧縮ビット配列 raw データ
	BitsPerBlock int     // エントリあたりのビット数
	LegacyPack   bool    // true の場合、エントリがlong境界をまたぐ旧(〜1.15)形式として読む
}

// GetIndexAt は指定されたブロックインデックス(0..4095)のパレット番号をオンデマンドで直接計算します
func (s *SectionData) GetIndexAt(i int) uint32 {
	if s.BitsPerBlock <= 0 || len(s.Data) == 0 {
		return 0
	}
	mask := uint64((1 << s.BitsPerBlock) - 1)

	if s.LegacyPack {
		// 1.13〜1.15: エントリがlong境界をまたぐ場合がある旧パッキング形式
		startBit := i * s.BitsPerBlock
		startLong := startBit / 64
		startOffset := startBit % 64
		endLong := ((i+1)*s.BitsPerBlock - 1) / 64

		if startLong >= len(s.Data) {
			return 0
		}

		if startLong == endLong {
			return uint32((uint64(s.Data[startLong]) >> startOffset) & mask)
		}

		if endLong >= len(s.Data) {
			return 0
		}

		lowBits := uint64(s.Data[startLong]) >> startOffset
		highBits := uint64(s.Data[endLong]) << (64 - startOffset)
		return uint32((lowBits | highBits) & mask)
	}

	// 1.16以降(1.18のsections形式含む): エントリがlong境界をまたがない形式
	entriesPerLong := 64 / s.BitsPerBlock
	longIndex := i / entriesPerLong
	if longIndex >= len(s.Data) {
		return 0
	}
	subIndex := i % entriesPerLong
	shift := subIndex * s.BitsPerBlock
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

// ParseChunkBlocksDynamic はチャンクのNBTを解析します。
// ルート直下に "sections" があれば 1.18+ 形式、"Level" タグでラップされていれば
// 1.13〜1.17 形式として自動判定します。
func ParseChunkBlocksDynamic(uncompressedNBT []byte) (*ChunkBlocksDynamic, error) {
	rawData, err := parseNBT(uncompressedNBT)
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

	return nil, fmt.Errorf("sections/Level tag not found (possibly an unsupported version)")
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

		secData, ok := parseBlockStateSection(sectionY, blockStates["palette"], blockStates["data"], false)
		if !ok {
			continue
		}

		parsedSections[sectionY] = secData
	}

	if len(parsedSections) == 0 {
		return nil, fmt.Errorf("no valid sections exist")
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
		return nil, fmt.Errorf("Sections tag not found")
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

		secData, ok := parseBlockStateSection(sectionY, section["Palette"], section["BlockStates"], legacyPack)
		if !ok {
			continue
		}

		parsedSections[sectionY] = secData
	}

	if len(parsedSections) == 0 {
		return nil, fmt.Errorf("no valid sections exist")
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

// parseBlockStateSection は Palette/BlockStates(またはblock_states.palette/data) 相当のタグから
// SectionData を構築します。共通ロジックを 1.18+ / 1.13〜1.17 の両方から利用します。
func parseBlockStateSection(sectionY int8, paletteRawIface, dataRawIface interface{}, legacyPack bool) (*SectionData, bool) {
	paletteRaw, ok := paletteRawIface.([]interface{})
	if !ok || len(paletteRaw) == 0 {
		return nil, false
	}

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
		Y:          sectionY,
		Palette:    palette,
		LegacyPack: legacyPack,
	}

	// 単一パレットの場合はデータ参照を持たせない
	if len(palette) > 1 {
		if dataRaw, ok := dataRawIface.([]int64); ok {
			secData.Data = dataRaw
			secData.BitsPerBlock = max(4, bitLen(uint(len(palette)-1)))
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

// parseLegacyBiomes は 1.13〜1.17 の "Biomes" タグを解析し、共通の
// map[int8][4][4][4]string 形式へ変換します。
//
//   - 1.15〜1.17: int[1024] (4x4 水平 × 64 垂直 の3Dバイオーム, y=0〜255をカバー)
//   - 1.13〜1.14: byte[256] (16x16 の2Dバイオーム, Y方向の変化なし)
func parseLegacyBiomes(biomesRaw interface{}, minSectionY, maxSectionY int) map[int8][4][4][4]string {
	result := make(map[int8][4][4][4]string)

	switch data := biomesRaw.(type) {
	case []int32:
		// 3D biomes: index = y4*16 + z4*4 + x4  (y4: 0..63, 4ブロック単位)
		for idx, biomeID := range data {
			if idx >= 1024 {
				break
			}
			y4 := idx / 16
			rem := idx % 16
			z4 := rem / 4
			x4 := rem % 4

			sectionY := int8(y4 / 4)
			by := y4 % 4

			name := legacyBiomeName(int(biomeID))
			cell := result[sectionY]
			cell[x4][by][z4] = name
			result[sectionY] = cell
		}

	case []byte:
		// 2D biomes: index = z*16 + x (Y方向の変化なし)
		if len(data) < 256 {
			return result
		}
		for sy := minSectionY; sy <= maxSectionY; sy++ {
			var cell [4][4][4]string
			for x4 := 0; x4 < 4; x4++ {
				for z4 := 0; z4 < 4; z4++ {
					x := x4 * 4
					z := z4 * 4
					biomeID := data[z*16+x]
					name := legacyBiomeName(int(int8(biomeID)))
					for by := 0; by < 4; by++ {
						cell[x4][by][z4] = name
					}
				}
			}
			result[int8(sy)] = cell
		}
	}

	return result
}

// legacyBiomeNames は 1.13〜1.17 で使われていた数値バイオームIDから
// 名前空間付きバイオームIDへの対応表です。
var legacyBiomeNames = map[int]string{
	0:   "minecraft:ocean",
	1:   "minecraft:plains",
	2:   "minecraft:desert",
	3:   "minecraft:mountains",
	4:   "minecraft:forest",
	5:   "minecraft:taiga",
	6:   "minecraft:swamp",
	7:   "minecraft:river",
	8:   "minecraft:nether_wastes",
	9:   "minecraft:the_end",
	10:  "minecraft:frozen_ocean",
	11:  "minecraft:frozen_river",
	12:  "minecraft:snowy_tundra",
	13:  "minecraft:snowy_mountains",
	14:  "minecraft:mushroom_fields",
	15:  "minecraft:mushroom_field_shore",
	16:  "minecraft:beach",
	17:  "minecraft:desert_hills",
	18:  "minecraft:wooded_hills",
	19:  "minecraft:taiga_hills",
	20:  "minecraft:mountain_edge",
	21:  "minecraft:jungle",
	22:  "minecraft:jungle_hills",
	23:  "minecraft:jungle_edge",
	24:  "minecraft:deep_ocean",
	25:  "minecraft:stone_shore",
	26:  "minecraft:snowy_beach",
	27:  "minecraft:birch_forest",
	28:  "minecraft:birch_forest_hills",
	29:  "minecraft:dark_forest",
	30:  "minecraft:snowy_taiga",
	31:  "minecraft:snowy_taiga_hills",
	32:  "minecraft:giant_tree_taiga",
	33:  "minecraft:giant_tree_taiga_hills",
	34:  "minecraft:wooded_mountains",
	35:  "minecraft:savanna",
	36:  "minecraft:savanna_plateau",
	37:  "minecraft:badlands",
	38:  "minecraft:wooded_badlands_plateau",
	39:  "minecraft:badlands_plateau",
	40:  "minecraft:small_end_islands",
	41:  "minecraft:end_midlands",
	42:  "minecraft:end_highlands",
	43:  "minecraft:end_barrens",
	44:  "minecraft:warm_ocean",
	45:  "minecraft:lukewarm_ocean",
	46:  "minecraft:cold_ocean",
	47:  "minecraft:deep_warm_ocean",
	48:  "minecraft:deep_lukewarm_ocean",
	49:  "minecraft:deep_cold_ocean",
	50:  "minecraft:deep_frozen_ocean",
	127: "minecraft:the_void",
	129: "minecraft:sunflower_plains",
	130: "minecraft:desert_lakes",
	131: "minecraft:gravelly_mountains",
	132: "minecraft:flower_forest",
	133: "minecraft:taiga_mountains",
	134: "minecraft:swamp_hills",
	140: "minecraft:ice_spikes",
	149: "minecraft:modified_jungle",
	151: "minecraft:modified_jungle_edge",
	155: "minecraft:tall_birch_forest",
	156: "minecraft:tall_birch_hills",
	157: "minecraft:dark_forest_hills",
	158: "minecraft:snowy_taiga_mountains",
	160: "minecraft:giant_spruce_taiga",
	161: "minecraft:giant_spruce_taiga_hills",
	162: "minecraft:modified_gravelly_mountains",
	163: "minecraft:shattered_savanna",
	164: "minecraft:shattered_savanna_plateau",
	165: "minecraft:eroded_badlands",
	166: "minecraft:modified_wooded_badlands_plateau",
	167: "minecraft:modified_badlands_plateau",
	168: "minecraft:bamboo_jungle",
	169: "minecraft:bamboo_jungle_hills",
	170: "minecraft:soul_sand_valley",
	171: "minecraft:crimson_forest",
	172: "minecraft:warped_forest",
	173: "minecraft:basalt_deltas",
}

func legacyBiomeName(id int) string {
	if name, ok := legacyBiomeNames[id]; ok {
		return name
	}
	return "minecraft:plains"
}
