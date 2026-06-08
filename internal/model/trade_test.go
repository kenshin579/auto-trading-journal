package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleDomestic() Trade {
	return Trade{Date: "2026-02-13", TradeType: "매도", StockName: "삼성전자", StockCode: "005930",
		Quantity: 10, Price: 70000, Amount: 700000, Currency: "KRW", ExchangeRate: 1,
		AmountKRW: 700000, Fee: 1500, Profit: 50000, ProfitRate: 14.68, Account: "미래에셋증권_국내계좌"}
}

func TestToDomesticRow(t *testing.T) {
	tr := sampleDomestic()
	r := tr.ToDomesticRow()
	// 국내 12컬럼: 일자, 구분, 종목코드, 종목명, 섹터, 산업, 수량, 단가, 금액, 수수료, 손익, 수익률(소수)
	// 수익률은 Python과 동일하게 반올림 없이 ProfitRate/100 (동일 float64 변수의 런타임 나눗셈)으로 비교.
	require.Len(t, r, 12)
	assert.Equal(t, []any{"2026-02-13", "매도", "005930", "삼성전자", "", "",
		10.0, 70000.0, 700000.0, 1500.0, 50000.0, tr.ProfitRate / 100}, r)
}

func TestToForeignRow_17Cols(t *testing.T) {
	tr := sampleDomestic()
	tr.Account = "미래에셋증권_해외계좌"
	tr.Currency = "USD"
	r := tr.ToForeignRow()
	require.Len(t, r, 17)
	assert.Equal(t, "USD", r[2])
	assert.Equal(t, "005930", r[3])
	assert.Equal(t, "삼성전자", r[4])
}

func TestRate_MatchesPythonUnrounded(t *testing.T) {
	tr := sampleDomestic()
	tr.ProfitRate = 14.68
	row := tr.ToDomesticRow()
	assert.Equal(t, tr.ProfitRate/100, row[11]) // 반올림 없음, Python과 동일 (수익률 = 국내 index 11)
	tr.ProfitRate = 0
	assert.Equal(t, 0.0, tr.ToDomesticRow()[11])
}

func TestDuplicateKey_NumberNormalization(t *testing.T) {
	tr := sampleDomestic()
	tr.Quantity = 10
	tr.Price = 70000
	assert.Equal(t, DupKey{"2026-02-13", "매도", "삼성전자", "10", "70000"}, tr.DuplicateKey())
}

func TestToDomesticRow_WithSectorIndustry(t *testing.T) {
	tr := sampleDomestic()
	tr.Sector = "전기·전자"
	tr.Industry = "반도체"
	r := tr.ToDomesticRow()
	require.Len(t, r, 12)
	assert.Equal(t, "삼성전자", r[3])
	assert.Equal(t, "전기·전자", r[4])
	assert.Equal(t, "반도체", r[5])
	assert.Equal(t, 10.0, r[6])
	assert.Equal(t, 70000.0, r[7])
}

func TestToForeignRow_SectorIndustryBlankCols(t *testing.T) {
	tr := sampleDomestic()
	tr.Account = "미래에셋증권_해외계좌"
	tr.Currency = "USD"
	r := tr.ToForeignRow()
	require.Len(t, r, 17)
	assert.Equal(t, "삼성전자", r[4])
	assert.Equal(t, "", r[5])
	assert.Equal(t, "", r[6])
	assert.Equal(t, 10.0, r[7])
}
