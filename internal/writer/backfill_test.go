package writer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackfillValues(t *testing.T) {
	resolve := func(code, name string) (string, string) {
		switch code {
		case "214150":
			return "의료·정밀기기", "의료용 기기 제조업"
		case "487240":
			return "ETF", "미국주식" // 종목명 기반 ETF 분류
		default:
			return "", ""
		}
	}
	got := backfillValues(
		[]string{"214150", "", "487240"},
		[]string{"클래시스", "", "KODEX 미국AI전력핵심설비"},
		resolve)
	want := [][]interface{}{
		{"의료·정밀기기", "의료용 기기 제조업"},
		{"", ""},
		{"ETF", "미국주식"},
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

func TestColStrings(t *testing.T) {
	vals := [][]interface{}{
		{"005930", "삼성전자"},
		{}, // 빈 행
		{"487240", "KODEX 미국AI전력핵심설비"},
	}
	assert.Equal(t, []string{"005930", "", "487240"}, colStrings(vals, 0))         // 코드(C)
	assert.Equal(t, []string{"삼성전자", "", "KODEX 미국AI전력핵심설비"}, colStrings(vals, 1)) // 종목명(D)
}
