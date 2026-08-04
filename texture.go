package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BlockColorInfo は各ブロックの情報構造
type BlockColorInfo struct {
	Color     [4]uint8 `json:"color"`     // [R, G, B, A]
	BiomeType string   `json:"biomeType"` // "none" | "grass" | "water" | "foliage" など
}

// MapData は map_color.json 全体の構造
type MapData struct {
	ColorMap map[string]string         `json:"color_map"` // キー: カラーマップ名 ("grass", "foliage" 等), 値: Base64文字列
	Blocks   map[string]BlockColorInfo `json:"blocks"`
}

type ModelJSON struct {
	Parent   string            `json:"parent"`
	Textures map[string]string `json:"textures"`
}

type tintFunc func(props map[string]string) color.RGBA

var tintResolvers = map[string]tintFunc{
	// --- 固定色（バイオーム非依存） ---
	"spruce_leaves": constTint(0x61, 0x99, 0x61),
	"birch_leaves":  constTint(0x80, 0xa7, 0x55),
	"lily_pad":      constTint(0x20, 0x80, 0x30),

	// --- age駆動（茎） ---
	"melon_stem_stage7":     stemTintFromProps,
	"pumpkin_stem_stage7":   stemTintFromProps,
	"attached_melon_stem":   fixedAgeStemTint(7),
	"attached_pumpkin_stem": fixedAgeStemTint(7),
}

// constTint は固定色を返すクロージャを生成
func constTint(r, g, b uint8) tintFunc {
	return func(_ map[string]string) color.RGBA {
		return color.RGBA{R: r, G: g, B: b, A: 255}
	}
}

// stemTintFromProps は Properties["age"] を読んで計算する
func stemTintFromProps(props map[string]string) color.RGBA {
	age, _ := strconv.Atoi(props["age"]) // 未設定時は0
	return stemTint(age)
}

// fixedAgeStemTint は age を固定値に決め打ちしたクロージャを返す
func fixedAgeStemTint(age int) tintFunc {
	return func(_ map[string]string) color.RGBA {
		return stemTint(age)
	}
}

func stemTint(age int) color.RGBA {
	if age < 0 {
		age = 0
	} else if age > 7 {
		age = 7
	}
	r := age * 32
	if r > 255 {
		r = 255
	}
	return color.RGBA{
		R: uint8(r),
		G: uint8(255 - age*8),
		B: uint8(age * 4),
		A: 255,
	}
}

