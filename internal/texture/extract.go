package texture

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/aatomu/minecraft-map/internal/block"
)

// ExtractMapColors は resourcePaths (config.json の "resources" 配列) を先頭から順に読み込み、
// 末尾のソースほど優先して同名パスを上書きしながら色情報を抽出します。
func ExtractMapColors(resourcePaths []string, jsonPath string) error {
	entries, closers, err := collectResourceEntries(resourcePaths)
	if err != nil {
		return fmt.Errorf("Failed to load resources: %w", err)
	}
	defer closeAll(closers)

	colorMapB64 := make(map[string]string)

	// カラーマップ画像の全自動/動的スキャン
	// assets/<namespace>/textures/colormap/*.png をすべて検出 (namespace は問わない)
	for path, entry := range entries {
		parts := strings.Split(path, "/")
		if len(parts) != 5 || parts[0] != "assets" || parts[2] != "textures" || parts[3] != "colormap" || !strings.HasSuffix(parts[4], ".png") {
			continue
		}
		ns := parts[1]
		keyName := strings.TrimSuffix(parts[4], ".png")

		img := decodeResourceImage(entry)
		if img == nil {
			continue
		}

		mapKey := keyName
		if ns != "minecraft" {
			// minecraft 以外の namespace は名前衝突を避けるため接頭辞を付与
			mapKey = ns + ":" + keyName
		}
		colorMapB64[mapKey] = encodeImageToBase64(img)
	}

	resultBlocks := make(map[string]BlockColorInfo)

	// 特殊流体ブロック (water / lava) の定義 (バニラ固定: minecraft 名前空間)
	fluidMap := map[string]string{
		"water": "assets/minecraft/textures/block/water_still.png",
		"lava":  "assets/minecraft/textures/block/lava_still.png",
	}
	for name, path := range fluidMap {
		if entry, ok := entries[path]; ok {
			avgColor := calculateAverageColor(entry)
			bType := "none"
			if name == "water" {
				bType = "water"
				avgColor[3] = 180 // 水に透過性を設定
			}
			resultBlocks[block.Normalize(name)] = BlockColorInfo{Color: avgColor, BiomeType: bType}
		}
	}

	// 各ブロックモデル JSON の解析
	// assets/<namespace>/models/block/*.json をすべて検出 (namespace は問わない)
	for path := range entries {
		parts := strings.Split(path, "/")
		if len(parts) != 5 || parts[0] != "assets" || parts[2] != "models" || parts[3] != "block" || !strings.HasSuffix(parts[4], ".json") {
			continue
		}

		ns := parts[1]
		blockName := strings.TrimSuffix(parts[4], ".json")
		blockID := ns + ":" + blockName

		texNamespace, texPath, err := resolveTexturePath(ns, path, entries)
		if err != nil || texPath == "" {
			continue
		}

		jarTexturePath := fmt.Sprintf("assets/%s/textures/%s.png", texNamespace, texPath)
		texEntry, exists := entries[jarTexturePath]
		if !exists {
			continue
		}

		avgColor := calculateAverageColor(texEntry)
		biomeType := determineBiomeType(blockName)

		resultBlocks[block.Normalize(blockID)] = BlockColorInfo{
			Color:     avgColor,
			BiomeType: biomeType,
		}
	}

	// 構造体を作成して保存
	mapData := MapData{
		ColorMap: colorMapB64,
		Blocks:   resultBlocks,
	}

	jsonData, err := json.MarshalIndent(mapData, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal map_color.json: %w", err)
	}
	return os.WriteFile(jsonPath, jsonData, 0644)
}

func determineBiomeType(blockID string) string {
	if blockID == "water" {
		return "water"
	}
	if blockID == "vine" {
		return "grass"
	}
	if strings.Contains(blockID, "grass") || strings.Contains(blockID, "fern") {
		return "grass"
	}
	if strings.Contains(blockID, "leaves") && !strings.Contains(blockID, "spruce") && !strings.Contains(blockID, "birch") && !strings.Contains(blockID, "pale") {
		return "foliage"
	}
	return "none"
}

func calculateAverageColor(entry fileEntry) [4]uint8 {
	img := decodeResourceImage(entry)
	if img == nil {
		return [4]uint8{128, 128, 128, 255}
	}

	bounds := img.Bounds()
	// アニメーションテクスチャ(高さが幅の倍数)は先頭フレームのみ使用
	if bounds.Dy() > bounds.Dx() && bounds.Dy()%bounds.Dx() == 0 {
		bounds = image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+bounds.Dx(), bounds.Min.Y+bounds.Dx())
	}
	var totalR, totalG, totalB, totalA uint64
	var count uint64

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 {
				totalR += uint64(r >> 8)
				totalG += uint64(g >> 8)
				totalB += uint64(b >> 8)
				totalA += uint64(a >> 8)
				count++
			}
		}
	}

	if count == 0 {
		return [4]uint8{128, 128, 128, 255}
	}

	return [4]uint8{
		uint8(totalR / count),
		uint8(totalG / count),
		uint8(totalB / count),
		uint8(totalA / count),
	}
}
