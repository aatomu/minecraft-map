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
	Ungenerated [4]uint8 `json:"ungenerated"` // 未生成
	ReadError   [4]uint8 `json:"readError"`   // 読み込み/解析エラー
	Other       [4]uint8 `json:"other"`       // その他(未登録ブロック等)
}

type ConfigExport struct {
	Dir      string `json:"dir"`      // 出力先
	Shading  bool   `json:"shading"`  // 影の有無
	ByRegion bool   `json:"byRegion"` // リージョン単位で出力するか
	Mode     string `json:"mode"`     // Mode は PNG 出力モード ("default" | "compression")
}
