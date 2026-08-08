package mapimg

import (
	"image"
	"image/color"
	"math"
)

// applyShading: 行優先アクセス(z外側, x内側)へ変更し、キャッシュ効率を最適化
func applyShading(canvas *image.RGBA, heightBuffer []int32, width, height int) {
	for z := 0; z < height; z++ {
		for x := 0; x < width; x++ {
			currY := heightBuffer[x+width*z]
			if currY == math.MinInt32 {
				continue
			}

			northY := currY
			if z > 0 && heightBuffer[x+(width*(z-1))] != math.MinInt32 {
				northY = heightBuffer[x+(width*(z-1))]
			}

			deltaY := currY - northY
			factor := 1.0
			if deltaY > 0 {
				factor = 1.15
			} else if deltaY < 0 {
				factor = 0.85
			}

			if factor != 1.0 {
				origColor := canvas.RGBAAt(x, z)
				canvas.SetRGBA(x, z, color.RGBA{
					R: uint8(math.Min(255, float64(origColor.R)*factor)),
					G: uint8(math.Min(255, float64(origColor.G)*factor)),
					B: uint8(math.Min(255, float64(origColor.B)*factor)),
					A: origColor.A,
				})
			}
		}
	}
}
