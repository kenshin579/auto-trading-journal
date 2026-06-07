package parser

import (
	"encoding/csv"
	"fmt"
	"os"
)

var registry = []Parser{MiraeDomestic{}, MiraeForeign{}, HankookDomestic{}}

func DetectParser(path string) (Parser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("헤더 읽기 실패: %s: %w", base(path), err)
	}
	for _, p := range registry {
		if p.CanParse(header) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("지원되지 않는 CSV 포맷: %s", path)
}
