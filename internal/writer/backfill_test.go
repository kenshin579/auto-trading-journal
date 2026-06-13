package writer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackfillValues(t *testing.T) {
	resolve := func(code string) (string, string) {
		switch code {
		case "214150":
			return "의료·정밀기기", "의료용 기기 제조업"
		case "487240":
			return "ETF", ""
		default:
			return "", ""
		}
	}
	got := backfillValues([]string{"214150", "", "487240"}, resolve)
	want := [][]interface{}{
		{"의료·정밀기기", "의료용 기기 제조업"},
		{"", ""},
		{"ETF", ""},
	}
	assert.Equal(t, want, got)
}

func TestPadStockCode(t *testing.T) {
	assert.Equal(t, "055550", padStockCode("55550"))  // 앞 0 유실 복원
	assert.Equal(t, "005935", padStockCode("5935"))   // 삼성전자우
	assert.Equal(t, "005930", padStockCode("005930")) // 이미 6자리 → 그대로
	assert.Equal(t, "0088N0", padStockCode("0088N0")) // 영문 포함 6자리 → 그대로
	assert.Equal(t, "", padStockCode(""))             // 빈값 → 그대로
	assert.Equal(t, "487240", padStockCode("487240")) // 6자리 숫자 → 그대로
}

func TestFirstColStrings(t *testing.T) {
	vals := [][]interface{}{
		{"005930"},
		{}, // 빈 행
		{"487240"},
	}
	assert.Equal(t, []string{"005930", "", "487240"}, firstColStrings(vals))
}
