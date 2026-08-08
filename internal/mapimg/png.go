package mapimg

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"slices"
)

// savePNG は canvas を PNG として書き出します。
// exportMode:
//   - "default":     可逆RGBA PNG (旧 config.Export.Mode == "default")
//   - "compression": ImageMagick の "-quantize YUV +dither -colors 256 -type Palette" 相当の
//     疑似パレット化を行い、ファイルサイズを削減します。
func savePNG(filePath string, img image.Image, exportMode string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// システムコール回数を減らすため bufio.Writer でラップする
	w := bufio.NewWriterSize(f, 256*1024)
	defer w.Flush()

	switch exportMode {
	case "default":
		if err := png.Encode(w, img); err != nil {
			return err
		}
		return w.Flush()
	case "compression":
		// --- ImageMagick: -quantize YUV +dither -colors 256 -type Palette 相当の処理 ---
		var finalImg image.Image = img

		if rgba, ok := img.(*image.RGBA); ok {
			bounds := rgba.Bounds()

			// 1. 画像から出現頻度の高い色を中心に256色のパレットを抽出 (-colors 256)
			colorCount := make(map[color.RGBA]int)
			for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 { // 高速化のためステップサンプリング
				for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
					colorCount[rgba.RGBAAt(x, y)]++
				}
			}

			type colorFreq struct {
				col   color.RGBA
				count int
			}
			var freqs []colorFreq
			for c, count := range colorCount {
				freqs = append(freqs, colorFreq{c, count})
			}
			slices.SortFunc(freqs, func(a, b colorFreq) int {
				return b.count - a.count
			})

			var palette color.Palette
			for _, f := range freqs {
				if len(palette) >= 256 {
					break
				}
				palette = append(palette, f.col)
			}

			// 2. YUV色空間への変換関数 (-quantize YUV)
			rgbToYUV := func(c color.RGBA) (float64, float64, float64) {
				r, g, b := float64(c.R), float64(c.G), float64(c.B)
				yVal := 0.299*r + 0.587*g + 0.114*b
				uVal := -0.14713*r - 0.28886*g + 0.436*b
				vVal := 0.615*r - 0.51499*g - 0.10001*b
				return yVal, uVal, vVal
			}

			// パレット色のYUV値はピクセル数分ループする間ずっと不変のため、
			// ピクセルごとに再計算せずここで1回だけ変換してキャッシュしておく
			type paletteYUV struct {
				y, u, v float64
				a       uint8
			}
			paletteCache := make([]paletteYUV, len(palette))
			for i, pc := range palette {
				pr, pg, pb, pa := pc.RGBA()
				pColor := color.RGBA{R: uint8(pr >> 8), G: uint8(pg >> 8), B: uint8(pb >> 8), A: uint8(pa >> 8)}
				y, u, v := rgbToYUV(pColor)
				paletteCache[i] = paletteYUV{y: y, u: u, v: v, a: pColor.A}
			}

			// YUV距離計算 (輝度Yを重視)
			yuvDistanceSq := func(y1, u1, v1, y2, u2, v2 float64) float64 {
				dy, du, dv := y1-y2, u1-u2, v1-v2
				return dy*dy*1.5 + du*du + dv*dv
			}

			// 最も近いパレット色を検索 (+dither 相当のベタ塗り)
			findNearestIndex := func(c color.RGBA) uint8 {
				if c.A == 0 {
					for i, pc := range palette {
						if _, _, _, a := pc.RGBA(); a == 0 {
							return uint8(i)
						}
					}
				}
				y, u, v := rgbToYUV(c)
				minDist := math.MaxFloat64
				bestIdx := 0

				for i, pc := range paletteCache {
					if math.Abs(float64(c.A)-float64(pc.a)) > 128 {
						continue
					}
					dist := yuvDistanceSq(pc.y, pc.u, pc.v, y, u, v)
					if dist < minDist {
						minDist = dist
						bestIdx = i
					}
				}
				return uint8(bestIdx)
			}

			// 3. Paletted 画像への変換 (-type Palette / png:color-type=3)
			palettedImg := image.NewPaletted(bounds, palette)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					c := rgba.RGBAAt(x, y)
					palettedImg.SetColorIndex(x, y, findNearestIndex(c))
				}
			}

			finalImg = palettedImg
		}

		// エンコーダーも標準固定でそのまま出力
		encoder := png.Encoder{
			CompressionLevel: png.BestCompression,
		}
		if err := encoder.Encode(w, finalImg); err != nil {
			return err
		}
		return w.Flush()
	}

	return fmt.Errorf("invalid export mode")
}
