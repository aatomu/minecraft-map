package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load は指定パスの config.json を読み込みます(旧 main.go の init() 内ロジック)。
// 失敗時のログ出力・os.Exit は呼び出し側(main)の責務とするため、ここではエラーを返すだけにしています。
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode %s: %w", path, err)
	}

	if err = cfg.Export.Validate(); err != nil {
		return nil, fmt.Errorf("export validate error: %w", err)
	}

	return &cfg, nil
}

func (c ConfigExport) Validate() error {
	if c.Mode != "default" && c.Mode != "compression" {
		return fmt.Errorf("export.mode must be \"default\" or \"compression\", got %q", c.Mode)
	}
	return nil
}
