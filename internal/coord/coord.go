// Package coord はワールド座標とリージョン/チャンク座標の相互変換を提供します。
package coord

func WorldToAbsoluteRegionXZ(blockX, blockZ int) (regionX, regionZ int) {
	return blockX >> 9, blockZ >> 9 // 1リージョン = 512ブロック (2^9)
}

func WorldToAbsoluteChunkXZ(blockX, blockZ int) (chunkX, chunkZ int) {
	return blockX >> 4, blockZ >> 4 // 1チャンク = 16ブロック (2^4)
}

func WorldToChunkRelativePos(blockX, blockZ int) (localX, localZ int) {
	return (blockX%16 + 16) % 16, (blockZ%16 + 16) % 16
}
