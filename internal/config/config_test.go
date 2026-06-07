package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_YAMLAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"google_sheets:\n  spreadsheet_id: from_yaml\n  service_account_path: /sa.json\n"+
			"logging:\n  level: INFO\n"+
			"openai:\n  model: gpt-4o-mini\n  sector_cache_file: config/sector_cache.json\n"), 0o644))

	t.Setenv("GOOGLE_SPREADSHEET_ID", "from_env")
	t.Setenv("STOCK_DATA_OPENAI_API_KEY", "sk-test")

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "from_env", cfg.SpreadsheetID())   // env 우선
	require.Equal(t, "/sa.json", cfg.ServiceAccountPath)
	require.Equal(t, "gpt-4o-mini", cfg.OpenAI.Model)
	require.Equal(t, "sk-test", cfg.OpenAIAPIKey())
}
