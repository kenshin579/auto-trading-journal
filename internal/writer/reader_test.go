package writer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRowToTradeDomestic_12cols(t *testing.T) {
	row := []interface{}{"2026-02-13", "매도", "005930", "삼성전자", "전기·전자", "반도체",
		"10", "75000", "750000", "1500", "50000", "0.0714"}
	tr := rowToTrade(row, false, "미래에셋증권_국내계좌")
	assert.Equal(t, "삼성전자", tr.StockName)
	assert.Equal(t, "005930", tr.StockCode)
	assert.Equal(t, "전기·전자", tr.Sector)
	assert.Equal(t, "반도체", tr.Industry)
	assert.Equal(t, 10.0, tr.Quantity)
	assert.Equal(t, 75000.0, tr.Price)
	assert.InDelta(t, 7.14, tr.ProfitRate, 0.01)
	assert.Equal(t, "KRW", tr.Currency)
	assert.Equal(t, 1.0, tr.ExchangeRate)
	assert.Equal(t, 750000.0, tr.Amount)
	assert.Equal(t, 750000.0, tr.AmountKRW)
	assert.Equal(t, 50000.0, tr.Profit)
	assert.Equal(t, 50000.0, tr.ProfitKRW)
	assert.Equal(t, 0.0, tr.Tax)
	assert.Equal(t, "미래에셋증권_국내계좌", tr.Account)
}

// 그리드 effectiveValue(float64) 입력으로도 동일하게 동작해야 한다.
func TestRowToTradeDomesticFloat(t *testing.T) {
	row := []interface{}{"2026-02-13", "매수", "005930", "삼성전자",
		"전기·전자", "반도체",
		float64(10), float64(75000), float64(750000), float64(1500), float64(0), float64(0)}
	tr := rowToTrade(row, false, "acct")
	assert.Equal(t, 10.0, tr.Quantity)
	assert.Equal(t, 75000.0, tr.Price)
	assert.Equal(t, 0.0, tr.ProfitRate)
}

func TestRowToTradeForeign_17cols(t *testing.T) {
	row := []interface{}{"2026-03-15", "매도", "USD", "AAPL", "Apple", "Technology", "Consumer Electronics",
		"5", "182", "910", "1365", "1241650", "910", "150", "32.5", "46700", "0.0371"}
	tr := rowToTrade(row, true, "미래에셋증권_해외계좌")
	assert.Equal(t, "AAPL", tr.StockCode)
	assert.Equal(t, "Apple", tr.StockName)
	assert.Equal(t, "Technology", tr.Sector)
	assert.Equal(t, "Consumer Electronics", tr.Industry)
	assert.Equal(t, 5.0, tr.Quantity)
	assert.Equal(t, 46700.0, tr.ProfitKRW)
	assert.InDelta(t, 3.71, tr.ProfitRate, 0.01)
}

// 종목코드가 숫자로 저장된 경우(TEXT 포맷 이전 행) 정수 문자열로 복원.
func TestGetCodeFromNumber(t *testing.T) {
	row := []interface{}{"", "", float64(461270)}
	assert.Equal(t, "461270", getCode(row, 2))
}

func TestExtractHeaderRow(t *testing.T) {
	vals := [][]interface{}{{"일자", "구분", "종목코드"}}
	assert.Equal(t, []string{"일자", "구분", "종목코드"}, extractHeaderRow(vals))
	assert.Nil(t, extractHeaderRow(nil))
}

func TestHeadersEqual(t *testing.T) {
	assert.True(t, headersEqual(DomesticHeaders, DomesticHeaders))
	assert.False(t, headersEqual(DomesticHeaders, ForeignHeaders))
	assert.False(t, headersEqual(DomesticHeaders, OldDomesticHeadersV1))
}
