// Package chunk はチャンクの生NBTデータ(region.ChunkData.UncompressedData)を
// 座標問い合わせ可能なブロック/バイオーム情報(ChunkBlocksDynamic)へ変換します。
// 1.18+ の sections 形式、1.13〜1.17 の Level.Sections 形式、
// 1.12.2以下 の数値ID+メタデータ形式のいずれにも対応します。
// Package blockid は "minecraft:stone" のような namespace 付きブロックID文字列を
// 正規化するための共通ユーティリティです。他の全パッケージから参照される最小の依存です。
package block

import "strings"

// Block は1マスのブロック情報を表します
type Block struct {
	Name       string
	Properties map[string]string
}

func Normalize(id string) string {
	if !strings.Contains(id, ":") {
		return "minecraft:" + id
	}
	return id
}
