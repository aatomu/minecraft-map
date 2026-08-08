// Package texture はリソースパック(.jar/.zip/展開済みディレクトリ)からブロックの
// 平均色・カラーマップ(バイオーム着色用グラデーション画像)を抽出し、
// map_color.json への保存/読み込み、およびレンダリング時の色解決(バイオーム/Tint適用)を担当します。
package texture

import "image/color"

// BlockColorInfo は各ブロックの情報構造
type BlockColorInfo struct {
	Color     [4]uint8 `json:"color"`     // [R, G, B, A]
	BiomeType string   `json:"biomeType"` // "none" | "grass" | "water" | "foliage" など
}

// MapData は map_color.json 全体の構造
type MapData struct {
	ColorMap map[string]string         `json:"color_map"` // キー: カラーマップ名 ("grass", "foliage" 等), 値: Base64文字列
	Blocks   map[string]BlockColorInfo `json:"blocks"`
}

// ModelJSON はブロックモデル定義(assets/*/models/block/*.json)の必要部分だけを表します
type ModelJSON struct {
	Parent   string            `json:"parent"`
	Textures map[string]string `json:"textures"`
}

// BiomeData はバイオームごとの気候パラメータと水面色です
type BiomeData struct {
	Temp     float64
	Downfall float64
	Water    color.RGBA
}
