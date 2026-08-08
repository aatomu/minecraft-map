// Package config は config.json のスキーマと読み込み処理を提供します。
package config

// Config は config.json 全体のスキーマです
type Config struct {
	RegionDir      string              `json:"regionDir"`
	Resources      []string            `json:"resources"` // .jar/.zip/ディレクトリのリスト。先頭=初期値、末尾=上書き後の値
	MapColorJson   string              `json:"mapColorJson"`
	Export         ConfigExport        `json:"export"`
	FallBackColor  ConfigFallbackColor `json:"fallbackColor"`
	FallbackBlocks map[string]string   `json:"fallbackBlocks"`
	SuppressBlocks []string            `json:"suppressBlocks"`
}

type ConfigFallbackColor struct {
	Ungenerated  [4]uint8 `json:"ungenerated"`  // 未生成チャンク
	RegionError  [4]uint8 `json:"regionError"`  // リージョンファイル自体が開けない(欠損・破損等)
	ChunkError   [4]uint8 `json:"chunkError"`   // リージョン内の個別チャンクの読み込み(I/O・解凍)失敗
	ParseError   [4]uint8 `json:"parseError"`   // チャンクNBTのパース失敗(データ破損等)
	Void         [4]uint8 `json:"void"`         // 全ブロックがsuppress対象(air等)の正常な空洞(既定: 透明)
	MissingColor [4]uint8 `json:"missingColor"` // ブロックは検出できたが色情報が map_color.json に無い
	Other        [4]uint8 `json:"other"`        // 上記いずれにも該当しない、予期しない状態(通常は発生しない)
}

type ConfigExport struct {
	Dir      string `json:"dir"`      // 出力先
	Shading  bool   `json:"shading"`  // 影の有無
	ByRegion bool   `json:"byRegion"` // リージョン単位で出力するか
	Mode     string `json:"mode"`     // Mode は PNG 出力モード ("default" | "compression")
}
