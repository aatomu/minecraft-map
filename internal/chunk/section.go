package chunk

import "github.com/aatomu/minecraft-map/internal/block"

// dataVersionNoCrossingBoundary は、ブロック配列のビットパック形式が変更された
// スナップショット 20w17a (1.16) の DataVersion です。
// これ以上であれば「エントリがlong境界をまたがない」新形式、未満であれば
// 「エントリがlong境界をまたぐことがある」旧形式(1.13〜1.15)として扱います。
const dataVersionNoCrossingBoundary = 2529

// SectionData は1セクション(16x16x16)分のパレットと元データ配列を保持します
type SectionData struct {
	Y            int8
	Palette      []block.Block
	Data         []int64 // 圧縮ビット配列 raw データ
	BitsPerBlock int     // エントリあたりのビット数
	LegacyPack   bool    // true の場合、エントリがlong境界をまたぐ旧(〜1.15)形式として読む

	// --- 1.13未満(数値ID+メタデータ形式)専用フィールド ---
	IsNumeric     bool   // true の場合、Palette ではなく数値ID+メタデータ形式として扱う
	NumericBlocks []byte // "Blocks" タグ: 4096要素、ブロックIDの下位8bit
	NumericAdd    []byte // "Add" タグ (任意): 2048要素のnibble配列、ブロックIDの上位4bit拡張
	NumericData   []byte // "Data" タグ: 2048要素のnibble配列、メタデータ(0-15)
}

// getNibble はnibble配列(1バイトに2要素を詰めた配列)から指定インデックスの4bit値を取り出します
func getNibble(arr []byte, i int) int {
	if arr == nil {
		return 0
	}
	byteIdx := i / 2
	if byteIdx >= len(arr) {
		return 0
	}
	if i%2 == 0 {
		return int(arr[byteIdx] & 0x0F)
	}
	return int(arr[byteIdx] >> 4 & 0x0F)
}

// GetNumericBlock は 1.13未満 の数値ID+メタデータ形式のセクションから
// 指定インデックス(0..4095)のブロックを解決します。
func (s *SectionData) GetNumericBlock(i int) block.Block {
	if i < 0 || i >= len(s.NumericBlocks) {
		return block.Block{Name: "minecraft:air"}
	}
	id := int(s.NumericBlocks[i])
	if s.NumericAdd != nil {
		id |= getNibble(s.NumericAdd, i) << 8
	}
	meta := getNibble(s.NumericData, i)
	return block.LegacyBlockFromIDMeta(id, meta)
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
