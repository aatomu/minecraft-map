package main

import "fmt"

// legacyBlockFromIDMeta は 1.12.2以下 で使われていた数値ブロックID(0-255、Add拡張で~4095まで)と
// メタデータ(0-15)を、"namespace:blockID" 形式の Block へ変換します。
//
// 見た目(色)に影響しないメタデータ(facing/lit等)は無視し、
// ウール・ステンドグラス・コンクリート・木材の樹種など見た目が大きく変わるものだけ
// メタデータで分岐しています。未知のID/メタデータの組み合わせは
// "legacy:unknown_id_<id>_meta_<meta>" という名前を返し、既存の
// "未登録ブロック" 警告の仕組みにそのまま乗せて可視化します。
func legacyBlockFromIDMeta(id int, meta int) Block {
	if resolver, ok := legacyBlockTable[id]; ok {
		return resolver(meta)
	}
	return Block{Name: fmt.Sprintf("legacy:unknown_id_%d_meta_%d", id, meta)}
}

type legacyResolver func(meta int) Block

// legacyDyeColors は 1.12.2 時点の染料メタデータ(0-15)の並び順です。
// ウール/ステンドグラス/ステンドグラス板/硬化粘土(染色テラコッタ)/カーペット/
// シュルカーボックス/コンクリート/コンクリートパウダーで共通です。
var legacyDyeColors = [16]string{
	"white", "orange", "magenta", "light_blue",
	"yellow", "lime", "pink", "gray",
	"light_gray", "cyan", "purple", "blue",
	"brown", "green", "red", "black",
}

func legacyDyeColorName(meta int) string {
	if meta < 0 || meta >= len(legacyDyeColors) {
		meta = 0
	}
	return legacyDyeColors[meta]
}

// legacySimple は meta を無視して常に同じブロック名を返すリゾルバを生成します
func legacySimple(name string) legacyResolver {
	return func(_ int) Block {
		return Block{Name: normalizeBlockID(name)}
	}
}

// legacyByMeta は meta の値に応じて names で分岐するリゾルバを生成します (範囲外は names[0])
func legacyByMeta(names ...string) legacyResolver {
	return func(meta int) Block {
		if meta < 0 || meta >= len(names) {
			meta = 0
		}
		return Block{Name: normalizeBlockID(names[meta])}
	}
}

// legacyByMetaMasked は meta に mask をかけてから names で分岐するリゾルバを生成します。
// 色に無関係な上位ビット(例: ハーフブロックの "smooth" フラグ)を無視したい場合に使用します。
func legacyByMetaMasked(mask int, names ...string) legacyResolver {
	return func(meta int) Block {
		m := meta & mask
		if m < 0 || m >= len(names) {
			m = 0
		}
		return Block{Name: normalizeBlockID(names[m])}
	}
}

// legacyByMetaMod は meta % len(names) で分岐するリゾルバを生成します (log/leaves の樹種ビット用)
func legacyByMetaMod(names ...string) legacyResolver {
	return func(meta int) Block {
		return Block{Name: normalizeBlockID(names[meta%len(names)])}
	}
}

// legacyDyed は "<color>_<suffix>" 形式で染料メタデータに応じたブロック名を返すリゾルバを生成します
func legacyDyed(suffix string) legacyResolver {
	return func(meta int) Block {
		return Block{Name: normalizeBlockID(legacyDyeColorName(meta) + "_" + suffix)}
	}
}

// legacyGlazedTerracottaByID は id (235-250) からそのまま色付きグレイズドテラコッタを返すリゾルバです
func legacyGlazedTerracottaByID(colorIdx int) legacyResolver {
	return legacyColoredByID(colorIdx, "glazed_terracotta")
}

// legacyColoredByID は id (シュルカーボックス 219-234 等) からそのまま色付きブロックを返すリゾルバです。
// ウールやコンクリートと違い、シュルカーボックスは meta ではなく id 自体で色が決まるため区別しています。
func legacyColoredByID(colorIdx int, suffix string) legacyResolver {
	return func(_ int) Block {
		return Block{Name: normalizeBlockID(legacyDyeColorName(colorIdx) + "_" + suffix)}
	}
}

// legacyAgeStem は melon_stem / pumpkin_stem 用。meta(0-7)を "age" プロパティとして保持し、
// texture.go の tintResolvers による成長段階の色分岐に利用します。
func legacyAgeStem(name string) legacyResolver {
	return func(meta int) Block {
		return Block{
			Name:       normalizeBlockID(name),
			Properties: map[string]string{"age": fmt.Sprintf("%d", meta)},
		}
	}
}

