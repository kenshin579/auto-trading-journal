package writer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRowToTradeDomestic(t *testing.T) {
	row := []interface{}{"2026-02-13", "매도", "005930", "삼성전자", "10", "75000", "750000", "1500", "50000", "0.0714"}
	tr := rowToTrade(row, false, "미래에셋증권_국내계좌")
	assert.Equal(t, "삼성전자", tr.StockName)
	assert.Equal(t, "005930", tr.StockCode)
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
		float64(10), float64(75000), float64(750000), float64(1500), float64(0), float64(0)}
	tr := rowToTrade(row, false, "acct")
	assert.Equal(t, 10.0, tr.Quantity)
	assert.Equal(t, 75000.0, tr.Price)
	assert.Equal(t, 0.0, tr.ProfitRate)
}

func TestRowToTradeForeign(t *testing.T) {
	row := []interface{}{"2026-02-13", "매도", "USD", "AAPL", "애플",
		float64(5), float64(180.5), float64(902.5), float64(1380.2), float64(1245630),
		float64(1.0), float64(0.5), float64(100.5), float64(138710), float64(0.1268)}
	tr := rowToTrade(row, true, "미래에셋증권_해외계좌")
	assert.Equal(t, "USD", tr.Currency)
	assert.Equal(t, "AAPL", tr.StockCode)
	assert.Equal(t, "애플", tr.StockName)
	assert.Equal(t, 5.0, tr.Quantity)
	assert.Equal(t, 180.5, tr.Price)
	assert.Equal(t, 902.5, tr.Amount)
	assert.Equal(t, 1380.2, tr.ExchangeRate)
	assert.Equal(t, 1245630.0, tr.AmountKRW)
	assert.Equal(t, 1.0, tr.Fee)
	assert.Equal(t, 0.5, tr.Tax)
	assert.Equal(t, 100.5, tr.Profit)
	assert.Equal(t, 138710.0, tr.ProfitKRW)
	assert.InDelta(t, 12.68, tr.ProfitRate, 0.01)
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
