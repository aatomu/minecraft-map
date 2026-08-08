package nbt

// UnpackBitArray は小規模データ（バイオーム等）用の一括アンパック関数
// (旧 unpackBitArray を公開関数化)
func UnpackBitArray(data []int64, bitsPerBlock, count int) []uint32 {
	result := make([]uint32, count)
	entriesPerLong := 64 / bitsPerBlock
	mask := uint64((1 << bitsPerBlock) - 1)

	for i := 0; i < count; i++ {
		longIndex := i / entriesPerLong
		subIndex := i % entriesPerLong

		if longIndex >= len(data) {
			break
		}

		shift := subIndex * bitsPerBlock
		value := (uint64(data[longIndex]) >> shift) & mask
		result[i] = uint32(value)
	}
	return result
}

// BitLen は n を表現するのに必要なビット数を返します(パレットのビット幅計算に使用)。
// (旧 bitLen を公開関数化)
func BitLen(n uint) int {
	count := 0
	for n > 0 {
		n >>= 1
		count++
	}
	return count
}
