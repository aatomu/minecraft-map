package mapimg

import "image/color"

// FallbackColors はチャンク/リージョンを塗りつぶす際に使う代替色です。
// (旧 main.go の FallbackColors を移設)
type FallbackColors struct {
	Ungenerated color.RGBA // 未生成
	ReadError   color.RGBA // 読み込み/解析エラー
	Other       color.RGBA // その他(未登録ブロック等)
}

// ToRGBA は config.json の [4]uint8 表現を color.RGBA に変換します(旧 main.go の toRGBA)
func ToRGBA(c [4]uint8) color.RGBA {
	return color.RGBA{R: c[0], G: c[1], B: c[2], A: c[3]}
}
