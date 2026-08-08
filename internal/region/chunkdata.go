package region

import "errors"

// ErrChunkNotGenerated は該当座標のチャンクがまだ生成されていないことを示すセンチネルエラーです。
// 「読み込みエラー」と区別するために使用します。
var ErrChunkNotGenerated = errors.New("chunk has not generated")

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

// ChunkData は .mca / .mcc から読み出した「解凍済みだが未パース」のチャンク生データです。
// 実際のブロック情報への変換は internal/chunk パッケージが担当します。
type ChunkData struct {
	X, Z             int
	CompressionType  byte
	UncompressedData []byte
	IsExternal       bool        // .mcc 外部ファイルから読み込まれたかのフラグ
	Status           ChunkStatus // チャンクの生成/読み込みステータス
}