var legacyBlockTable = map[int]legacyResolver{
	0:   legacySimple("air"),
	1:   legacyByMeta("stone", "granite", "polished_granite", "diorite", "polished_diorite", "andesite", "polished_andesite"),
	2:   legacySimple("grass_block"),
	3:   legacyByMeta("dirt", "coarse_dirt", "podzol"),
	4:   legacySimple("cobblestone"),
	5:   legacyByMeta("oak_planks", "spruce_planks", "birch_planks", "jungle_planks", "acacia_planks", "dark_oak_planks"),
	6:   legacyByMeta("oak_sapling", "spruce_sapling", "birch_sapling", "jungle_sapling", "acacia_sapling", "dark_oak_sapling"),
	7:   legacySimple("bedrock"),
	8:   legacySimple("water"),
	9:   legacySimple("water"),
	10:  legacySimple("lava"),
	11:  legacySimple("lava"),
	12:  legacyByMeta("sand", "red_sand"),
	13:  legacySimple("gravel"),
	14:  legacySimple("gold_ore"),
	15:  legacySimple("iron_ore"),
	16:  legacySimple("coal_ore"),
	17:  legacyByMetaMod("oak_log", "spruce_log", "birch_log", "jungle_log"),
	18:  legacyByMetaMod("oak_leaves", "spruce_leaves", "birch_leaves", "jungle_leaves"),
	19:  legacyByMeta("sponge", "wet_sponge"),
	20:  legacySimple("glass"),
	21:  legacySimple("lapis_ore"),
	22:  legacySimple("lapis_block"),
	23:  legacySimple("dispenser"),
	24:  legacyByMeta("sandstone", "chiseled_sandstone", "smooth_sandstone"),
	25:  legacySimple("note_block"),
	26:  legacySimple("red_bed"),
	27:  legacySimple("golden_rail"),
	28:  legacySimple("detector_rail"),
	29:  legacySimple("sticky_piston"),
	30:  legacySimple("cobweb"),
	31:  legacyByMeta("dead_bush", "grass", "fern"),
	32:  legacySimple("dead_bush"),
	33:  legacySimple("piston"),
	34:  legacySimple("piston_head"),
	35:  legacyDyed("wool"),
	36:  legacySimple("air"), // piston_extension (見えない技術ブロック)
	37:  legacySimple("dandelion"),
	38:  legacyByMeta("poppy", "blue_orchid", "allium", "azure_bluet", "red_tulip", "orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy"),
	39:  legacySimple("brown_mushroom"),
	40:  legacySimple("red_mushroom"),
	41:  legacySimple("gold_block"),
	42:  legacySimple("iron_block"),
	43:  legacyByMetaMasked(0x07, "smooth_stone", "sandstone", "oak_planks", "cobblestone", "bricks", "stone_bricks", "nether_bricks", "quartz_block"),
	44:  legacyByMetaMasked(0x07, "stone_slab", "sandstone", "oak_planks", "cobblestone", "bricks", "stone_bricks", "nether_bricks", "quartz_block"),
	45:  legacySimple("bricks"),
	46:  legacySimple("tnt"),
	47:  legacySimple("bookshelf"),
	48:  legacySimple("mossy_cobblestone"),
	49:  legacySimple("obsidian"),
	50:  legacySimple("torch"),
	51:  legacySimple("fire"),
	52:  legacySimple("spawner"),
	53:  legacySimple("oak_stairs"),
	54:  legacySimple("chest"),
	55:  legacySimple("redstone_wire"),
	56:  legacySimple("diamond_ore"),
	57:  legacySimple("diamond_block"),
	58:  legacySimple("crafting_table"),
	59:  legacySimple("wheat"),
	60:  legacySimple("farmland"),
	61:  legacySimple("furnace"),
	62:  legacySimple("furnace"),
	63:  legacySimple("oak_sign"),
	64:  legacySimple("oak_door"),
	65:  legacySimple("ladder"),
	66:  legacySimple("rail"),
	67:  legacySimple("cobblestone_stairs"),
	68:  legacySimple("oak_wall_sign"),
	69:  legacySimple("lever"),
	70:  legacySimple("stone_pressure_plate"),
	71:  legacySimple("iron_door"),
	72:  legacySimple("oak_pressure_plate"),
	73:  legacySimple("redstone_ore"),
	74:  legacySimple("redstone_ore"),
	75:  legacySimple("redstone_torch"),
	76:  legacySimple("redstone_torch"),
	77:  legacySimple("stone_button"),
	78:  legacySimple("snow"),
	79:  legacySimple("ice"),
	80:  legacySimple("snow_block"),
	81:  legacySimple("cactus"),
	82:  legacySimple("clay"),
	83:  legacySimple("sugar_cane"),
	84:  legacySimple("jukebox"),
	85:  legacySimple("oak_fence"),
	86:  legacySimple("pumpkin"),
	87:  legacySimple("netherrack"),
	88:  legacySimple("soul_sand"),
	89:  legacySimple("glowstone"),
	90:  legacySimple("nether_portal"),
	91:  legacySimple("jack_o_lantern"),
	92:  legacySimple("cake"),
	93:  legacySimple("repeater"),
	94:  legacySimple("repeater"),
	95:  legacyDyed("stained_glass"),
	96:  legacySimple("oak_trapdoor"),
	97:  legacyByMeta("infested_stone", "infested_cobblestone", "infested_stone_bricks", "infested_mossy_stone_bricks", "infested_cracked_stone_bricks", "infested_chiseled_stone_bricks"),
	98:  legacyByMeta("stone_bricks", "mossy_stone_bricks", "cracked_stone_bricks", "chiseled_stone_bricks"),
	99:  legacySimple("brown_mushroom_block"),
	100: legacySimple("red_mushroom_block"),
	101: legacySimple("iron_bars"),
	102: legacySimple("glass_pane"),
	103: legacySimple("melon_block"),
	104: legacyAgeStem("pumpkin_stem"),
	105: legacyAgeStem("melon_stem"),
	106: legacySimple("vine"),
	107: legacySimple("oak_fence_gate"),
	108: legacySimple("brick_stairs"),
	109: legacySimple("stone_brick_stairs"),
	110: legacySimple("mycelium"),
	111: legacySimple("lily_pad"),
	112: legacySimple("nether_bricks"),
	113: legacySimple("nether_brick_fence"),
	114: legacySimple("nether_brick_stairs"),
	115: legacySimple("nether_wart"),
	116: legacySimple("enchanting_table"),
	117: legacySimple("brewing_stand"),
	118: legacySimple("cauldron"),
	119: legacySimple("end_portal"),
	120: legacySimple("end_portal_frame"),
	121: legacySimple("end_stone"),
	122: legacySimple("dragon_egg"),
	123: legacySimple("redstone_lamp"),
	124: legacySimple("redstone_lamp"),
	125: legacyByMeta("oak_planks", "spruce_planks", "birch_planks", "jungle_planks", "acacia_planks", "dark_oak_planks"),
	126: legacyByMeta("oak_planks", "spruce_planks", "birch_planks", "jungle_planks", "acacia_planks", "dark_oak_planks"),
	127: legacySimple("cocoa"),
	128: legacySimple("sandstone_stairs"),
	129: legacySimple("emerald_ore"),
	130: legacySimple("ender_chest"),
	131: legacySimple("tripwire_hook"),
	132: legacySimple("tripwire"),
	133: legacySimple("emerald_block"),
	134: legacySimple("spruce_stairs"),
	135: legacySimple("birch_stairs"),
	136: legacySimple("jungle_stairs"),
	137: legacySimple("command_block"),
	138: legacySimple("beacon"),
	139: legacyByMeta("cobblestone_wall", "mossy_cobblestone_wall"),
	140: legacySimple("flower_pot"),
	141: legacySimple("carrots"),
	142: legacySimple("potatoes"),
	143: legacySimple("oak_button"),
	144: legacySimple("skeleton_skull"),
	145: legacySimple("anvil"),
	146: legacySimple("trapped_chest"),
	147: legacySimple("light_weighted_pressure_plate"),
	148: legacySimple("heavy_weighted_pressure_plate"),
	149: legacySimple("comparator"),
	150: legacySimple("comparator"),
	151: legacySimple("daylight_detector"),
	152: legacySimple("redstone_block"),
	153: legacySimple("nether_quartz_ore"),
	154: legacySimple("hopper"),
	155: legacyByMeta("quartz_block", "chiseled_quartz_block", "quartz_pillar", "quartz_pillar", "quartz_pillar"),
	156: legacySimple("quartz_stairs"),
	157: legacySimple("activator_rail"),
	158: legacySimple("dropper"),
	159: legacyDyed("terracotta"),
	160: legacyDyed("stained_glass_pane"),
	161: legacyByMetaMod("acacia_leaves", "dark_oak_leaves"),
	162: legacyByMetaMod("acacia_log", "dark_oak_log"),
	163: legacySimple("acacia_stairs"),
	164: legacySimple("dark_oak_stairs"),
	165: legacySimple("slime_block"),
	166: legacySimple("barrier"),
	167: legacySimple("iron_trapdoor"),
	168: legacyByMeta("prismarine", "prismarine_bricks", "dark_prismarine"),
	169: legacySimple("sea_lantern"),
	170: legacySimple("hay_block"),
	171: legacyDyed("carpet"),
	172: legacySimple("terracotta"),
	173: legacySimple("coal_block"),
	174: legacySimple("packed_ice"),
	175: legacyByMeta("sunflower", "lilac", "tall_grass", "large_fern", "rose_bush", "peony"),
	176: legacySimple("white_banner"),
	177: legacySimple("white_wall_banner"),
	178: legacySimple("daylight_detector"),
	179: legacyByMeta("red_sandstone", "chiseled_red_sandstone", "smooth_red_sandstone"),
	180: legacySimple("red_sandstone_stairs"),
	181: legacySimple("red_sandstone"),
	182: legacySimple("red_sandstone"),
	183: legacySimple("spruce_fence_gate"),
	184: legacySimple("birch_fence_gate"),
	185: legacySimple("jungle_fence_gate"),
	186: legacySimple("dark_oak_fence_gate"),
	187: legacySimple("acacia_fence_gate"),
	188: legacySimple("spruce_fence"),
	189: legacySimple("birch_fence"),
	190: legacySimple("jungle_fence"),
	191: legacySimple("dark_oak_fence"),
	192: legacySimple("acacia_fence"),
	193: legacySimple("spruce_door"),
	194: legacySimple("birch_door"),
	195: legacySimple("jungle_door"),
	196: legacySimple("acacia_door"),
	197: legacySimple("dark_oak_door"),
	198: legacySimple("end_rod"),
	199: legacySimple("chorus_plant"),
	200: legacySimple("chorus_flower"),
	201: legacySimple("purpur_block"),
	202: legacySimple("purpur_pillar"),
	203: legacySimple("purpur_stairs"),
	204: legacySimple("purpur_block"),
	205: legacySimple("purpur_block"),
	206: legacySimple("end_stone_bricks"),
	207: legacySimple("beetroots"),
	208: legacySimple("dirt_path"),
	209: legacySimple("end_gateway"),
	210: legacySimple("command_block"),
	211: legacySimple("command_block"),
	212: legacySimple("frosted_ice"),
	213: legacySimple("magma_block"),
	214: legacySimple("nether_wart_block"),
	215: legacySimple("red_nether_bricks"),
	216: legacySimple("bone_block"),
	217: legacySimple("structure_void"),
	218: legacySimple("observer"),
	219: legacyColoredByID(0, "shulker_box"),
	220: legacyColoredByID(1, "shulker_box"),
	221: legacyColoredByID(2, "shulker_box"),
	222: legacyColoredByID(3, "shulker_box"),
	223: legacyColoredByID(4, "shulker_box"),
	224: legacyColoredByID(5, "shulker_box"),
	225: legacyColoredByID(6, "shulker_box"),
	226: legacyColoredByID(7, "shulker_box"),
	227: legacyColoredByID(8, "shulker_box"),
	228: legacyColoredByID(9, "shulker_box"),
	229: legacyColoredByID(10, "shulker_box"),
	230: legacyColoredByID(11, "shulker_box"),
	231: legacyColoredByID(12, "shulker_box"),
	232: legacyColoredByID(13, "shulker_box"),
	233: legacyColoredByID(14, "shulker_box"),
	234: legacyColoredByID(15, "shulker_box"),
	235: legacyGlazedTerracottaByID(0),
	236: legacyGlazedTerracottaByID(1),
	237: legacyGlazedTerracottaByID(2),
	238: legacyGlazedTerracottaByID(3),
	239: legacyGlazedTerracottaByID(4),
	240: legacyGlazedTerracottaByID(5),
	241: legacyGlazedTerracottaByID(6),
	242: legacyGlazedTerracottaByID(7),
	243: legacyGlazedTerracottaByID(8),
	244: legacyGlazedTerracottaByID(9),
	245: legacyGlazedTerracottaByID(10),
	246: legacyGlazedTerracottaByID(11),
	247: legacyGlazedTerracottaByID(12),
	248: legacyGlazedTerracottaByID(13),
	249: legacyGlazedTerracottaByID(14),
	250: legacyGlazedTerracottaByID(15),
	251: legacyDyed("concrete"),
	252: legacyDyed("concrete_powder"),
	255: legacySimple("structure_block"),
}
