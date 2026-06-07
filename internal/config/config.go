package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GoogleSheets struct {
		SpreadsheetID      string `yaml:"spreadsheet_id"`
		ServiceAccountPath string `yaml:"service_account_path"`
	} `yaml:"google_sheets"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
	OpenAI struct {
		Model           string `yaml:"model"`
		SectorCacheFile string `yaml:"sector_cache_file"`
	} `yaml:"openai"`
	BatchSize int `yaml:"batch_size"`

	// 편의 접근자용 (YAML 외부)
	ServiceAccountPath string `yaml:"-"`
}

// SpreadsheetID 는 env(GOOGLE_SPREADSHEET_ID) 우선, 없으면 YAML 값.
func (c *Config) SpreadsheetID() string {
	if v := os.Getenv("GOOGLE_SPREADSHEET_ID"); v != "" {
		return v
	}
	return c.GoogleSheets.SpreadsheetID
}

// OpenAIAPIKey 는 env(STOCK_DATA_OPENAI_API_KEY). 미설정이면 "".
func (c *Config) OpenAIAPIKey() string { return os.Getenv("STOCK_DATA_OPENAI_API_KEY") }

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("설정 파일을 찾을 수 없습니다: %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config 파싱 실패: %w", err)
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "INFO"
	}
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = "gpt-4o-mini"
	}
	if c.OpenAI.SectorCacheFile == "" {
		c.OpenAI.SectorCacheFile = "config/sector_cache.json"
	}
	c.ServiceAccountPath = c.GoogleSheets.ServiceAccountPath
	return &c, nil
}
