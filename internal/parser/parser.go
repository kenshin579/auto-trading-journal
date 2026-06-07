package parser

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kenshin579/auto-trading-journal/internal/model"
)

// Parser: 증권사별 CSV 파서.
type Parser interface {
	Name() string
	CanParse(header []string) bool
	Parse(path string, account string) ([]model.Trade, error)
}

func parseFloat(v string) float64 {
	c := strings.ReplaceAll(strings.Trim(strings.TrimSpace(v), `"`), ",", "")
	if c == "" {
		return 0
	}
	f, err := strconv.ParseFloat(c, 64)
	if err != nil {
		return 0
	}
	return f
}

func convertDate(s string) string {
	return strings.ReplaceAll(strings.Trim(strings.TrimSpace(s), `"`), "/", "-")
}

func base(path string) string { return filepath.Base(path) }

func trimSpace(s string) string { return strings.TrimSpace(s) }

func hasAll(header []string, keys ...string) bool {
	set := map[string]bool{}
	for _, h := range header {
		set[strings.Trim(strings.TrimSpace(h), `"`)] = true
	}
	for _, k := range keys {
		if !set[k] {
			return false
		}
	}
	return true
}
