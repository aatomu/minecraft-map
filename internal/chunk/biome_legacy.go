package chunk

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
