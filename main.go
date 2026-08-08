package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aatomu/minecraft-map/internal/block"
	"github.com/aatomu/minecraft-map/internal/config"
	"github.com/aatomu/minecraft-map/internal/mapimg"
	"github.com/aatomu/minecraft-map/internal/region"
	"github.com/aatomu/minecraft-map/internal/texture"
)

func main() {
	start := time.Now()
	log.Println("[INFO] Starting to load config.json")
	cfg, err := config.Load("./config.json")
	if err != nil {
		log.Fatalf("[FATAL] %v", err)
	}

	log.Println("[INFO] Success to load config.json")

	if len(os.Args) < 2 {
		log.Fatalf("[FATAL] Please specify command: 'color' or 'generate'")
	}

	cmd := os.Args[1]

	switch cmd {
	case "color":
		log.Println("[INFO] Starting to parse textures")
		err := texture.ExtractMapColors(cfg.Resources, cfg.MapColorJson)
		if err != nil {
			log.Fatalf("[FATAL] Failed to parse textures: %v", err)
		}
		log.Println("[INFO] Success to parse textures")

	case "generate":
		cleanedFallbackBlocks := make(map[string]string, len(cfg.FallbackBlocks))
		for k, v := range cfg.FallbackBlocks {
			cleanedFallbackBlocks[block.Normalize(k)] = block.Normalize(v)
		}
		cfg.FallbackBlocks = cleanedFallbackBlocks

		// 1. map_color.json を取得
		blockColor, colorMap, err := texture.LoadMapColors(cfg.MapColorJson)
		if err != nil {
			log.Fatalf("[FATAL] Failed to read map color: %v", err)
		}

		// 2. ディレクトリ内の全 .mca ファイルを検出
		files, err := os.ReadDir(cfg.RegionDir)
		if err != nil {
			log.Fatalf("[FATAL] Failed to open region directory: %v", err)
		}

		var regionList []region.RegionPos
		for _, f := range files {
			if f.IsDir() || !strings.HasPrefix(f.Name(), "r.") || !strings.HasSuffix(f.Name(), ".mca") {
				continue
			}
			parts := strings.Split(f.Name(), ".")
			if len(parts) == 4 {
				rx, err1 := strconv.Atoi(parts[1])
				rz, err2 := strconv.Atoi(parts[2])
				if err1 == nil && err2 == nil {
					regionList = append(regionList, region.RegionPos{X: rx, Z: rz})
				}
			}
		}

		if len(regionList) == 0 {
			log.Fatalf("[FATAL] Target region not found in: %s", cfg.RegionDir)
		}

		fallback := mapimg.FallbackColors{
			Ungenerated:  mapimg.ToRGBA(cfg.FallBackColor.Ungenerated),
			RegionError:  mapimg.ToRGBA(cfg.FallBackColor.RegionError),
			ChunkError:   mapimg.ToRGBA(cfg.FallBackColor.ChunkError),
			ParseError:   mapimg.ToRGBA(cfg.FallBackColor.ParseError),
			Void:         mapimg.ToRGBA(cfg.FallBackColor.Void),
			MissingColor: mapimg.ToRGBA(cfg.FallBackColor.MissingColor),
			Other:        mapimg.ToRGBA(cfg.FallBackColor.Other),
		}

		// 3. suppressBlocksのSet化
		suppressMap := make(map[string]bool, len(cfg.SuppressBlocks))
		for _, blockID := range cfg.SuppressBlocks {
			suppressMap[block.Normalize(blockID)] = true
		}

		// 4. config.json の byRegion 設定に基づいて処理を切り替え
		if cfg.Export.ByRegion {
			log.Println("[INFO] Mode: Exporting individual region maps...")
			err = mapimg.ExportRegion(
				cfg.RegionDir,
				regionList,
				fallback,
				colorMap,
				blockColor,
				suppressMap,
				cfg.FallbackBlocks,
				cfg.Export.Shading,
				cfg.Export.Dir,
				cfg.Export.Mode,
			)
		} else {
			log.Println("[INFO] Mode: Exporting single combined map...")
			exportPath := filepath.Join(cfg.Export.Dir, "map.png")
			err = mapimg.ExportFull(
				cfg.RegionDir,
				regionList,
				fallback,
				colorMap,
				blockColor,
				suppressMap,
				cfg.FallbackBlocks,
				cfg.Export.Shading,
				exportPath,
				cfg.Export.Mode,
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

	log.Printf("[INFO] Program running for : %s", time.Since(start))
}
