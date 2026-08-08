package texture

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
)

// LoadMapColors は ExtractMapColors が生成した map_color.json を読み込み、
// ブロック色テーブルとカラーマップ画像(バイオーム着色用)を復元します。
func LoadMapColors(jsonPath string) (blockColor map[string]BlockColorInfo, colorMap map[string]image.Image, err error) {
	_, err = os.Stat(jsonPath)
	if err != nil {
		return blockColor, colorMap, fmt.Errorf("Failed to stat map_color.json: %w", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return blockColor, colorMap, fmt.Errorf("Failed to read map_color.json: %w", err)
	}

	var mapData MapData
	err = json.Unmarshal(data, &mapData)
	if err != nil {
		return blockColor, colorMap, fmt.Errorf("Failed to parse map_color.json: %w", err)
	}

	blockColor = mapData.Blocks
	colorMap = map[string]image.Image{}
	for name, b64Str := range mapData.ColorMap {
		log.Printf("[INFO] Load color map: %s\n", name)
		img := decodeBase64Image(b64Str)
		if img != nil {
			colorMap[name] = img
		} else {
			log.Printf("[WARN] Failed to decode color map: %s\n", name)
		}
	}

	return
}

// BiomeTable はバイオームごとの気候パラメータと水面色です
var BiomeTable = map[string]BiomeData{
	"minecraft:plains":           {Temp: 0.8, Downfall: 0.4, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:sunflower_plains": {Temp: 0.8, Downfall: 0.4, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:desert":           {Temp: 2.0, Downfall: 0.0, Water: color.RGBA{50, 76, 168, 255}},
	"minecraft:forest":           {Temp: 0.7, Downfall: 0.8, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:flower_forest":    {Temp: 0.7, Downfall: 0.8, Water: color.RGBA{43, 81, 224, 255}},
	"minecraft:taiga":            {Temp: 0.25, Downfall: 0.8, Water: color.RGBA{40, 90, 200, 255}},
	"minecraft:swamp":            {Temp: 0.8, Downfall: 0.9, Water: color.RGBA{97, 123, 100, 255}},
	"minecraft:jungle":           {Temp: 0.95, Downfall: 0.9, Water: color.RGBA{44, 98, 224, 255}},
	"minecraft:ocean":            {Temp: 0.5, Downfall: 0.5, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:deep_ocean":       {Temp: 0.5, Downfall: 0.5, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:warm_ocean":       {Temp: 0.5, Downfall: 0.5, Water: color.RGBA{67, 213, 238, 255}},
	"minecraft:lukewarm_ocean":   {Temp: 0.5, Downfall: 0.5, Water: color.RGBA{69, 173, 242, 255}},
	"minecraft:cold_ocean":       {Temp: 0.5, Downfall: 0.5, Water: color.RGBA{61, 87, 214, 255}},
	"minecraft:frozen_ocean":     {Temp: 0.0, Downfall: 0.5, Water: color.RGBA{57, 56, 201, 255}},
	"minecraft:birch_forest":     {Temp: 0.6, Downfall: 0.6, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:dark_forest":      {Temp: 0.7, Downfall: 0.8, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:snowy_plains":     {Temp: 0.0, Downfall: 0.5, Water: color.RGBA{63, 118, 228, 255}},
	"minecraft:savanna":          {Temp: 1.2, Downfall: 0.0, Water: color.RGBA{44, 113, 244, 255}},
	"minecraft:badlands":         {Temp: 2.0, Downfall: 0.0, Water: color.RGBA{78, 80, 189, 255}},
}

// getColorFromMap は color.RGBA を直接返してインタフェース型変換を回避
func getColorFromMap(img image.Image, temp, downfall float64) color.RGBA {
	if img == nil {
		return color.RGBA{85, 140, 50, 255}
	}
	temp = math.Max(0.0, math.Min(1.0, temp))
	downfall = math.Max(0.0, math.Min(1.0, downfall)) * temp

	bounds := img.Bounds()
	x := int((1.0 - temp) * float64(bounds.Dx()-1))
	y := int((1.0 - downfall) * float64(bounds.Dy()-1))

	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// GetTintedColor は BlockColorInfo.BiomeType に応じて、動的に対応するカラーマップ画像を取得して適用します
func GetTintedColor(info BlockColorInfo, biomeName string, colorMap map[string]image.Image) color.RGBA {
	baseColor := color.RGBA{R: info.Color[0], G: info.Color[1], B: info.Color[2], A: info.Color[3]}

	if info.BiomeType == "none" {
		return baseColor
	}

	bData, exists := BiomeTable[biomeName]
	if !exists {
		bData = BiomeTable["minecraft:plains"]
	}

	if info.BiomeType == "water" {
		return MultiplyColor(baseColor, bData.Water)
	}

	if cmapImg, ok := colorMap[info.BiomeType]; ok {
		tint := getColorFromMap(cmapImg, bData.Temp, bData.Downfall)
		return MultiplyColor(baseColor, tint)
	}

	return baseColor
}

// MultiplyColor は color.NRGBAModel.Convert を介さず直接 uint8 演算を行います
// (旧 multiplyColor を公開関数化: render パッケージからも利用するため)
func MultiplyColor(base, tint color.RGBA) color.RGBA {
	r := uint8((uint32(base.R) * uint32(tint.R)) / 255)
	g := uint8((uint32(base.G) * uint32(tint.G)) / 255)
	b := uint8((uint32(base.B) * uint32(tint.B)) / 255)
	a := base.A

	return color.RGBA{R: r, G: g, B: b, A: a}
}
