package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
)

// RegionRenderResult は 1 リージョン分のレンダリング結果データ
type RegionRenderResult struct {
	Pos           RegionPos
	Img           *image.RGBA
	HeightMap     []int
	MissingBlocks []string
}

// renderRegion は 1 つのリージョン (.mca) を解析し、画像データと高さバッファを返します
func renderRegion(rootDir string, rPos RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]BlockColorInfo, suppressMap map[string]bool) (*RegionRenderResult, error) {
	imgSize := 512
	canvas := image.NewRGBA(image.Rect(0, 0, imgSize, imgSize))
	heightBuffer := slices.Repeat([]int{math.MinInt}, imgSize*imgSize)

	region, err := OpenRegion(rootDir, rPos.X, rPos.Z)
	if err != nil {
		fillRegionColor(canvas, rPos, rPos.X, rPos.Z, fallback.ReadError)
		return &RegionRenderResult{Pos: rPos, Img: canvas, HeightMap: heightBuffer}, nil
	}
	defer region.Close()

	missingBlocksSet := make(map[string]struct{})
	for cz := 0; cz < 32; cz++ {
		for cx := 0; cx < 32; cx++ {
			absChunkX := rPos.X*32 + cx
			absChunkZ := rPos.Z*32 + cz

			chunkData, err := region.ReadChunk(absChunkX, absChunkZ)
			if err != nil {
				switch chunkData.Status {
				case ChunkStatusError:
					log.Printf("[WARN] %d,%d in r.%d.%d.mca read error: %v\n", absChunkX, absChunkZ, rPos.X, rPos.Z, err)
					fillChunkColor(canvas, absChunkX, absChunkZ, rPos.X, rPos.Z, fallback.ReadError)
				case ChunkStatusNotGenerated:
					fillChunkColor(canvas, absChunkX, absChunkZ, rPos.X, rPos.Z, fallback.Ungenerated)
				}
				continue
			}

			chunkBlocks, err := ParseChunkBlocksDynamic(chunkData.UncompressedData)
			if err != nil {
				fillChunkColor(canvas, absChunkX, absChunkZ, rPos.X, rPos.Z, fallback.ReadError)
				continue
			}

			for lx := 0; lx < 16; lx++ {
				for lz := 0; lz < 16; lz++ {
					pixelX := cx*16 + lx
					pixelZ := cz*16 + lz

					var layers []color.RGBA
					topSolidY := math.MinInt

					for y := chunkBlocks.MaxY; y >= chunkBlocks.MinY; y-- {
						b := chunkBlocks.GetBlock(lx, y, lz)

						if _, hit := suppressMap[b.Name]; hit {
							continue
						}

						cleanName := b.Name
						if fallbackId, ok := config.FallbackBlocks[cleanName]; ok {
							cleanName = fallbackId
						}

						info, ok := blockColors[cleanName]
						if !ok {
							missingBlocksSet[cleanName] = struct{}{}
							continue
						}

						// 水没判定
						if b.Properties["waterlogged"] == "true" {
							waterInfo := BlockColorInfo{Color: [4]uint8{64, 128, 255, 90}, BiomeType: "none"}
							if w, ok := blockColors["minecraft:water"]; ok {
								waterInfo = w
							}
							tinted := multiplyColor(
								color.RGBA{R: info.Color[0], G: info.Color[1], B: info.Color[2], A: info.Color[3]},
								color.RGBA{R: waterInfo.Color[0], G: waterInfo.Color[1], B: waterInfo.Color[2], A: 255},
							)
							info.Color = [4]uint8{tinted.R, tinted.G, tinted.B, info.Color[3]}
						}

						// 特殊なTint対応
						if tintf, hasTint := tintResolvers[cleanName]; hasTint {
							base := color.RGBA{R: info.Color[0], G: info.Color[1], B: info.Color[2], A: info.Color[3]}
							tinted := multiplyColor(base, tintf(b.Properties))
							info.Color = [4]uint8{tinted.R, tinted.G, tinted.B, info.Color[3]}
						}

						biomeName := chunkBlocks.GetBiome(lx, y, lz)
						blockColor := GetTintedColor(info, biomeName, colorMap)

						if topSolidY == math.MinInt {
							topSolidY = y
						}

						layers = append(layers, blockColor)

						if blockColor.A == 255 {
							break
						}
					}

					if len(layers) > 0 {
						finalColor := blendLayers(layers)
						canvas.Set(pixelX, pixelZ, finalColor)
						heightBuffer[pixelX+imgSize*pixelZ] = topSolidY
					} else {
						canvas.Set(pixelX, pixelZ, fallback.Other)
					}
				}
			}
		}
	}

	missingList := []string{}
	if len(missingBlocksSet) > 0 {
		for k := range missingBlocksSet {
			missingList = append(missingList, k)
		}
		log.Printf("[WARN] r.%d.%d.mca missing blocks: %v\n", rPos.X, rPos.Z, missingList)
	}

	return &RegionRenderResult{
		Pos:           rPos,
		Img:           canvas,
		HeightMap:     heightBuffer,
		MissingBlocks: missingList,
	}, nil
}

