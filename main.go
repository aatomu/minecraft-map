package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Config struct {
	RegionDir    string `json:"regionDir"`
	ClientJar    string `json:"clientJar"`
	MapColorJson string `json:"mapColorJson"`
	Export       struct {
		Dir      string `json:"dir"`
		Shading  bool   `json:"shading"`
		ByRegion bool   `json:"byRegion"`
	} `json:"export"`
	FallBackColor struct {
		Ungenerated [4]uint8 `json:"ungenerated"` // 未生成
		ReadError   [4]uint8 `json:"readError"`   // 読み込み/解析エラー
		Other       [4]uint8 `json:"other"`       // その他(未登録ブロック等)
	} `json:"fallbackColor"`
	FallbackId map[string]string `json:"fallbackId"`
}

type FallbackColors struct {
	Ungenerated color.RGBA // 未生成
	ReadError   color.RGBA // 読み込み/解析エラー
	Other       color.RGBA // その他(未登録ブロック等)
}

var (
	config Config
)

func init() {
	log.Println("[INFO] Starting to load config.json")
	f, err := os.Open("./config.json")
	if err != nil {
		log.Fatalf("[FATAL] Failed to open config.json: %v", err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatalf("[FATAL] Failed to decode config.json: %v", err)
	}
	log.Println("[INFO] Success to load config.json")
}

func main() {
	cmd := os.Args[1]

	switch cmd {
	case "color":
		log.Println("[INFO] Starting to parse textures")
		err := ExtractMapColors(config.ClientJar, config.MapColorJson)
		if err != nil {
			log.Fatalf("[FATAL] Failed to parse textures: %v", err)
		}
		log.Println("[INFO] Success to parse textures")
	case "generate":
		// 1. map_color.json を取得
		blockColor, colorMap, err := LoadMapColors(config.MapColorJson)
		if err != nil {
			log.Fatalf("[FATAL] Failed to read map color: %v", err)
		}

		// 2. ディレクトリ内の全 .mca ファイルを検出
		files, err := os.ReadDir(config.RegionDir)
		if err != nil {
			log.Fatalf("[FATAL] Failed to open region directory: %v", err)
		}

		var regionList []RegionPos
		for _, f := range files {
			if f.IsDir() || !strings.HasPrefix(f.Name(), "r.") || !strings.HasSuffix(f.Name(), ".mca") {
				continue
			}
			parts := strings.Split(f.Name(), ".")
			if len(parts) == 4 {
				rx, err1 := strconv.Atoi(parts[1])
				rz, err2 := strconv.Atoi(parts[2])
				if err1 == nil && err2 == nil {
					regionList = append(regionList, RegionPos{X: rx, Z: rz})
				}
			}
		}

		if len(regionList) == 0 {
			log.Fatalf("[FATAL] Target region not found: %v", err)
		}

		fallback := FallbackColors{
			Ungenerated: toRGBA(config.FallBackColor.Ungenerated),
			ReadError:   toRGBA(config.FallBackColor.ReadError),
			Other:       toRGBA(config.FallBackColor.Other),
		}
		err = exportMap(config.RegionDir, regionList, fallback, colorMap, blockColor, config.Export.Shading, filepath.Join(config.Export.Dir, "map.png"))
		if err != nil {
			log.Fatalf("[FATAL] Failed to export: %v\n", err)
		} else {
			log.Println("[INFO] Success to export")
		}
	}
}

func toRGBA(c [4]uint8) color.RGBA {
	return color.RGBA{R: c[0], G: c[1], B: c[2], A: c[3]}
}

func WorldToAbsoluteRegionXZ(blockX, blockZ int) (regionX, regionZ int) {
	return blockX >> 9, blockZ >> 9 // 1リージョン = 512ブロック (2^9)
}
func WorldToAbsoluteChunkXZ(blockX, blockZ int) (chunkX, chunkZ int) {
	return blockX >> 4, blockZ >> 4 // 1チャンク = 16ブロック (2^4)
}
func WorldToChunkRelativePos(blockX, blockZ int) (localX, localZ int) {
	return (blockX%16 + 16) % 16, (blockZ%16 + 16) % 16
}

// exportMap は全リージョンからブロックを読み込んでマップ画像を出力します
func exportMap(rootDir string, regionList []RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]BlockColorInfo, shading bool, filepath string) error {
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

	log.Printf("[INFO] Detected regens: %d (range: X[%d..%d], Z[%d..%d])\n", len(regionList), minRX, maxRX, minRZ, maxRZ)
	log.Printf("[INFO] Image size: %dx%d px\n", imgWidth, imgHeight)

	canvas := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// 高低差シェーディング用高さバッファ
	heightBuffer := slices.Repeat([]int{math.MinInt}, imgWidth*imgHeight)

	// ---------------------------------------------------------
	//  並列処理の設定 (セマフォで同時実行数をCPUコア数に制限)
	// ---------------------------------------------------------
	numWorkers := runtime.NumCPU() // システムの論理CPU数を使用
	log.Printf("[INFO] Processing regions in parallel using %d workers...\n", numWorkers)

	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	var completed int32 = 0
	for idx, rPos := range regionList {
		wg.Add(1)
		sem <- struct{}{} // 枠を1つ確保（numWorkers を超えたら待機）

		go func(idx int, rPos RegionPos) {
			defer wg.Done()
			defer func() {
				c := atomic.AddInt32(&completed, 1)
				log.Printf("[%d/%d] r.%d.%d.mca has processed.\n", c, len(regionList), rPos.X, rPos.Z)
				<-sem
			}()

			region, err := OpenRegion(rootDir, rPos.X, rPos.Z)
			if err != nil {
				fillRegionColor(canvas, rPos, minRX, minRZ, fallback.ReadError)
				return
			}
			defer region.Close()

			for cz := 0; cz < 32; cz++ {
				for cx := 0; cx < 32; cx++ {
					absChunkX := rPos.X*32 + cx
					absChunkZ := rPos.Z*32 + cz

					chunkData, err := region.ReadChunk(absChunkX, absChunkZ)
					if err != nil {
						switch chunkData.Status {
						case ChunkStatusError:
							fillChunkColor(canvas, absChunkX, absChunkZ, minRX, minRZ, fallback.ReadError)
						case ChunkStatusNotGenerated:
							fillChunkColor(canvas, absChunkX, absChunkZ, minRX, minRZ, fallback.Ungenerated)
						}
						continue
					}

					chunkBlocks, err := ParseChunkBlocksDynamic(chunkData.UncompressedData)
					if err != nil {
						fillChunkColor(canvas, absChunkX, absChunkZ, minRX, minRZ, fallback.ReadError)
						continue
					}

					for lx := 0; lx < 16; lx++ {
						for lz := 0; lz < 16; lz++ {
							worldX := absChunkX*16 + lx
							worldZ := absChunkZ*16 + lz

							pixelX := worldX - minRX*512
							pixelZ := worldZ - minRZ*512

							var layers []color.RGBA
							topSolidY := math.MinInt

							for y := chunkBlocks.MaxY; y >= chunkBlocks.MinY; y-- {
								b := chunkBlocks.GetBlock(lx, y, lz)

								if isAir(b.Name) {
									continue
								}

								cleanName := strings.TrimPrefix(b.Name, "minecraft:")

								info, ok := blockColors[cleanName]
								if !ok {
									if cleanName == "grass_block" || cleanName == "grass_block_snow" {
										info = BlockColorInfo{Color: [4]uint8{120, 120, 120, 255}, BiomeType: "grass"}
										ok = true
									} else if strings.Contains(cleanName, "leaves") {
										info = BlockColorInfo{Color: [4]uint8{120, 120, 120, 255}, BiomeType: "foliage"}
										ok = true
									} else if cleanName == "water" {
										info = BlockColorInfo{Color: [4]uint8{64, 128, 255, 180}, BiomeType: "water"}
										ok = true
									} else if cleanName == "lava" {
										info = BlockColorInfo{Color: [4]uint8{255, 90, 0, 255}, BiomeType: "none"}
										ok = true
									}
								}

								if tintf, hasTint := tintResolvers[cleanName]; hasTint {
									base := color.RGBA{R: info.Color[0], G: info.Color[1], B: info.Color[2], A: info.Color[3]}
									tinted := multiplyColor(base, tintf(b.Properties))
									info.Color = [4]uint8{tinted.R, tinted.G, tinted.B, info.Color[3]}
								}

								if ok && b.Properties["waterlogged"] == "true" {
									waterInfo := BlockColorInfo{Color: [4]uint8{64, 128, 255, 90}, BiomeType: "none"}
									if w, ok := blockColors["water"]; ok {
										waterInfo = w
									}
									tinted := multiplyColor(
										color.RGBA{info.Color[0], info.Color[1], info.Color[2], info.Color[3]},
										color.RGBA{waterInfo.Color[0], waterInfo.Color[1], waterInfo.Color[2], 255},
									)
									info.Color = [4]uint8{tinted.R, tinted.G, tinted.B, info.Color[3]}
								}

								if ok {
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
							}

							if len(layers) > 0 {
								finalColor := blendLayers(layers)
								// ピクセル領域が他のリージョンと重複しないため安全に直接セット可能
								canvas.Set(pixelX, pixelZ, finalColor)
								heightBuffer[pixelX+imgWidth*pixelZ] = topSolidY
							} else {
								canvas.Set(pixelX, pixelZ, fallback.Other)
							}
						}
					}
				}
			}
		}(idx, rPos)
	}

	// 全リージョンの処理完了を待機
	wg.Wait()

	// 高低差 (ΔY) によるシェーディングの適用
	if shading {
		log.Println("[INFO] Applying height difference shading...")
		for x := 0; x < imgWidth; x++ {
			for z := 0; z < imgHeight; z++ {
				currY := heightBuffer[x+imgWidth*z]
				if currY == math.MinInt {
					continue
				}

				// 北隣 (Z - 1) の高さと比較
				northY := currY
				if z > 0 && heightBuffer[x+(imgWidth*(z-1))] != math.MinInt {
					northY = heightBuffer[x+(imgWidth*(z-1))]
				}

				deltaY := currY - northY
				factor := 1.0
				if deltaY > 0 {
					factor = 1.15 // 高い場合は明るく
				} else if deltaY < 0 {
					factor = 0.85 // 低い場合は暗く
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

	log.Printf("[INFO]: Saving map image: %s\n", filepath)
	outFile, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if err := png.Encode(outFile, canvas); err != nil {
		panic(err)
	}

	return nil
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
