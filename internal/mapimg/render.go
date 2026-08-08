// Package mapimg は解析済みリージョン/チャンクを1色/ピクセルへ変換し、
// PNG画像として合成・出力する責務を持ちます(旧 render.go を分割)。
package mapimg

import (
	"image"
	"image/color"
	"log"
	"math"
	"slices"

	"github.com/aatomu/minecraft-map/internal/chunk"
	"github.com/aatomu/minecraft-map/internal/region"
	"github.com/aatomu/minecraft-map/internal/texture"
)

// RegionRenderResult は 1 リージョン分のレンダリング結果データ
type RegionRenderResult struct {
	Pos           region.RegionPos
	Img           *image.RGBA
	HeightMap     []int32 // 高さは int32 で十分なため、メモリ節約のため int ではなく int32 を使用
	MissingBlocks []string
}

// renderRegion は 1 つのリージョン (.mca) を解析し、画像データと高さバッファを返します。
// fallbackBlocks は config.json の "fallbackBlocks" (未登録ブロックの代替先)です。
func renderRegion(rootDir string, rPos region.RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]texture.BlockColorInfo, suppressMap map[string]bool, fallbackBlocks map[string]string) (*RegionRenderResult, error) {
	imgSize := 512
	canvas := image.NewRGBA(image.Rect(0, 0, imgSize, imgSize))
	heightBuffer := slices.Repeat([]int32{math.MinInt32}, imgSize*imgSize)

	reg, err := region.Open(rootDir, rPos.X, rPos.Z)
	if err != nil {
		fillRegionColor(canvas, rPos, rPos.X, rPos.Z, fallback.ReadError)
		return &RegionRenderResult{Pos: rPos, Img: canvas, HeightMap: heightBuffer}, nil
	}
	defer reg.Close()

	missingBlocksSet := make(map[string]struct{})
	// ピクセルごとに再確保しないよう、ループ外で使い回すバッファを用意する
	layers := make([]color.RGBA, 0, 8)
	for cz := 0; cz < 32; cz++ {
		for cx := 0; cx < 32; cx++ {
			absChunkX := rPos.X*32 + cx
			absChunkZ := rPos.Z*32 + cz

			chunkData, err := reg.ReadChunk(absChunkX, absChunkZ)
			if err != nil {
				switch chunkData.Status {
				case region.ChunkStatusError:
					log.Printf("[WARN] %d,%d in r.%d.%d.mca read error: %v\n", absChunkX, absChunkZ, rPos.X, rPos.Z, err)
					fillChunkColor(canvas, absChunkX, absChunkZ, rPos.X, rPos.Z, fallback.ReadError)
				case region.ChunkStatusNotGenerated:
					fillChunkColor(canvas, absChunkX, absChunkZ, rPos.X, rPos.Z, fallback.Ungenerated)
				}
				continue
			}

			chunkBlocks, err := chunk.ParseChunkBlocksDynamic(chunkData.UncompressedData)
			if err != nil {
				fillChunkColor(canvas, absChunkX, absChunkZ, rPos.X, rPos.Z, fallback.ReadError)
				continue
			}

			for lx := 0; lx < 16; lx++ {
				for lz := 0; lz < 16; lz++ {
					pixelX := cx*16 + lx
					pixelZ := cz*16 + lz

					layers = layers[:0]
					topSolidY := math.MinInt

					for y := chunkBlocks.MaxY; y >= chunkBlocks.MinY; y-- {
						b := chunkBlocks.GetBlock(lx, y, lz)

						if _, hit := suppressMap[b.Name]; hit {
							continue
						}

						cleanName := b.Name
						if fallbackId, ok := fallbackBlocks[cleanName]; ok {
							cleanName = fallbackId
						}

						info, ok := blockColors[cleanName]
						if !ok {
							missingBlocksSet[cleanName] = struct{}{}
							continue
						}

						// 水没判定
						if b.Properties["waterlogged"] == "true" {
							waterInfo := texture.BlockColorInfo{Color: [4]uint8{64, 128, 255, 90}, BiomeType: "none"}
							if w, ok := blockColors["minecraft:water"]; ok {
								waterInfo = w
							}
							tinted := texture.MultiplyColor(
								color.RGBA{R: info.Color[0], G: info.Color[1], B: info.Color[2], A: info.Color[3]},
								color.RGBA{R: waterInfo.Color[0], G: waterInfo.Color[1], B: waterInfo.Color[2], A: 255},
							)
							info.Color = [4]uint8{tinted.R, tinted.G, tinted.B, info.Color[3]}
						}

						// 特殊なTint対応
						if tintf, hasTint := texture.TintResolvers[cleanName]; hasTint {
							base := color.RGBA{R: info.Color[0], G: info.Color[1], B: info.Color[2], A: info.Color[3]}
							tinted := texture.MultiplyColor(base, tintf(b.Properties))
							info.Color = [4]uint8{tinted.R, tinted.G, tinted.B, info.Color[3]}
						}

						biomeName := chunkBlocks.GetBiome(lx, y, lz)
						blockColor := texture.GetTintedColor(info, biomeName, colorMap)

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
						heightBuffer[pixelX+imgSize*pixelZ] = int32(topSolidY)
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
