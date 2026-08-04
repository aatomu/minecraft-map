package main

import (
	"encoding/json"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if len(os.Args) < 2 {
		log.Fatalf("[FATAL] Please specify command: 'color' or 'generate'")
	}

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
			log.Fatalf("[FATAL] Target region not found in: %s", config.RegionDir)
		}

		fallback := FallbackColors{
			Ungenerated: toRGBA(config.FallBackColor.Ungenerated),
			ReadError:   toRGBA(config.FallBackColor.ReadError),
			Other:       toRGBA(config.FallBackColor.Other),
		}

		// 3. config.json の byRegion 設定に基づいて処理を切り替え
		if config.Export.ByRegion {
			log.Println("[INFO] Mode: Exporting individual region maps...")
			err = exportMapRegion(
				config.RegionDir,
				regionList,
				fallback,
				colorMap,
				blockColor,
				config.Export.Shading,
				config.Export.Dir,
			)
		} else {
			log.Println("[INFO] Mode: Exporting single combined map...")
			exportPath := filepath.Join(config.Export.Dir, "map.png")
			err = exportMapFull(
				config.RegionDir,
				regionList,
				fallback,
				colorMap,
				blockColor,
				config.Export.Shading,
				exportPath,
			)
		}

		if err != nil {
			log.Fatalf("[FATAL] Failed to export: %v\n", err)
		} else {
			log.Println("[INFO] Success to export map!")
		}

	default:
		log.Fatalf("[FATAL] Unknown command: %s (use 'color' or 'generate')", cmd)
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
