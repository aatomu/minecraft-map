package mapimg

import (
	"image"
	"image/color"
	"math"

	"github.com/aatomu/minecraft-map/internal/region"
)

// fillChunkColor は指定した絶対チャンク座標に対応する16x16ピクセル範囲を単色で塗りつぶします
func fillChunkColor(canvas *image.RGBA, absChunkX, absChunkZ, minRX, minRZ int, c color.RGBA) {
	baseX := absChunkX*16 - minRX*512
	baseZ := absChunkZ*16 - minRZ*512
	bounds := canvas.Bounds()

	for lx := 0; lx < 16; lx++ {
		for lz := 0; lz < 16; lz++ {
			px, pz := baseX+lx, baseZ+lz
			if px < bounds.Min.X || px >= bounds.Max.X || pz < bounds.Min.Y || pz >= bounds.Max.Y {
				continue
			}
			canvas.Set(px, pz, c)
		}
	}
}

// fillRegionColor は指定リージョン全体（32x32チャンク = 512x512ブロック）を単色で塗りつぶします
func fillRegionColor(canvas *image.RGBA, rPos region.RegionPos, minRX, minRZ int, c color.RGBA) {
	for cz := 0; cz < 32; cz++ {
		for cx := 0; cx < 32; cx++ {
			fillChunkColor(canvas, rPos.X*32+cx, rPos.Z*32+cz, minRX, minRZ, c)
		}
	}
}

// blendLayers は上層から下層へのカラーレイヤーを重ね合わせて1つの色に合成します
func blendLayers(layers []color.RGBA) color.RGBA {
	if len(layers) == 0 {
		return color.RGBA{0, 0, 0, 0}
	}

	// 最下層から順にブレンド
	accR, accG, accB, accA := float64(layers[len(layers)-1].R), float64(layers[len(layers)-1].G), float64(layers[len(layers)-1].B), float64(layers[len(layers)-1].A)/255.0

	for i := len(layers) - 2; i >= 0; i-- {
		top := layers[i]
		topA := float64(top.A) / 255.0

		accR = float64(top.R)*topA + accR*(1.0-topA)
		accG = float64(top.G)*topA + accG*(1.0-topA)
		accB = float64(top.B)*topA + accB*(1.0-topA)
		accA = topA + accA*(1.0-topA)
	}

	return color.RGBA{
		R: uint8(math.Min(255, accR)),
		G: uint8(math.Min(255, accG)),
		B: uint8(math.Min(255, accB)),
		A: uint8(math.Min(255, accA*255.0)),
	}
}