func ExtractMapColors(jarPath, jsonPath string) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return fmt.Errorf("Failed to load client.jar: %w", err)
	}
	defer r.Close()

	fileMap := make(map[string]*zip.File)
	for _, f := range r.File {
		fileMap[f.Name] = f
	}

	colorMapB64 := make(map[string]string)

	// JARからカラーマップ画像の全自動/動的スキャン
	// (assets/minecraft/textures/colormap/ 内の png をすべて検出)
	colormapPrefix := "assets/minecraft/textures/colormap/"
	for path, f := range fileMap {
		if strings.HasPrefix(path, colormapPrefix) && strings.HasSuffix(path, ".png") {
			keyName := strings.TrimSuffix(filepath.Base(path), ".png")
			img := decodeZipImage(f)
			if img != nil {
				colorMapB64[keyName] = encodeImageToBase64(img)
			}
		}
	}

	resultBlocks := make(map[string]BlockColorInfo)

	// 特殊流体ブロック (water / lava) の定義
	fluidMap := map[string]string{
		"water": "assets/minecraft/textures/block/water_still.png",
		"lava":  "assets/minecraft/textures/block/lava_still.png",
	}
	for name, jPath := range fluidMap {
		if f, ok := fileMap[jPath]; ok {
			avgColor := calculateAverageColor(f)
			bType := "none"
			if name == "water" {
				bType = "water"
				avgColor[3] = 180 // 水に透過性を設定
			}
			resultBlocks[name] = BlockColorInfo{Color: avgColor, BiomeType: bType}
		}
	}

	// 各ブロックモデル JSON の解析
	blockModelPrefix := "assets/minecraft/models/block/"
	for path := range fileMap {
		if !strings.HasPrefix(path, blockModelPrefix) || !strings.HasSuffix(path, ".json") {
			continue
		}

		blockID := strings.TrimSuffix(filepath.Base(path), ".json")
		texturePath, err := resolveTexturePath(path, fileMap)
		if err != nil || texturePath == "" {
			continue
		}

		jarTexturePath := fmt.Sprintf("assets/minecraft/textures/%s.png", texturePath)
		texFile, exists := fileMap[jarTexturePath]
		if !exists {
			continue
		}

		avgColor := calculateAverageColor(texFile)
		biomeType := determineBiomeType(blockID)

		resultBlocks[blockID] = BlockColorInfo{
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
	return os.WriteFile(jsonPath, jsonData, 0644)
}

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

func calculateAverageColor(f *zip.File) [4]uint8 {
	img := decodeZipImage(f)
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

func decodeZipImage(f *zip.File) image.Image {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	img, _, err := image.Decode(rc)
	if err != nil {
		return nil
	}
	return img
}

func resolveTexturePath(modelPath string, fileMap map[string]*zip.File) (string, error) {
	textures := make(map[string]string)
	currentPath := modelPath

	for i := 0; i < 10; i++ {
		file, exists := fileMap[currentPath]
		if !exists {
			break
		}

		model, err := parseModelJSON(file)
		if err != nil {
			return "", err
		}

		for k, v := range model.Textures {
			if _, ok := textures[k]; !ok {
				textures[k] = v
			}
		}

		if model.Parent == "" {
			break
		}

		parentName := strings.TrimPrefix(model.Parent, "minecraft:")
		currentPath = fmt.Sprintf("assets/minecraft/models/%s.json", parentName)
	}

	if len(textures) == 0 {
		return "", nil
	}

	priorityKeys := []string{"layer0", "top", "all", "side", "texture", "particle"}
	var selectedRaw string

	for _, k := range priorityKeys {
		if val, ok := textures[k]; ok {
			selectedRaw = val
			break
		}
	}

	if selectedRaw == "" {
		for _, val := range textures {
			selectedRaw = val
			break
		}
	}

	visitedRef := make(map[string]bool)
	for strings.HasPrefix(selectedRaw, "#") {
		refKey := strings.TrimPrefix(selectedRaw, "#")
		if visitedRef[refKey] {
			break
		}
		visitedRef[refKey] = true

		if nextVal, ok := textures[refKey]; ok {
			selectedRaw = nextVal
		} else {
			break
		}
	}

	return strings.TrimPrefix(selectedRaw, "minecraft:"), nil
}

func parseModelJSON(file *zip.File) (*ModelJSON, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var model ModelJSON
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, err
	}

	return &model, nil
}

// バイオーム色の適用・乗算ロジック
type BiomeData struct {
	Temp     float64
	Downfall float64
	Water    color.RGBA
}

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

func getColorFromMap(img image.Image, temp, downfall float64) color.Color {
	if img == nil {
		return color.RGBA{85, 140, 50, 255}
	}
	temp = math.Max(0.0, math.Min(1.0, temp))
	downfall = math.Max(0.0, math.Min(1.0, downfall)) * temp

	bounds := img.Bounds()
	x := int((1.0 - temp) * float64(bounds.Dx()-1))
	y := int((1.0 - downfall) * float64(bounds.Dy()-1))

	return img.At(x, y)
}

// BiomeType から動的に対応するカラーマップ画像を取得して適用
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
		return multiplyColor(baseColor, bData.Water)
	}

	if cmapImg, ok := colorMap[info.BiomeType]; ok {
		tint := getColorFromMap(cmapImg, bData.Temp, bData.Downfall)
		return multiplyColor(baseColor, tint)
	}

	return baseColor
}

// --- ヘルパー関数 ---

func multiplyColor(base, tint color.Color) color.RGBA {
	// color.NRGBAModel を使うことで、アルファ乗算を解除した生の 0~255 の RGB を安全に取得
	nrgbaBase := color.NRGBAModel.Convert(base).(color.NRGBA)
	nrgbaTint := color.NRGBAModel.Convert(tint).(color.NRGBA)

	// 0〜255 同士の乗算
	r := uint8((uint32(nrgbaBase.R) * uint32(nrgbaTint.R)) / 255)
	g := uint8((uint32(nrgbaBase.G) * uint32(nrgbaTint.G)) / 255)
	b := uint8((uint32(nrgbaBase.B) * uint32(nrgbaTint.B)) / 255)
	a := nrgbaBase.A

	return color.RGBA{R: r, G: g, B: b, A: a}
}

func encodeImageToBase64(img image.Image) string {
	if img == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodeBase64Image(b64Str string) image.Image {
	if b64Str == "" {
		return nil
	}

	cleanStr := b64Str
	if idx := strings.Index(b64Str, ","); idx != -1 {
		cleanStr = b64Str[idx+1:]
	}

	cleanStr = strings.ReplaceAll(cleanStr, "\r", "")
	cleanStr = strings.ReplaceAll(cleanStr, "\n", "")
	cleanStr = strings.ReplaceAll(cleanStr, " ", "")

	data, err := base64.StdEncoding.DecodeString(cleanStr)
	if err != nil {
		return nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}
