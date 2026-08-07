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
	"minecraft:spruce_leaves": constTint(0x61, 0x99, 0x61),
	"minecraft:birch_leaves":  constTint(0x80, 0xa7, 0x55),
	"minecraft:lily_pad":      constTint(0x20, 0x80, 0x30),

	// --- age駆動（茎） ---
	"minecraft:melon_stem_stage7":     stemTintFromProps,
	"minecraft:pumpkin_stem_stage7":   stemTintFromProps,
	"minecraft:attached_melon_stem":   fixedAgeStemTint(7),
	"minecraft:attached_pumpkin_stem": fixedAgeStemTint(7),
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

type fileEntry struct {
	open func() (io.ReadCloser, error)
}

// collectResourceEntries は resourcePaths に列挙された *.jar / *.zip / ディレクトリ を
// 「先頭が初期値(最も優先度が低い)、末尾が最終的な上書き値(最も優先度が高い)」の順で
// 単一の仮想ファイルマップへマージします。戻り値の closers は呼び出し側で defer Close してください。
func collectResourceEntries(resourcePaths []string) (map[string]fileEntry, []io.Closer, error) {
	if len(resourcePaths) == 0 {
		return nil, nil, fmt.Errorf("resources is empty: specify at least one .jar/.zip/directory in config.json's \"resources\"")
	}

	entries := make(map[string]fileEntry)
	var closers []io.Closer
	sourceCount := 0

	for _, fullPath := range resourcePaths {
		info, err := os.Stat(fullPath)
		if err != nil {
			log.Printf("[WARN] Failed to stat resource %s: %v\n", fullPath, err)
			continue
		}
		lowerName := strings.ToLower(fullPath)

		switch {
		case info.IsDir():
			if err := addDirSource(fullPath, entries); err != nil {
				log.Printf("[WARN] Failed to load resource directory %s: %v\n", fullPath, err)
				continue
			}
			log.Printf("[INFO] Loaded resource directory (priority %d): %s\n", sourceCount, fullPath)
			sourceCount++

		case strings.HasSuffix(lowerName, ".jar") || strings.HasSuffix(lowerName, ".zip"):
			zr, err := zip.OpenReader(fullPath)
			if err != nil {
				log.Printf("[WARN] Failed to open archive %s: %v\n", fullPath, err)
				continue
			}
			closers = append(closers, zr)
			for _, f := range zr.File {
				if f.FileInfo().IsDir() {
					continue
				}
				zf := f
				// 後から処理されたソースほど優先されるため、単純に上書き代入でよい
				entries[filepath.ToSlash(zf.Name)] = fileEntry{open: func() (io.ReadCloser, error) {
					return zf.Open()
				}}
			}
			log.Printf("[INFO] Loaded resource archive (priority %d): %s\n", sourceCount, fullPath)
			sourceCount++

		default:
			log.Printf("[WARN] Unsupported resource entry (expected .jar/.zip/directory): %s\n", fullPath)
			continue
		}
	}

	if sourceCount == 0 {
		return nil, closers, fmt.Errorf("no valid .jar/.zip/directory resource sources found in resources list")
	}

	return entries, closers, nil
}

// addDirSource は展開済みディレクトリ (例: assets/minecraft/... を含むフォルダ) を
// 相対パスをキーとして entries へ追加します。
func addDirSource(root string, entries map[string]fileEntry) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		p := path
		entries[key] = fileEntry{open: func() (io.ReadCloser, error) {
			return os.Open(p)
		}}
		return nil
	})
}

func closeAll(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}

// splitNamespacedID は "namespace:path" 形式の文字列を分解します。
// namespace 省略時は defaultNamespace を使用します。
func splitNamespacedID(id, defaultNamespace string) (namespace, path string) {
	if idx := strings.Index(id, ":"); idx != -1 {
		return id[:idx], id[idx+1:]
	}
	return defaultNamespace, id
}

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
			resultBlocks[normalizeBlockID(name)] = BlockColorInfo{Color: avgColor, BiomeType: bType}
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

		resultBlocks[normalizeBlockID(blockID)] = BlockColorInfo{
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

func decodeResourceImage(entry fileEntry) image.Image {
	rc, err := entry.open()
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

// resolveTexturePath はブロックモデル JSON (namespace 付き) を parent チェーンに沿って解決し、
// 最終的に使用すべきテクスチャの namespace とパス (拡張子・"assets/<ns>/textures/" を除いた部分) を返します。
func resolveTexturePath(namespace, modelPath string, entries map[string]fileEntry) (string, string, error) {
	textures := make(map[string]string)
	currentNS := namespace
	currentPath := modelPath

	for i := 0; i < 10; i++ {
		entry, exists := entries[currentPath]
		if !exists {
			break
		}

		model, err := parseModelJSON(entry)
		if err != nil {
			return "", "", err
		}

		for k, v := range model.Textures {
			if _, ok := textures[k]; !ok {
				textures[k] = v
			}
		}

		if model.Parent == "" {
			break
		}

		parentNS, parentRest := splitNamespacedID(model.Parent, currentNS)
		currentNS = parentNS
		currentPath = fmt.Sprintf("assets/%s/models/%s.json", parentNS, parentRest)
	}

	if len(textures) == 0 {
		return "", "", nil
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

	texNS, texPath := splitNamespacedID(selectedRaw, namespace)
	return texNS, texPath, nil
}

func parseModelJSON(entry fileEntry) (*ModelJSON, error) {
	rc, err := entry.open()
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

// multiplyColor は color.NRGBAModel.Convert を介さず直接 uint8 演算を行います
func multiplyColor(base, tint color.RGBA) color.RGBA {
	r := uint8((uint32(base.R) * uint32(tint.R)) / 255)
	g := uint8((uint32(base.G) * uint32(tint.G)) / 255)
	b := uint8((uint32(base.B) * uint32(tint.B)) / 255)
	a := base.A

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