// exportMapRegion は各リージョンを個別の PNG ファイルとして出力します
func exportMapRegion(rootDir string, regionList []RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]BlockColorInfo, suppressMap map[string]bool, shading bool, exportDir string) error {
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return fmt.Errorf("failed to create export dir: %w", err)
	}

	numWorkers := runtime.NumCPU()
	log.Printf("[INFO] Processing individual regions in parallel (%d workers)...\n", numWorkers)

	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup
	var completed int32 = 0

	for _, rPos := range regionList {
		wg.Add(1)
		sem <- struct{}{}

		go func(rPos RegionPos) {
			defer wg.Done()
			defer func() {
				c := atomic.AddInt32(&completed, 1)
				log.Printf("[INFO] [%d/%d] r.%d.%d.png exported.\n", c, len(regionList), rPos.X, rPos.Z)
				<-sem
			}()

			res, err := renderRegion(rootDir, rPos, fallback, colorMap, blockColors, suppressMap)
			if err != nil {
				log.Printf("[ERROR] Failed to render r.%d.%d: %v\n", rPos.X, rPos.Z, err)
				return
			}

			if shading {
				applyShading(res.Img, res.HeightMap, 512, 512)
			}

			outPath := filepath.Join(exportDir, fmt.Sprintf("r.%d.%d.png", rPos.X, rPos.Z))
			if err := savePNG(outPath, res.Img); err != nil {
				log.Printf("[ERROR] Failed to save r.%d.%d.png: %v\n", rPos.X, rPos.Z, err)
			}
		}(rPos)
	}

	wg.Wait()
	return nil
}

