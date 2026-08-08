package mapimg

import "image/color"

// FallbackColors はチャンク/リージョンを塗りつぶす際に使う代替色です。
// (旧 main.go の FallbackColors を移設)
type FallbackColors struct {
	Ungenerated  color.RGBA // 未生成チャンク
	RegionError  color.RGBA // リージョンファイル自体が開けない(欠損・破損等)
	ChunkError   color.RGBA // リージョン内の個別チャンクの読み込み(I/O・解凍)失敗
	ParseError   color.RGBA // チャンクNBTのパース失敗(データ破損等)
	Void         color.RGBA // 全ブロックがsuppress対象(air等)で、本来ブロックが存在しない正常な空洞
	MissingColor color.RGBA // ブロックは検出できたが map_color.json に該当する色情報が無い
	Other        color.RGBA // 上記いずれにも該当しない、予期しない状態(通常は発生しない)
}

// ToRGBA は config.json の [4]uint8 表現を color.RGBA に変換します(旧 main.go の toRGBA)
func ToRGBA(c [4]uint8) color.RGBA {
	return color.RGBA{R: c[0], G: c[1], B: c[2], A: c[3]}
}
