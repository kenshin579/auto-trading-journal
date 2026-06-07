package parser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kenshin579/auto-trading-journal/internal/model"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// Parser: 증권사별 CSV 파서.
type Parser interface {
	Name() string
	CanParse(header []string) bool
	Parse(path string, account string) ([]model.Trade, error)
}

// readCSVRows 는 CSV 파일을 읽어 모든 행을 반환한다. 증권사 CSV 는 CP949(EUC-KR)
// 인코딩이 흔하므로, 내용이 유효한 UTF-8 이 아니면 CP949 로 디코딩한다.
// (Python 은 run.sh 가 iconv 로 CP949→UTF-8 사전 변환했으나, Go 는 네이티브 처리한다.)
func readCSVRows(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		if decoded, derr := io.ReadAll(
			transform.NewReader(bytes.NewReader(data), korean.EUCKR.NewDecoder()),
		); derr == nil {
			data = decoded
		}
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

// readCSVHeader 는 CSV 첫 행(헤더)만 반환한다 (인코딩 처리 포함).
func readCSVHeader(path string) ([]string, error) {
	rows, err := readCSVRows(path)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("빈 CSV: %s", base(path))
	}
	return rows[0], nil
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