// exportMapFull は全リージョンを並列処理後に結合し、1枚の巨大な PNG マップとして出力します
func exportMapFull(rootDir string, regionList []RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]BlockColorInfo, suppressMap map[string]bool, shading bool, exportPath string) error {
	minRX, minRZ := math.MaxInt, math.MaxInt
	maxRX, maxRZ := math.MinInt, math.MinInt

	for _, rPos := range regionList {
		if rPos.X < minRX {
			minRX = rPos.X
		}
		if rPos.X > maxRX {
			maxRX = rPos.X
		}
		if rPos.Z < minRZ {
			minRZ = rPos.Z
		}
		if rPos.Z > maxRZ {
			maxRZ = rPos.Z
		}
	}

	totalRegionsX := maxRX - minRX + 1
	totalRegionsZ := maxRZ - minRZ + 1
	imgWidth := totalRegionsX * 512
	imgHeight := totalRegionsZ * 512

	log.Printf("[INFO] Detected regions: %d (range: X[%d..%d], Z[%d..%d])\n", len(regionList), minRX, maxRX, minRZ, maxRZ)
	log.Printf("[INFO] Combined Image size: %dx%d px\n", imgWidth, imgHeight)

	canvas := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	fullHeightBuffer := slices.Repeat([]int{math.MinInt}, imgWidth*imgHeight)

	// Parrallel Control
	numWorkers := runtime.NumCPU()
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup
	var completed int32 = 0
	log.Printf("[INFO] Processing full map regions in parallel (%d workers)...\n", numWorkers)

	allMissingBlocksSet := make(map[string]struct{})
	results := make(chan *RegionRenderResult, len(regionList))

	for _, rPos := range regionList {
		wg.Add(1)
		sem <- struct{}{}

		go func(rPos RegionPos) {
			defer wg.Done()
			defer func() {
				c := atomic.AddInt32(&completed, 1)
				log.Printf("[INFO] [%d/%d] r.%d.%d.mca rendered.\n", c, len(regionList), rPos.X, rPos.Z)
				<-sem
			}()

			res, err := renderRegion(rootDir, rPos, fallback, colorMap, blockColors, suppressMap)
			if err == nil {
				results <- res
			}
		}(rPos)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// 収集したデータを全キャンバスへマージ
	for res := range results {
		offsetX := (res.Pos.X - minRX) * 512
		offsetZ := (res.Pos.Z - minRZ) * 512

		// 1. 画像描画領域をマージ (draw.Draw により高速コピー)
		rect := image.Rect(offsetX, offsetZ, offsetX+512, offsetZ+512)
		draw.Draw(canvas, rect, res.Img, image.Point{0, 0}, draw.Src)

		// 2. 高さマップ領域をマージ (slice copy により高速転送)
		for z := 0; z < 512; z++ {
			fullIdx := (offsetZ+z)*imgWidth + offsetX
			subIdx := z * 512
			copy(fullHeightBuffer[fullIdx:fullIdx+512], res.HeightMap[subIdx:subIdx+512])
		}

		// 3. このリージョンで発生した未登録ブロックを処理
		for _, blockID := range res.MissingBlocks {
			allMissingBlocksSet[blockID] = struct{}{}
		}
	}

	// 3. 最後に全リージョンの未登録ブロック一覧をログ表示
	if len(allMissingBlocksSet) > 0 {
		var globalMissingList []string
		for k := range allMissingBlocksSet {
			globalMissingList = append(globalMissingList, k)
		}
		// ソート表示
		slices.Sort(globalMissingList)
		log.Printf("[WARN] All missing color blocks in world (%d types): %v\n", len(globalMissingList), globalMissingList)
	}

	// 結合後の全体バッファを用いて境目も含めて綺麗にシェーディング
	if shading {
		log.Println("[INFO] Applying height difference shading on full map...")
		applyShading(canvas, fullHeightBuffer, imgWidth, imgHeight)
	}

	if err := os.MkdirAll(filepath.Dir(exportPath), 0755); err != nil {
		return fmt.Errorf("failed to create export dir: %w", err)
	}

	log.Printf("[INFO] Saving combined map image: %s\n", exportPath)
	return savePNG(exportPath, canvas)
}

// --- 共通ヘルパー関数 ---

// applyShading: 行優先アクセス(z外側, x内側)へ変更し、キャッシュ効率を最適化
func applyShading(canvas *image.RGBA, heightBuffer []int, width, height int) {
	for z := 0; z < height; z++ {
		for x := 0; x < width; x++ {
			currY := heightBuffer[x+width*z]
			if currY == math.MinInt {
				continue
			}

			northY := currY
			if z > 0 && heightBuffer[x+(width*(z-1))] != math.MinInt {
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
func fillRegionColor(canvas *image.RGBA, rPos RegionPos, minRX, minRZ int, c color.RGBA) {
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

func savePNG(filePath string, img image.Image) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	switch config.Export.Mode {
	case "default":
		return png.Encode(f, img)
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

			// YUV距離計算 (輝度Yを重視)
			yuvDistanceSq := func(c1 color.RGBA, y2, u2, v2 float64) float64 {
				y1, u1, v1 := rgbToYUV(c1)
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

				for i, pc := range palette {
					pr, pg, pb, pa := pc.RGBA()
					pColor := color.RGBA{R: uint8(pr >> 8), G: uint8(pg >> 8), B: uint8(pb >> 8), A: uint8(pa >> 8)}
					if math.Abs(float64(c.A)-float64(pColor.A)) > 128 {
						continue
					}
					dist := yuvDistanceSq(pColor, y, u, v)
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
		return encoder.Encode(f, finalImg)
	}

	return fmt.Errorf("invalid export mode")
}
