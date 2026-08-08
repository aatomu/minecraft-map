package texture

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png" // PNGデコーダを登録するための副作用インポート
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// fileEntry は zip 内エントリ/実ファイルのどちらでも同じ方法で開けるようにするための抽象です
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
