package mapimg

import (
	"fmt"
	"image"
	"image/draw"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aatomu/minecraft-map/internal/region"
	"github.com/aatomu/minecraft-map/internal/texture"
)

// ExportRegion は各リージョンを個別の PNG ファイルとして出力します(旧 exportMapRegion)。
// exportMode は savePNG に渡す出力モード("default" / "compression")です。
func ExportRegion(rootDir string, regionList []region.RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]texture.BlockColorInfo, suppressMap map[string]bool, fallbackBlocks map[string]string, shading bool, exportDir string, exportMode string) error {
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return fmt.Errorf("failed to create export dir: %w", err)
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	log.Printf("[INFO] Processing individual regions in parallel (%d workers)...\n", numWorkers)

	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup
	var completed int32 = 0

	for _, rPos := range regionList {
		wg.Add(1)
		sem <- struct{}{}

		go func(rPos region.RegionPos) {
			defer wg.Done()
			defer func() {
				c := atomic.AddInt32(&completed, 1)
				log.Printf("[INFO] [%d/%d] r.%d.%d.png exported.\n", c, len(regionList), rPos.X, rPos.Z)
				<-sem
			}()

			res, err := renderRegion(rootDir, rPos, fallback, colorMap, blockColors, suppressMap, fallbackBlocks)
			if err != nil {
				log.Printf("[ERROR] Failed to render r.%d.%d: %v\n", rPos.X, rPos.Z, err)
				return
			}

			if shading {
				applyShading(res.Img, res.HeightMap, 512, 512)
			}

			outPath := filepath.Join(exportDir, fmt.Sprintf("r.%d.%d.png", rPos.X, rPos.Z))
			if err := savePNG(outPath, res.Img, exportMode); err != nil {
				log.Printf("[ERROR] Failed to save r.%d.%d.png: %v\n", rPos.X, rPos.Z, err)
			}
		}(rPos)
	}

	wg.Wait()
	return nil
}

// ExportFull は全リージョンを並列処理後に結合し、1枚の巨大な PNG マップとして出力します(旧 exportMapFull)。
func ExportFull(rootDir string, regionList []region.RegionPos, fallback FallbackColors, colorMap map[string]image.Image, blockColors map[string]texture.BlockColorInfo, suppressMap map[string]bool, fallbackBlocks map[string]string, shading bool, exportPath string, exportMode string) error {
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
	// int(8byte) ではなく int32(4byte) にすることで、巨大マップでのメモリ使用量を半減させる
	fullHeightBuffer := slices.Repeat([]int32{math.MinInt32}, imgWidth*imgHeight)

	// Parrallel Control
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup
	var completed int32 = 0
	log.Printf("[INFO] Processing full map regions in parallel (%d workers)...\n", numWorkers)

	allMissingBlocksSet := make(map[string]struct{})
	results := make(chan *RegionRenderResult, len(regionList))

	for _, rPos := range regionList {
		wg.Add(1)
		sem <- struct{}{}

		go func(rPos region.RegionPos) {
			defer wg.Done()
			defer func() {
				c := atomic.AddInt32(&completed, 1)
				log.Printf("[INFO] [%d/%d] r.%d.%d.mca rendered.\n", c, len(regionList), rPos.X, rPos.Z)
				<-sem
			}()

			res, err := renderRegion(rootDir, rPos, fallback, colorMap, blockColors, suppressMap, fallbackBlocks)
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
	return savePNG(exportPath, canvas, exportMode)
}
