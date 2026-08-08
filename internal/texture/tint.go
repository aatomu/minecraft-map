package texture

import (
	"image/color"
	"strconv"
)

// TintFunc はブロックの Properties から乗算用のTint色を計算する関数です
type TintFunc func(props map[string]string) color.RGBA

// TintResolvers は通常のバイオーム着色では表現できない特殊なブロックのTint計算関数です。
// キーは正規化済みブロックID("namespace:blockID")。
var TintResolvers = map[string]TintFunc{
	// --- 固定色（バイオーム非依存） ---
	"minecraft:spruce_leaves": constTint(0x61, 0x99, 0x61),
	"minecraft:birch_leaves":  constTint(0x80, 0xa7, 0x55),
	"minecraft:lily_pad":      constTint(0x20, 0x80, 0x30),

	// --- age駆動（茎） ---
	"minecraft:melon_stem_stage7":     stemTintFromProps,
	"minecraft:pumpkin_stem_stage7":   stemTintFromProps,
	"minecraft:attached_melon_stem":   fixedAgeStemTint(7),
	"minecraft:attached_pumpkin_stem": fixedAgeStemTint(7),
}

// constTint は固定色を返すクロージャを生成
func constTint(r, g, b uint8) TintFunc {
	return func(_ map[string]string) color.RGBA {
		return color.RGBA{R: r, G: g, B: b, A: 255}
	}
}

// stemTintFromProps は Properties["age"] を読んで計算する
func stemTintFromProps(props map[string]string) color.RGBA {
	age, _ := strconv.Atoi(props["age"]) // 未設定時は0
	return stemTint(age)
}

// fixedAgeStemTint は age を固定値に決め打ちしたクロージャを返す
func fixedAgeStemTint(age int) TintFunc {
	return func(_ map[string]string) color.RGBA {
		return stemTint(age)
	}
}

func stemTint(age int) color.RGBA {
	if age < 0 {
		age = 0
	} else if age > 7 {
		age = 7
	}
	r := age * 32
	if r > 255 {
		r = 255
	}
	return color.RGBA{
		R: uint8(r),
		G: uint8(255 - age*8),
		B: uint8(age * 4),
		A: 255,
	}
}
