package block

import (
	"fmt"
)

// LegacyBlockFromIDMeta は 1.12.2以下 で使われていた数値ブロックID(0-255、Add拡張で~4095まで)と
// メタデータ(0-15)を、"namespace:blockID" 形式の Block へ変換します。
//
// 見た目(色)に影響しないメタデータ(facing/lit等)は無視し、
// ウール・ステンドグラス・コンクリート・木材の樹種など見た目が大きく変わるものだけ
// メタデータで分岐しています。未知のID/メタデータの組み合わせは
// "legacy:unknown_id_<id>_meta_<meta>" という名前を返し、既存の
// "未登録ブロック" 警告の仕組みにそのまま乗せて可視化します。
func LegacyBlockFromIDMeta(id int, meta int) Block {
	if resolver, ok := LegacyBlockTable[id]; ok {
		return resolver(meta)
	}
	return Block{Name: fmt.Sprintf("legacy:unknown_id_%d_meta_%d", id, meta)}
}

type LegacyResolver func(meta int) Block

// LegacyDyeColors は 1.12.2 時点の染料メタデータ(0-15)の並び順です。
// ウール/ステンドグラス/ステンドグラス板/硬化粘土(染色テラコッタ)/カーペット/
// シュルカーボックス/コンクリート/コンクリートパウダーで共通です。
var LegacyDyeColors = [16]string{
	"white", "orange", "magenta", "light_blue",
	"yellow", "lime", "pink", "gray",
	"light_gray", "cyan", "purple", "blue",
	"brown", "green", "red", "black",
}

func LegacyDyeColorName(meta int) string {
	if meta < 0 || meta >= len(LegacyDyeColors) {
		meta = 0
	}
	return LegacyDyeColors[meta]
}

// LegacySimple は meta を無視して常に同じブロック名を返すリゾルバを生成します
func LegacySimple(name string) LegacyResolver {
	return func(_ int) Block {
		return Block{Name: Normalize(name)}
	}
}

// LegacyByMeta は meta の値に応じて names で分岐するリゾルバを生成します (範囲外は names[0])
func LegacyByMeta(names ...string) LegacyResolver {
	return func(meta int) Block {
		if meta < 0 || meta >= len(names) {
			meta = 0
		}
		return Block{Name: Normalize(names[meta])}
	}
}

// LegacyByMetaMasked は meta に mask をかけてから names で分岐するリゾルバを生成します。
// 色に無関係な上位ビット(例: ハーフブロックの "smooth" フラグ)を無視したい場合に使用します。
func LegacyByMetaMasked(mask int, names ...string) LegacyResolver {
	return func(meta int) Block {
		m := meta & mask
		if m < 0 || m >= len(names) {
			m = 0
		}
		return Block{Name: Normalize(names[m])}
	}
}

// LegacyByMetaMod は meta % len(names) で分岐するリゾルバを生成します (log/leaves の樹種ビット用)
func LegacyByMetaMod(names ...string) LegacyResolver {
	return func(meta int) Block {
		return Block{Name: Normalize(names[meta%len(names)])}
	}
}

// LegacyDyed は "<color>_<suffix>" 形式で染料メタデータに応じたブロック名を返すリゾルバを生成します
func LegacyDyed(suffix string) LegacyResolver {
	return func(meta int) Block {
		return Block{Name: Normalize(LegacyDyeColorName(meta) + "_" + suffix)}
	}
}

// LegacyGlazedTerracottaByID は id (235-250) からそのまま色付きグレイズドテラコッタを返すリゾルバです
func LegacyGlazedTerracottaByID(colorIdx int) LegacyResolver {
	return LegacyColoredByID(colorIdx, "glazed_terracotta")
}

// LegacyColoredByID は id (シュルカーボックス 219-234 等) からそのまま色付きブロックを返すリゾルバです。
// ウールやコンクリートと違い、シュルカーボックスは meta ではなく id 自体で色が決まるため区別しています。
func LegacyColoredByID(colorIdx int, suffix string) LegacyResolver {
	return func(_ int) Block {
		return Block{Name: Normalize(LegacyDyeColorName(colorIdx) + "_" + suffix)}
	}
}

// LegacyAgeStem は melon_stem / pumpkin_stem 用。meta(0-7)を "age" プロパティとして保持し、
// texture パッケージの tintResolvers による成長段階の色分岐に利用します。
func LegacyAgeStem(name string) LegacyResolver {
	return func(meta int) Block {
		return Block{
			Name:       Normalize(name),
			Properties: map[string]string{"age": fmt.Sprintf("%d", meta)},
		}
	}
}

var LegacyBlockTable = map[int]LegacyResolver{
	0:   LegacySimple("air"),
	1:   LegacyByMeta("stone", "granite", "polished_granite", "diorite", "polished_diorite", "andesite", "polished_andesite"),
	2:   LegacySimple("grass_block"),
	3:   LegacyByMeta("dirt", "coarse_dirt", "podzol"),
	4:   LegacySimple("cobblestone"),
	5:   LegacyByMeta("oak_planks", "spruce_planks", "birch_planks", "jungle_planks", "acacia_planks", "dark_oak_planks"),
	6:   LegacyByMeta("oak_sapling", "spruce_sapling", "birch_sapling", "jungle_sapling", "acacia_sapling", "dark_oak_sapling"),
	7:   LegacySimple("bedrock"),
	8:   LegacySimple("water"),
	9:   LegacySimple("water"),
	10:  LegacySimple("lava"),
	11:  LegacySimple("lava"),
	12:  LegacyByMeta("sand", "red_sand"),
	13:  LegacySimple("gravel"),
	14:  LegacySimple("gold_ore"),
	15:  LegacySimple("iron_ore"),
	16:  LegacySimple("coal_ore"),
	17:  LegacyByMetaMod("oak_log", "spruce_log", "birch_log", "jungle_log"),
	18:  LegacyByMetaMod("oak_leaves", "spruce_leaves", "birch_leaves", "jungle_leaves"),
	19:  LegacyByMeta("sponge", "wet_sponge"),
	20:  LegacySimple("glass"),
	21:  LegacySimple("lapis_ore"),
	22:  LegacySimple("lapis_block"),
	23:  LegacySimple("dispenser"),
	24:  LegacyByMeta("sandstone", "chiseled_sandstone", "smooth_sandstone"),
	25:  LegacySimple("note_block"),
	26:  LegacySimple("red_bed"),
	27:  LegacySimple("golden_rail"),
	28:  LegacySimple("detector_rail"),
	29:  LegacySimple("sticky_piston"),
	30:  LegacySimple("cobweb"),
	31:  LegacyByMeta("dead_bush", "grass", "fern"),
	32:  LegacySimple("dead_bush"),
	33:  LegacySimple("piston"),
	34:  LegacySimple("piston_head"),
	35:  LegacyDyed("wool"),
	36:  LegacySimple("air"), // piston_extension (見えない技術ブロック)
	37:  LegacySimple("dandelion"),
	38:  LegacyByMeta("poppy", "blue_orchid", "allium", "azure_bluet", "red_tulip", "orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy"),
	39:  LegacySimple("brown_mushroom"),
	40:  LegacySimple("red_mushroom"),
	41:  LegacySimple("gold_block"),
	42:  LegacySimple("iron_block"),
	43:  LegacyByMetaMasked(0x07, "smooth_stone", "sandstone", "oak_planks", "cobblestone", "bricks", "stone_bricks", "nether_bricks", "quartz_block"),
	44:  LegacyByMetaMasked(0x07, "stone_slab", "sandstone", "oak_planks", "cobblestone", "bricks", "stone_bricks", "nether_bricks", "quartz_block"),
	45:  LegacySimple("bricks"),
	46:  LegacySimple("tnt"),
	47:  LegacySimple("bookshelf"),
	48:  LegacySimple("mossy_cobblestone"),
	49:  LegacySimple("obsidian"),
	50:  LegacySimple("torch"),
	51:  LegacySimple("fire"),
	52:  LegacySimple("spawner"),
	53:  LegacySimple("oak_stairs"),
	54:  LegacySimple("chest"),
	55:  LegacySimple("redstone_wire"),
	56:  LegacySimple("diamond_ore"),
	57:  LegacySimple("diamond_block"),
	58:  LegacySimple("crafting_table"),
	59:  LegacySimple("wheat"),
	60:  LegacySimple("farmland"),
	61:  LegacySimple("furnace"),
	62:  LegacySimple("furnace"),
	63:  LegacySimple("oak_sign"),
	64:  LegacySimple("oak_door"),
	65:  LegacySimple("ladder"),
	66:  LegacySimple("rail"),
	67:  LegacySimple("cobblestone_stairs"),
	68:  LegacySimple("oak_wall_sign"),
	69:  LegacySimple("lever"),
	70:  LegacySimple("stone_pressure_plate"),
	71:  LegacySimple("iron_door"),
	72:  LegacySimple("oak_pressure_plate"),
	73:  LegacySimple("redstone_ore"),
	74:  LegacySimple("redstone_ore"),
	75:  LegacySimple("redstone_torch"),
	76:  LegacySimple("redstone_torch"),
	77:  LegacySimple("stone_button"),
	78:  LegacySimple("snow"),
	79:  LegacySimple("ice"),
	80:  LegacySimple("snow_block"),
	81:  LegacySimple("cactus"),
	82:  LegacySimple("clay"),
	83:  LegacySimple("sugar_cane"),
	84:  LegacySimple("jukebox"),
	85:  LegacySimple("oak_fence"),
	86:  LegacySimple("pumpkin"),
	87:  LegacySimple("netherrack"),
	88:  LegacySimple("soul_sand"),
	89:  LegacySimple("glowstone"),
	90:  LegacySimple("nether_portal"),
	91:  LegacySimple("jack_o_lantern"),
	92:  LegacySimple("cake"),
	93:  LegacySimple("repeater"),
	94:  LegacySimple("repeater"),
	95:  LegacyDyed("stained_glass"),
	96:  LegacySimple("oak_trapdoor"),
	97:  LegacyByMeta("infested_stone", "infested_cobblestone", "infested_stone_bricks", "infested_mossy_stone_bricks", "infested_cracked_stone_bricks", "infested_chiseled_stone_bricks"),
	98:  LegacyByMeta("stone_bricks", "mossy_stone_bricks", "cracked_stone_bricks", "chiseled_stone_bricks"),
	99:  LegacySimple("brown_mushroom_block"),
	100: LegacySimple("red_mushroom_block"),
	101: LegacySimple("iron_bars"),
	102: LegacySimple("glass_pane"),
	103: LegacySimple("melon_block"),
	104: LegacyAgeStem("pumpkin_stem"),
	105: LegacyAgeStem("melon_stem"),
	106: LegacySimple("vine"),
	107: LegacySimple("oak_fence_gate"),
	108: LegacySimple("brick_stairs"),
	109: LegacySimple("stone_brick_stairs"),
	110: LegacySimple("mycelium"),
	111: LegacySimple("lily_pad"),
	112: LegacySimple("nether_bricks"),
	113: LegacySimple("nether_brick_fence"),
	114: LegacySimple("nether_brick_stairs"),
	115: LegacySimple("nether_wart"),
	116: LegacySimple("enchanting_table"),
	117: LegacySimple("brewing_stand"),
	118: LegacySimple("cauldron"),
	119: LegacySimple("end_portal"),
	120: LegacySimple("end_portal_frame"),
	121: LegacySimple("end_stone"),
	122: LegacySimple("dragon_egg"),
	123: LegacySimple("redstone_lamp"),
	124: LegacySimple("redstone_lamp"),
	125: LegacyByMeta("oak_planks", "spruce_planks", "birch_planks", "jungle_planks", "acacia_planks", "dark_oak_planks"),
	126: LegacyByMeta("oak_planks", "spruce_planks", "birch_planks", "jungle_planks", "acacia_planks", "dark_oak_planks"),
	127: LegacySimple("cocoa"),
	128: LegacySimple("sandstone_stairs"),
	129: LegacySimple("emerald_ore"),
	130: LegacySimple("ender_chest"),
	131: LegacySimple("tripwire_hook"),
	132: LegacySimple("tripwire"),
	133: LegacySimple("emerald_block"),
	134: LegacySimple("spruce_stairs"),
	135: LegacySimple("birch_stairs"),
	136: LegacySimple("jungle_stairs"),
	137: LegacySimple("command_block"),
	138: LegacySimple("beacon"),
	139: LegacyByMeta("cobblestone_wall", "mossy_cobblestone_wall"),
	140: LegacySimple("flower_pot"),
	141: LegacySimple("carrots"),
	142: LegacySimple("potatoes"),
	143: LegacySimple("oak_button"),
	144: LegacySimple("skeleton_skull"),
	145: LegacySimple("anvil"),
	146: LegacySimple("trapped_chest"),
	147: LegacySimple("light_weighted_pressure_plate"),
	148: LegacySimple("heavy_weighted_pressure_plate"),
	149: LegacySimple("comparator"),
	150: LegacySimple("comparator"),
	151: LegacySimple("daylight_detector"),
	152: LegacySimple("redstone_block"),
	153: LegacySimple("nether_quartz_ore"),
	154: LegacySimple("hopper"),
	155: LegacyByMeta("quartz_block", "chiseled_quartz_block", "quartz_pillar", "quartz_pillar", "quartz_pillar"),
	156: LegacySimple("quartz_stairs"),
	157: LegacySimple("activator_rail"),
	158: LegacySimple("dropper"),
	159: LegacyDyed("terracotta"),
	160: LegacyDyed("stained_glass_pane"),
	161: LegacyByMetaMod("acacia_leaves", "dark_oak_leaves"),
	162: LegacyByMetaMod("acacia_log", "dark_oak_log"),
	163: LegacySimple("acacia_stairs"),
	164: LegacySimple("dark_oak_stairs"),
	165: LegacySimple("slime_block"),
	166: LegacySimple("barrier"),
	167: LegacySimple("iron_trapdoor"),
	168: LegacyByMeta("prismarine", "prismarine_bricks", "dark_prismarine"),
	169: LegacySimple("sea_lantern"),
	170: LegacySimple("hay_block"),
	171: LegacyDyed("carpet"),
	172: LegacySimple("terracotta"),
	173: LegacySimple("coal_block"),
	174: LegacySimple("packed_ice"),
	175: LegacyByMeta("sunflower", "lilac", "tall_grass", "large_fern", "rose_bush", "peony"),
	176: LegacySimple("white_banner"),
	177: LegacySimple("white_wall_banner"),
	178: LegacySimple("daylight_detector"),
	179: LegacyByMeta("red_sandstone", "chiseled_red_sandstone", "smooth_red_sandstone"),
	180: LegacySimple("red_sandstone_stairs"),
	181: LegacySimple("red_sandstone"),
	182: LegacySimple("red_sandstone"),
	183: LegacySimple("spruce_fence_gate"),
	184: LegacySimple("birch_fence_gate"),
	185: LegacySimple("jungle_fence_gate"),
	186: LegacySimple("dark_oak_fence_gate"),
	187: LegacySimple("acacia_fence_gate"),
	188: LegacySimple("spruce_fence"),
	189: LegacySimple("birch_fence"),
	190: LegacySimple("jungle_fence"),
	191: LegacySimple("dark_oak_fence"),
	192: LegacySimple("acacia_fence"),
	193: LegacySimple("spruce_door"),
	194: LegacySimple("birch_door"),
	195: LegacySimple("jungle_door"),
	196: LegacySimple("acacia_door"),
	197: LegacySimple("dark_oak_door"),
	198: LegacySimple("end_rod"),
	199: LegacySimple("chorus_plant"),
	200: LegacySimple("chorus_flower"),
	201: LegacySimple("purpur_block"),
	202: LegacySimple("purpur_pillar"),
	203: LegacySimple("purpur_stairs"),
	204: LegacySimple("purpur_block"),
	205: LegacySimple("purpur_block"),
	206: LegacySimple("end_stone_bricks"),
	207: LegacySimple("beetroots"),
	208: LegacySimple("dirt_path"),
	209: LegacySimple("end_gateway"),
	210: LegacySimple("command_block"),
	211: LegacySimple("command_block"),
	212: LegacySimple("frosted_ice"),
	213: LegacySimple("magma_block"),
	214: LegacySimple("nether_wart_block"),
	215: LegacySimple("red_nether_bricks"),
	216: LegacySimple("bone_block"),
	217: LegacySimple("structure_void"),
	218: LegacySimple("observer"),
	219: LegacyColoredByID(0, "shulker_box"),
	220: LegacyColoredByID(1, "shulker_box"),
	221: LegacyColoredByID(2, "shulker_box"),
	222: LegacyColoredByID(3, "shulker_box"),
	223: LegacyColoredByID(4, "shulker_box"),
	224: LegacyColoredByID(5, "shulker_box"),
	225: LegacyColoredByID(6, "shulker_box"),
	226: LegacyColoredByID(7, "shulker_box"),
	227: LegacyColoredByID(8, "shulker_box"),
	228: LegacyColoredByID(9, "shulker_box"),
	229: LegacyColoredByID(10, "shulker_box"),
	230: LegacyColoredByID(11, "shulker_box"),
	231: LegacyColoredByID(12, "shulker_box"),
	232: LegacyColoredByID(13, "shulker_box"),
	233: LegacyColoredByID(14, "shulker_box"),
	234: LegacyColoredByID(15, "shulker_box"),
	235: LegacyGlazedTerracottaByID(0),
	236: LegacyGlazedTerracottaByID(1),
	237: LegacyGlazedTerracottaByID(2),
	238: LegacyGlazedTerracottaByID(3),
	239: LegacyGlazedTerracottaByID(4),
	240: LegacyGlazedTerracottaByID(5),
	241: LegacyGlazedTerracottaByID(6),
	242: LegacyGlazedTerracottaByID(7),
	243: LegacyGlazedTerracottaByID(8),
	244: LegacyGlazedTerracottaByID(9),
	245: LegacyGlazedTerracottaByID(10),
	246: LegacyGlazedTerracottaByID(11),
	247: LegacyGlazedTerracottaByID(12),
	248: LegacyGlazedTerracottaByID(13),
	249: LegacyGlazedTerracottaByID(14),
	250: LegacyGlazedTerracottaByID(15),
	251: LegacyDyed("concrete"),
	252: LegacyDyed("concrete_powder"),
	255: LegacySimple("structure_block"),
}
